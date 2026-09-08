package telnet

import (
	"errors"
	"io"
	"net"
	"testing"

	"github.com/sirupsen/logrus"
)

type drainTestListener struct{ closed bool }

func (l *drainTestListener) Accept() (net.Conn, error) { return nil, errors.New("not implemented") }
func (l *drainTestListener) Close() error              { l.closed = true; return nil }
func (l *drainTestListener) Addr() net.Addr            { return drainTestAddr("test") }

type drainTestAddr string

func (a drainTestAddr) Network() string { return string(a) }
func (a drainTestAddr) String() string  { return string(a) }

func TestDrainClosesListenerWithoutClosingConnections(t *testing.T) {
	listener := &drainTestListener{}
	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()

	server := &Server{
		listener:    listener,
		connections: map[string]*Connection{"existing": {Conn: serverSide}},
		logger:      logrus.New(),
	}
	if err := server.Drain(); err != nil {
		t.Fatalf("Drain() error = %v", err)
	}
	if !server.IsDraining() {
		t.Fatal("server should report draining")
	}
	if !listener.closed {
		t.Fatal("listener was not closed")
	}
	if got := server.GetActiveConnections(); got != 1 {
		t.Fatalf("active connections = %d, want 1", got)
	}
	read := make(chan error, 1)
	go func() {
		buf := make([]byte, len("still connected"))
		_, err := io.ReadFull(serverSide, buf)
		read <- err
	}()
	if _, err := clientSide.Write([]byte("still connected")); err != nil {
		t.Fatalf("existing connection was closed: %v", err)
	}
	if err := <-read; err != nil {
		t.Fatalf("existing connection could not read: %v", err)
	}
	_ = serverSide.Close()
}

func TestDrainIsIdempotent(t *testing.T) {
	server := &Server{logger: logrus.New()}
	if err := server.Drain(); err != nil {
		t.Fatalf("first Drain() error = %v", err)
	}
	if err := server.Drain(); err != nil {
		t.Fatalf("second Drain() error = %v", err)
	}
}
