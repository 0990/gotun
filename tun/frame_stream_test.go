package tun

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

type frameStreamTestConn struct {
	net.Conn
	id string
}

func (c *frameStreamTestConn) ID() string { return c.id }

func TestFrameStreamBandwidthDoneReturnsEOFBeforeClose(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()

	reader := NewFrameStream(&frameStreamTestConn{Conn: left, id: "reader"}, streamRoleBandwidth)
	writer := NewFrameStream(&frameStreamTestConn{Conn: right, id: "writer"}, streamRoleBandwidth)

	writeDone := make(chan error, 1)
	go func() {
		if _, err := writer.Write([]byte("hello")); err != nil {
			writeDone <- err
			return
		}
		writeDone <- writer.WriteBandwidthDone()
	}()

	buf := make([]byte, 16)
	n, err := reader.Read(buf)
	if err != nil {
		t.Fatalf("first read failed: %v", err)
	}
	if got := string(buf[:n]); got != "hello" {
		t.Fatalf("unexpected payload: %q", got)
	}

	eofCh := make(chan error, 1)
	go func() {
		_, readErr := reader.Read(buf)
		eofCh <- readErr
	}()

	select {
	case readErr := <-eofCh:
		if !errors.Is(readErr, io.EOF) {
			t.Fatalf("expected EOF after bandwidth done frame, got %v", readErr)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for EOF after bandwidth done frame")
	}

	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatalf("writer failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for bandwidth done frame write")
	}
}
