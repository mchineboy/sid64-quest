package terminalwire

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Listen supports private TCP or a Unix socket. A held advisory file lock makes
// removing a stale socket after SIGKILL safe against another core starting on
// the same path. An existing live listener or ordinary file is never replaced.
func Listen(address string) (net.Listener, error) {
	if !strings.HasPrefix(address, "unix:") {
		return net.Listen("tcp", address)
	}
	path := strings.TrimPrefix(address, "unix:")
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		lock.Close()
		return nil, fmt.Errorf("core socket in use: %w", err)
	}
	ok := false
	defer func() {
		if !ok {
			lock.Close()
		}
	}()
	if info, statErr := os.Lstat(path); statErr == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("core socket path is not a socket")
		}
		conn, dialErr := net.DialTimeout("unix", path, time.Second)
		if dialErr == nil {
			conn.Close()
			return nil, fmt.Errorf("core socket is already listening")
		}
		if !errors.Is(dialErr, syscall.ECONNREFUSED) {
			return nil, dialErr
		}
		if err = os.Remove(path); err != nil {
			return nil, err
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return nil, statErr
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err = os.Chmod(path, 0600); err != nil {
		listener.Close()
		return nil, err
	}
	ok = true
	return &lockedListener{Listener: listener, lock: lock}, nil
}

type lockedListener struct {
	net.Listener
	lock *os.File
	once sync.Once
	err  error
}

func (l *lockedListener) Close() error {
	l.once.Do(func() { l.err = l.Listener.Close(); _ = l.lock.Close() })
	return l.err
}
