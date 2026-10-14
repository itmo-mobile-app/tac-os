package server

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/itmo-mobile-app/tac-os/window-manager/backend"
	"github.com/itmo-mobile-app/tac-os/window-manager/protocol"
)

type receivedRequest struct {
	conn net.Conn
	req protocol.Request
	err error
}

type Server struct {
	SocketPath      string
	EventSocketPath string
	Backend         backend.Backend

	mu       sync.Mutex
	window   backend.Window
	windowID string
	stop     chan struct{}
	stopOnce sync.Once

	eventMu      sync.Mutex
	eventClients map[net.Conn]struct{}
}

func (s *Server) broadcastEvent(event, windowID string) {
	s.eventMu.Lock()
	defer s.eventMu.Unlock()

	response := protocol.Response{
		OK:       true,
		Event:    event,
		WindowID: windowID,
	}

	for conn := range s.eventClients {
		_ = conn.SetWriteDeadline(time.Now().Add(time.Second))

		if err := protocol.WriteResponse(conn, response); err != nil {
			_ = conn.Close()
			delete(s.eventClients, conn)
		}
	}
}

func (s *Server) Run(ctx context.Context) error {
	if s.SocketPath == "" {
		s.SocketPath = "/tmp/tacos-wm.sock"
	}
	if s.EventSocketPath == "" {
		s.EventSocketPath = "/tmp/tacos-wm-events.sock"
	}
	if s.Backend == nil {
		return errors.New("backend is required")
	}

	s.stop = make(chan struct{})
	s.stopOnce = sync.Once{}
	s.eventClients = make(map[net.Conn]struct{})

	_ = os.Remove(s.SocketPath)
	commandListener, err := net.Listen("unix", s.SocketPath)
	if err != nil {
		return fmt.Errorf(
			"listen on command socket %s: %w",
			s.SocketPath,
			err,
		)
	}

	_ = os.Remove(s.EventSocketPath)
	eventListener, err := net.Listen("unix", s.EventSocketPath)
	if err != nil {
		_ = commandListener.Close()
		_ = os.Remove(s.SocketPath)
		return fmt.Errorf(
			"listen on event socket %s: %w",
			s.EventSocketPath,
			err,
		)
	}

	_ = os.Chmod(s.SocketPath, 0600)
	_ = os.Chmod(s.EventSocketPath, 0600)

	defer func() {
		_ = commandListener.Close()
		_ = eventListener.Close()
		_ = os.Remove(s.SocketPath)
		_ = os.Remove(s.EventSocketPath)

		s.eventMu.Lock()
		defer s.eventMu.Unlock()

		for conn := range s.eventClients {
			_ = conn.Close()
			delete(s.eventClients, conn)
		}
	}()

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = commandListener.Close()
			_ = eventListener.Close()
		case <-done:
		}
	}()
	defer close(done)

	// Принимаем подключения Map к каналу событий.
	go func() {
		for {
			conn, err := eventListener.Accept()
			if err != nil {
				return
			}

			// Блокировки берутся в одном порядке:
			// сначала mu, затем eventMu.
			s.mu.Lock()
			s.eventMu.Lock()

			s.eventClients[conn] = struct{}{}

			_ = conn.SetWriteDeadline(time.Now().Add(time.Second))
			err = protocol.WriteResponse(conn, protocol.Response{
				OK:      true,
				Message: "subscribed",
			})

			if err != nil {
				delete(s.eventClients, conn)
				_ = conn.Close()
			}

			s.eventMu.Unlock()
			s.mu.Unlock()
		}
	}()

	// Read slow/partial requests on separate goroutines while keeping all
	// SDL backend operations on this event-loop goroutine.
	requests := make(chan receivedRequest)
	go func() {
		for {
			conn, err := commandListener.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
				req, err := protocol.ReadRequest(bufio.NewReader(conn))
				select {
				case requests <- receivedRequest{conn: conn, req: req, err: err}:
				case <-ctx.Done():
					_ = conn.Close()
				case <-s.stop:
					_ = conn.Close()
				}
			}(conn)
		}
	}()

	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-s.stop:
			return nil
		case <-ticker.C:
			if err := s.pollWindowEvents(); err != nil {
				return fmt.Errorf("poll window events: %w", err)
			}
		case received := <-requests:
			_ = s.handleRequest(received.conn, received.req, received.err)
		}
	}
}

func (s *Server) pollWindowEvents() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.window == nil {
		return nil
	}

	events, err := s.window.PollEvents()
	if err != nil {
		return err
	}

	for _, event := range events {
		if event != "window_close" {
			continue
		}

		id := s.windowID
		closeErr := s.window.Close()

		s.window = nil
		s.windowID = ""

		// Сообщаем Map о закрытии окна.
		s.broadcastEvent("window_close", id)

		if closeErr != nil {
			return closeErr
		}

		break
	}

	return nil
}

func (s *Server) handleRequest(conn net.Conn, req protocol.Request, readErr error) error {
	defer conn.Close()
	if readErr != nil {
		_ = protocol.WriteResponse(conn, protocol.Response{OK: false, Message: readErr.Error()})
		return readErr
	}

	cmd := strings.ToUpper(strings.TrimSpace(req.Command))

	s.mu.Lock()
	defer s.mu.Unlock()

	switch cmd {
	case "CREATE":
		if s.window != nil {
			return protocol.WriteResponse(conn, protocol.Response{
				OK:      false,
				Message: "window already exists",
			})
		}

		cfg := backend.WindowConfig{
			ID:     req.WindowID,
			Title:  req.Title,
			Width:  req.Width,
			Height: req.Height,
		}

		if cfg.ID == "" {
			cfg.ID = "map"
		}
		if cfg.Title == "" {
			cfg.Title = "Tactical Map"
		}
		if cfg.Width == 0 {
			cfg.Width = 960
		}
		if cfg.Height == 0 {
			cfg.Height = 640
		}

		w, err := s.Backend.Create(cfg)
		if err != nil {
			_ = protocol.WriteResponse(conn, protocol.Response{
				OK:      false,
				Message: err.Error(),
			})
			return err
		}

		s.window = w
		s.windowID = cfg.ID

		return protocol.WriteResponse(conn, protocol.Response{
			OK:       true,
			Message:  "created",
			WindowID: cfg.ID,
		})

	case "SHOW":
		if s.window == nil {
			return protocol.WriteResponse(conn, protocol.Response{
				OK:      false,
				Message: "no window; send CREATE first",
			})
		}

		if err := s.window.Show(); err != nil {
			return protocol.WriteResponse(conn, protocol.Response{
				OK:      false,
				Message: err.Error(),
			})
		}

		return protocol.WriteResponse(conn, protocol.Response{
			OK:      true,
			Message: "shown",
		})

	case "CLOSE":
		if s.window == nil {
			return protocol.WriteResponse(conn, protocol.Response{
				OK:      false,
				Message: "no window; send CREATE first",
			})
		}

		closeErr := s.window.Close()
		s.window = nil
		s.windowID = ""

		if closeErr != nil {
			return protocol.WriteResponse(conn, protocol.Response{
				OK:      false,
				Message: closeErr.Error(),
			})
		}

		return protocol.WriteResponse(conn, protocol.Response{
			OK:      true,
			Message: "closed",
		})

	case "QUIT":
		if s.window != nil {
			closeErr := s.window.Close()
			s.window = nil
			s.windowID = ""

			if closeErr != nil {
				_ = protocol.WriteResponse(conn, protocol.Response{
					OK:      false,
					Message: closeErr.Error(),
				})
				return closeErr
			}
		}

		if err := protocol.WriteResponse(conn, protocol.Response{
			OK:      true,
			Message: "stopping",
		}); err != nil {
			return err
		}

		s.stopOnce.Do(func() {
			close(s.stop)
		})

		return nil

	default:
		return protocol.WriteResponse(conn, protocol.Response{
			OK:      false,
			Message: "unknown command: " + cmd,
		})
	}
}
