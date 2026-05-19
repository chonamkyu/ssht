package session

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/chonamkyu/ssht/internal/config"
	sshclient "github.com/chonamkyu/ssht/internal/ssh"
	"golang.org/x/term"
)

type SessionInfo struct {
	ID        int       `json:"id"`
	HostName  string    `json:"host_name"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at"`
	Socket    string    `json:"socket"`
}

type Session struct {
	Info   SessionInfo
	client *sshclient.Client
	stdin  io.WriteCloser
	stdout io.Reader
	buffer *RingBuffer
	mu     sync.Mutex
	done   chan struct{}
}

var (
	sessions   = make(map[int]*Session)
	sessionsMu sync.Mutex
	nextID     = 1
)

func sessionsDir() string {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".ssht", "sessions")
	os.MkdirAll(dir, 0700)
	return dir
}

func socketPath(id int) string {
	return filepath.Join(sessionsDir(), fmt.Sprintf("session-%d.sock", id))
}

func StartBackground(host *config.Host) (int, error) {
	client, err := sshclient.Connect(host)
	if err != nil {
		return 0, err
	}

	_, stdin, stdout, err := client.StartShell()
	if err != nil {
		client.Close()
		return 0, err
	}

	sessionsMu.Lock()
	id := nextID
	nextID++
	sessionsMu.Unlock()

	buf := NewRingBuffer(64 * 1024) // 64KB ring buffer

	s := &Session{
		Info: SessionInfo{
			ID:        id,
			HostName:  host.Name,
			Status:    "running",
			StartedAt: time.Now(),
			Socket:    socketPath(id),
		},
		client: client,
		stdin:  stdin,
		stdout: stdout,
		buffer: buf,
		done:   make(chan struct{}),
	}

	go s.readLoop()
	go s.startSocketServer()

	sessionsMu.Lock()
	sessions[id] = s
	sessionsMu.Unlock()

	s.saveInfo()

	return id, nil
}

func StartInteractive(host *config.Host) error {
	id, err := StartBackground(host)
	if err != nil {
		return err
	}

	return Attach(id)
}

func List() ([]SessionInfo, error) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	var result []SessionInfo
	for _, s := range sessions {
		result = append(result, s.Info)
	}

	if len(result) == 0 {
		result = loadSessionInfos()
	}

	return result, nil
}

func Attach(id int) error {
	s := getSession(id)
	if s != nil {
		return attachLocal(s)
	}

	// Try socket-based attach for daemon sessions
	conn, err := connectToSocket(id)
	if err != nil {
		return fmt.Errorf("session %d not found", id)
	}
	defer conn.Close()

	return attachRemote(conn)
}

func attachLocal(s *Session) error {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("setting raw terminal: %w", err)
	}
	defer term.Restore(fd, oldState)

	reader := s.buffer.NewReader()
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				os.Stdout.Write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	buf := make([]byte, 1024)
	detachState := false
	for {
		select {
		case <-s.done:
			fmt.Print("\r\nConnection closed.\r\n")
			return nil
		default:
		}

		n, err := os.Stdin.Read(buf)
		if err != nil {
			return err
		}

		for i := 0; i < n; i++ {
			if detachState && buf[i] == 'd' {
				fmt.Print("\r\nDetached.\r\n")
				return nil
			}
			detachState = buf[i] == 0x02 // Ctrl+B
		}

		if !detachState {
			s.mu.Lock()
			s.stdin.Write(buf[:n])
			s.mu.Unlock()
		} else if n > 1 {
			s.mu.Lock()
			s.stdin.Write(buf[:n])
			s.mu.Unlock()
			detachState = false
		}
	}
}

func attachRemote(conn net.Conn) error {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("setting raw terminal: %w", err)
	}
	defer term.Restore(fd, oldState)

	// Send attach request
	enc := json.NewEncoder(conn)
	if err := enc.Encode(socketMessage{Action: "attach"}); err != nil {
		return err
	}

	done := make(chan struct{})

	// Read output from socket → stdout
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				os.Stdout.Write(buf[:n])
			}
			if err != nil {
				close(done)
				return
			}
		}
	}()

	// Read stdin → send to socket
	buf := make([]byte, 1024)
	detachState := false
	for {
		select {
		case <-done:
			fmt.Print("\r\nConnection closed.\r\n")
			return nil
		default:
		}

		n, err := os.Stdin.Read(buf)
		if err != nil {
			return err
		}

		for i := 0; i < n; i++ {
			if detachState && buf[i] == 'd' {
				fmt.Print("\r\nDetached.\r\n")
				return nil
			}
			detachState = buf[i] == 0x02 // Ctrl+B
		}

		if !detachState {
			conn.Write(buf[:n])
		} else if n > 1 {
			conn.Write(buf[:n])
			detachState = false
		}
	}
}

func SendInput(id int, data []byte) error {
	s := getSession(id)
	if s == nil {
		conn, err := connectToSocket(id)
		if err != nil {
			return fmt.Errorf("session %d not found or not accessible", id)
		}
		defer conn.Close()
		return sendViaSocket(conn, "input", data)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.stdin.Write(data)
	return err
}

func ReadOutput(id int, lines int) (string, error) {
	s := getSession(id)
	if s == nil {
		conn, err := connectToSocket(id)
		if err != nil {
			return "", fmt.Errorf("session %d not found or not accessible", id)
		}
		defer conn.Close()
		return readViaSocket(conn, lines)
	}

	return s.buffer.LastLines(lines), nil
}

func Follow(id int) (io.Reader, error) {
	s := getSession(id)
	if s == nil {
		return nil, fmt.Errorf("session %d not found", id)
	}
	return s.buffer.NewReader(), nil
}

func Kill(id int) error {
	sessionsMu.Lock()
	s, ok := sessions[id]
	if ok {
		delete(sessions, id)
	}
	sessionsMu.Unlock()

	if !ok {
		// Try socket connection
		sockPath := socketPath(id)
		conn, err := net.Dial("unix", sockPath)
		if err != nil {
			// Try reading from session info file
			infoPath := filepath.Join(sessionsDir(), fmt.Sprintf("session-%d.json", id))
			data, rerr := os.ReadFile(infoPath)
			if rerr != nil {
				return fmt.Errorf("session %d not found", id)
			}
			var info SessionInfo
			if jerr := json.Unmarshal(data, &info); jerr != nil {
				return fmt.Errorf("session %d not found", id)
			}
			conn, err = net.Dial("unix", info.Socket)
			if err != nil {
				// Socket dead, just clean up files
				os.Remove(infoPath)
				os.Remove(info.Socket)
				return nil
			}
		}
		defer conn.Close()

		enc := json.NewEncoder(conn)
		enc.Encode(socketMessage{Action: "kill"})

		os.Remove(sockPath)
		removeSessionInfo(id)
		return nil
	}

	s.client.Close()
	close(s.done)
	os.Remove(s.Info.Socket)
	removeSessionInfo(id)
	return nil
}

func KillAll() (int, error) {
	sessionsMu.Lock()
	ids := make([]int, 0, len(sessions))
	for id := range sessions {
		ids = append(ids, id)
	}
	sessionsMu.Unlock()

	for _, id := range ids {
		Kill(id)
	}

	dir := sessionsDir()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		os.Remove(filepath.Join(dir, e.Name()))
	}

	return len(ids), nil
}

func removeSessionInfo(id int) {
	path := filepath.Join(sessionsDir(), fmt.Sprintf("session-%d.json", id))
	os.Remove(path)
}

func getSession(id int) *Session {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	return sessions[id]
}

func (s *Session) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := s.stdout.Read(buf)
		if n > 0 {
			s.buffer.Write(buf[:n])
		}
		if err != nil {
			s.Info.Status = "disconnected"
			close(s.done)
			return
		}
	}
}

func (s *Session) startSocketServer() {
	os.Remove(s.Info.Socket)

	ln, err := net.Listen("unix", s.Info.Socket)
	if err != nil {
		return
	}
	defer ln.Close()

	go func() {
		<-s.done
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go s.handleSocketConn(conn)
	}
}

func (s *Session) handleSocketConn(conn net.Conn) {
	defer conn.Close()

	var msg socketMessage
	dec := json.NewDecoder(conn)
	if err := dec.Decode(&msg); err != nil {
		return
	}

	enc := json.NewEncoder(conn)

	switch msg.Action {
	case "input":
		s.mu.Lock()
		_, err := s.stdin.Write(msg.Data)
		s.mu.Unlock()
		enc.Encode(socketResponse{OK: err == nil})
	case "read":
		output := s.buffer.LastLines(msg.Lines)
		enc.Encode(socketResponse{OK: true, Data: output})
	case "info":
		enc.Encode(socketResponse{OK: true, Data: s.Info.Status})
	case "kill":
		enc.Encode(socketResponse{OK: true})
		s.client.Close()
		os.Exit(0)
	case "attach":
		s.handleAttach(conn)
	}
}

func (s *Session) handleAttach(conn net.Conn) {
	reader := s.buffer.NewReader()

	done := make(chan struct{})

	// Stream buffer output to the attached client
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				if _, werr := conn.Write(buf[:n]); werr != nil {
					close(done)
					return
				}
			}
			if err != nil {
				close(done)
				return
			}
		}
	}()

	// Read input from attached client → stdin
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				s.mu.Lock()
				s.stdin.Write(buf[:n])
				s.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

	// Wait for disconnect or session end
	select {
	case <-done:
	case <-s.done:
	}
}

func (s *Session) saveInfo() {
	data, _ := json.Marshal(s.Info)
	path := filepath.Join(sessionsDir(), fmt.Sprintf("session-%d.json", s.Info.ID))
	os.WriteFile(path, data, 0600)
}

type socketMessage struct {
	Action string `json:"action"`
	Data   []byte `json:"data,omitempty"`
	Lines  int    `json:"lines,omitempty"`
}

type socketResponse struct {
	OK   bool   `json:"ok"`
	Data string `json:"data,omitempty"`
}

func connectToSocket(id int) (net.Conn, error) {
	return net.Dial("unix", socketPath(id))
}

func sendViaSocket(conn net.Conn, action string, data []byte) error {
	enc := json.NewEncoder(conn)
	if err := enc.Encode(socketMessage{Action: action, Data: data}); err != nil {
		return err
	}

	var resp socketResponse
	dec := json.NewDecoder(conn)
	if err := dec.Decode(&resp); err != nil {
		return err
	}

	if !resp.OK {
		return fmt.Errorf("send failed")
	}
	return nil
}

func readViaSocket(conn net.Conn, lines int) (string, error) {
	enc := json.NewEncoder(conn)
	if err := enc.Encode(socketMessage{Action: "read", Lines: lines}); err != nil {
		return "", err
	}

	var resp socketResponse
	dec := json.NewDecoder(conn)
	if err := dec.Decode(&resp); err != nil {
		return "", err
	}

	return resp.Data, nil
}

func loadSessionInfos() []SessionInfo {
	dir := sessionsDir()
	entries, _ := os.ReadDir(dir)
	var infos []SessionInfo
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var info SessionInfo
		if err := json.Unmarshal(data, &info); err != nil {
			continue
		}
		if _, err := net.Dial("unix", info.Socket); err != nil {
			os.Remove(filepath.Join(dir, e.Name()))
			continue
		}
		infos = append(infos, info)
	}
	return infos
}
