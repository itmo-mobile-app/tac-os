//go:build unix

package ipc

import (
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"time"
)

// maxSocketPath is below the limit of sockaddr_un on macOS (104 bytes) and Linux (108 bytes).
const maxSocketPath = 104

// Listen creates the Unix-domain socket the Runtime connects to. The socket is accessible only for the current
// user. Closing the returned listener removes the socket file again, and nothing else ever removes it.
//
// A leftover socket file of a process that no longer exists is replaced. A file that is not a socket and a
// socket that another process still listens on are left alone and cause an error.
func Listen(path string) (net.Listener, error) {
	if path == "" || len(path) >= maxSocketPath {
		return nil, fmt.Errorf("ipc: the socket path must be 1..%d bytes long: %q", maxSocketPath-1, path)
	}
	if err := removeStale(path); err != nil {
		return nil, err
	}

	// The socket is created with the permissions allowed by the umask, so no other user can connect to it
	// even for a moment. The umask is process-wide, but Listen is called once at startup.
	old := syscall.Umask(0o177)
	l, err := net.Listen("unix", path)
	syscall.Umask(old)
	if err != nil {
		return nil, fmt.Errorf("ipc: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		l.Close()
		return nil, fmt.Errorf("ipc: %w", err)
	}
	return l, nil
}

// removeStale deletes path only if it is a socket that nobody listens on.
func removeStale(path string) error {
	fi, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("ipc: %w", err)
	}
	if fi.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("ipc: %s exists and is not a socket, it is not removed", path)
	}

	c, err := net.DialTimeout("unix", path, time.Second)
	if err == nil {
		c.Close()
		return fmt.Errorf("ipc: another process is already listening on %s", path)
	}
	if !errors.Is(err, syscall.ECONNREFUSED) {
		return fmt.Errorf("ipc: cannot tell whether %s is in use: %w", path, err)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("ipc: %w", err)
	}
	return nil
}
