package tools

import (
	"bytes"
	"encoding/binary"
	"io"
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

// ReadUInt32 read an uint32
func (r *StreamReader) ReadUInt32() (v uint32, err error) {
	bytes, err := r.ReadBytes(4)
	if err != nil {
		return
	}
	v = binary.BigEndian.Uint32(bytes)
	return
}

// ReadBytes read bytes from r
func (r *StreamReader) ReadBytes(delim int64) ([]byte, error) {
	w := bytes.NewBuffer(make([]byte, 0, delim))
	_, err := io.CopyN(w, r, delim)
	if err != nil {
		return nil, err
	}
	return w.Bytes(), nil
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
