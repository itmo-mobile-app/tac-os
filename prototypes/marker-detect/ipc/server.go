package ipc

import (
	"bufio"
	"errors"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

// writeTimeout bounds a response write, so that a client that stopped reading cannot block the server.
const writeTimeout = 5 * time.Second

// Server answers DETECT requests on stream connections. Every connection carries any number of request and
// response pairs, one request at a time. Requests from all connections are served one by one, because there is
// one camera.
type Server struct {
	// ErrorLog receives the reasons of error responses and of dropped connections. Nil disables logging.
	ErrorLog *log.Logger

	source   Source
	detectMu sync.Mutex // one DETECT at a time

	mu       sync.Mutex // guards the fields below
	listener net.Listener
	conns    map[net.Conn]struct{}
	closed   bool
	wg       sync.WaitGroup // connection handlers
}

// NewServer creates a server that takes detections from source.
func NewServer(source Source) *Server {
	return &Server{source: source, conns: make(map[net.Conn]struct{})}
}

// Serve accepts connections on l until Close is called, and then returns nil. It closes l on return.
func (s *Server) Serve(l net.Listener) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		l.Close()
		return nil
	}
	s.listener = l
	s.mu.Unlock()
	defer l.Close()

	for {
		c, err := l.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}

		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			c.Close()
			return nil
		}
		s.conns[c] = struct{}{}
		s.wg.Add(1)
		s.mu.Unlock()

		go s.handle(c)
	}
}

// Close stops accepting connections, closes the open ones and waits for the handlers to finish. A handler that
// is inside Source.Detect is waited for until Detect returns. Close can be called more than once.
func (s *Server) Close() error {
	s.mu.Lock()
	var err error
	if !s.closed {
		s.closed = true
		if s.listener != nil {
			err = s.listener.Close()
		}
		for c := range s.conns {
			c.Close()
		}
	}
	s.mu.Unlock()

	s.wg.Wait()
	if errors.Is(err, net.ErrClosed) {
		err = nil
	}
	return err
}

func (s *Server) logf(format string, args ...any) {
	if s.ErrorLog != nil {
		s.ErrorLog.Printf(format, args...)
	}
}

func (s *Server) handle(c net.Conn) {
	defer func() {
		c.Close()
		s.mu.Lock()
		delete(s.conns, c)
		s.mu.Unlock()
		s.wg.Done()
	}()

	// The buffer holds the largest allowed frame, so a longer line makes ReadSlice return ErrBufferFull.
	r := bufio.NewReaderSize(c, maxFrameBytes)
	for {
		line, err := r.ReadSlice('\n')
		switch {
		case err == nil:
		case errors.Is(err, bufio.ErrBufferFull):
			// The rest of the line is unknown, so the stream cannot be resynchronized: answer and hang up.
			s.logf("request is longer than %d bytes", maxFrameBytes)
			s.write(c, encodeFailure(codeBadRequest))
			return
		case errors.Is(err, io.EOF) && len(line) > 0:
			s.logf("the connection was closed in the middle of a request")
			s.write(c, encodeFailure(codeBadRequest))
			return
		default:
			// A closed connection (by the client or by Close) is the normal end.
			return
		}

		if perr := parseRequest(line); perr != nil {
			s.logf("bad request: %v", perr)
			if !s.write(c, encodeFailure(codeBadRequest)) {
				return
			}
			continue
		}
		if !s.write(c, s.detect()) {
			return
		}
	}
}

// detect runs one DETECT request and returns the response line.
func (s *Server) detect() []byte {
	s.detectMu.Lock()
	dets, err := s.source.Detect()
	s.detectMu.Unlock()

	switch {
	case errors.Is(err, ErrCameraUnavailable):
		s.logf("%v", err)
		return encodeFailure(codeCameraUnavailable)
	case err != nil:
		s.logf("detection failed: %v", err)
		return encodeFailure(codeDetectionFailed)
	}
	if err := validate(dets); err != nil {
		s.logf("detection failed: the detector produced an invalid result: %v", err)
		return encodeFailure(codeDetectionFailed)
	}
	return encodeSuccess(dets)
}

// write sends one response line and reports whether the connection is still usable.
func (s *Server) write(c net.Conn, line []byte) bool {
	c.SetWriteDeadline(time.Now().Add(writeTimeout))
	if _, err := c.Write(line); err != nil {
		s.logf("cannot write the response: %v", err)
		return false
	}
	return true
}
