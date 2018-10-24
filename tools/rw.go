package tools

import (
	"net"
	"time"
)

// StreamReader implements buffering for an io.StreamReader object.
type StreamReader struct {
	conn    net.Conn
	timeout time.Duration
}

// NewStreamReader returns a new Reader.
func NewStreamReader(conn net.Conn, timeout time.Duration) *StreamReader {
	return &StreamReader{
		conn:    conn,
		timeout: timeout,
	}
}

func (r *StreamReader) Read(b []byte) (int, error) {
	err := r.conn.SetReadDeadline(time.Now().Add(r.timeout))
	if err != nil {
		return 0, err
	}
	return r.conn.Read(b)
}

// StreamWriter is stream writer
type StreamWriter struct {
	conn    net.Conn
	timeout time.Duration
}

// NewStreamWriter returns a new Writer.
func NewStreamWriter(conn net.Conn, timeout time.Duration) *StreamWriter {
	return &StreamWriter{
		conn:    conn,
		timeout: timeout,
	}
}

func (w *StreamWriter) Write(b []byte) (int, error) {
	err := w.conn.SetWriteDeadline(time.Now().Add(w.timeout))
	if err != nil {
		return 0, err
	}

	return w.conn.Write(b)
}
