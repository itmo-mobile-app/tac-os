package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"syscall"
	"time"
)

// Server owns its listener and a single in-memory database. Call Serve once.
type Server struct {
	listener    *net.UnixListener
	lock        *os.File
	path        string
	inode       os.FileInfo
	timeout     time.Duration
	executor    executor
	mu          sync.Mutex
	connections map[net.Conn]bool
	closed      bool
	once        sync.Once
	workers     sync.WaitGroup
}

func owned(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}

// Listen creates a user-only socket. The persistent .lock file is only a lifecycle
// guard, not database storage. Never remove it while a server may be starting.
func Listen(path string, timeout time.Duration) (*Server, error) {
	if path == "" || timeout <= 0 {
		return nil, fmt.Errorf("socket path and positive I/O timeout are required")
	}
	fd, err := syscall.Open(path+".lock", syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	lock := os.NewFile(uintptr(fd), path+".lock")
	success := false
	defer func() {
		if !success {
			_ = lock.Close()
		}
	}()
	info, err := lock.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || !owned(info) || info.Mode().Perm() != 0600 {
		return nil, fmt.Errorf("unsafe socket lock file")
	}
	if err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return nil, fmt.Errorf("socket is locked: %w", err)
	}
	info, err = os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSocket == 0 || !owned(info) {
			return nil, fmt.Errorf("refusing to remove non-socket or foreign socket: %s", path)
		}
		conn, dialErr := net.DialTimeout("unix", path, 200*time.Millisecond)
		if dialErr == nil {
			_ = conn.Close()
			return nil, fmt.Errorf("socket already active: %s", path)
		}
		if !errors.Is(dialErr, syscall.ECONNREFUSED) {
			return nil, fmt.Errorf("cannot establish that socket is stale: %w", dialErr)
		}
		if err = os.Remove(path); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	listener.SetUnlinkOnClose(false)
	info, err = os.Lstat(path)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	if err = os.Chmod(path, 0600); err != nil {
		_ = listener.Close()
		_ = os.Remove(path)
		return nil, err
	}
	success = true
	return &Server{listener: listener, lock: lock, path: path, inode: info, timeout: timeout, connections: make(map[net.Conn]bool)}, nil
}

// Close interrupts blocked I/O and removes only the socket created by this server.
func (s *Server) Close() {
	s.once.Do(func() {
		s.mu.Lock()
		s.closed = true
		_ = s.listener.Close()
		for conn := range s.connections {
			_ = conn.Close()
		}
		s.mu.Unlock()
		if info, err := os.Lstat(s.path); err == nil && os.SameFile(info, s.inode) && info.Mode()&os.ModeSocket != 0 {
			_ = os.Remove(s.path)
		}
		_ = s.lock.Close()
	})
}

func (s *Server) Serve(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			s.Close()
		case <-done:
		}
	}()
	defer close(done)
	defer func() { s.Close(); s.workers.Wait() }()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		s.mu.Lock()
		if s.closed {
			_ = conn.Close()
			s.mu.Unlock()
			return nil
		}
		s.connections[conn] = true
		s.workers.Add(1)
		s.mu.Unlock()
		go s.handle(conn)
	}
}
func (s *Server) handle(conn net.Conn) {
	defer s.workers.Done()
	defer func() { _ = conn.Close(); s.mu.Lock(); delete(s.connections, conn); s.mu.Unlock() }()
	for {
		if err := conn.SetReadDeadline(time.Now().Add(s.timeout)); err != nil {
			return
		}
		data, err := readFrame(conn)
		if err != nil {
			return
		}
		payload := s.executor.execute(data)
		if err = conn.SetWriteDeadline(time.Now().Add(s.timeout)); err != nil {
			return
		}
		if err = writeFrame(conn, payload); err != nil {
			return
		}
	}
}
