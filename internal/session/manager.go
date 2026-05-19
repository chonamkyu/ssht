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
	client, err := sshclient.Connect(host)
	if err != nil {
		return err
	}
	defer client.Close()

	return client.Shell(os.Stdin, os.Stdout, os.Stderr)
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
	if s == nil {
		return fmt.Errorf("session %d not found", id)
	}

	fmt.Printf("Attached to session %d (%s). Press Ctrl+B then 'd' to detach.\n", id, s.Info.HostName)

	go func() {
		io.Copy(os.Stdout, s.buffer.NewReader())
	}()

	buf := make([]byte, 1024)
	detachState := false
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return err
		}

		for i := 0; i < n; i++ {
			if detachState && buf[i] == 'd' {
				fmt.Println("\nDetached.")
				return nil
			}
			detachState = buf[i] == 0x02 // Ctrl+B
		}

		if !detachState {
			s.stdin.Write(buf[:n])
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
		sockPath := socketPath(id)
		conn, err := net.Dial("unix", sockPath)
		if err != nil {
			return fmt.Errorf("session %d not found", id)
		}
		conn.Close()
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
