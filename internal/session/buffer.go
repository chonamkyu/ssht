package session

import (
	"io"
	"strings"
	"sync"
)

type RingBuffer struct {
	data    []byte
	size    int
	pos     int
	full    bool
	mu      sync.Mutex
	readers []*bufferReader
}

type bufferReader struct {
	buf    *RingBuffer
	notify chan struct{}
	closed bool
}

func NewRingBuffer(size int) *RingBuffer {
	return &RingBuffer{
		data: make([]byte, size),
		size: size,
	}
}

func (rb *RingBuffer) Write(p []byte) (int, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	n := len(p)
	for _, b := range p {
		rb.data[rb.pos] = b
		rb.pos = (rb.pos + 1) % rb.size
		if rb.pos == 0 {
			rb.full = true
		}
	}

	for _, r := range rb.readers {
		select {
		case r.notify <- struct{}{}:
		default:
		}
	}

	return n, nil
}

func (rb *RingBuffer) Contents() []byte {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if !rb.full {
		result := make([]byte, rb.pos)
		copy(result, rb.data[:rb.pos])
		return result
	}

	result := make([]byte, rb.size)
	copy(result, rb.data[rb.pos:])
	copy(result[rb.size-rb.pos:], rb.data[:rb.pos])
	return result
}

func (rb *RingBuffer) LastLines(n int) string {
	content := string(rb.Contents())
	lines := strings.Split(content, "\n")

	if len(lines) <= n {
		return content
	}

	return strings.Join(lines[len(lines)-n:], "\n")
}

func (rb *RingBuffer) NewReader() io.Reader {
	r := &bufferReader{
		buf:    rb,
		notify: make(chan struct{}, 1),
	}

	rb.mu.Lock()
	rb.readers = append(rb.readers, r)
	rb.mu.Unlock()

	return r
}

func (r *bufferReader) Read(p []byte) (int, error) {
	if r.closed {
		return 0, io.EOF
	}

	content := r.buf.Contents()
	if len(content) == 0 {
		<-r.notify
		content = r.buf.Contents()
	}

	n := copy(p, content)
	return n, nil
}
