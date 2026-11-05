package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const scenarioTimeout = 15 * time.Second

type suite struct {
	processes []*process
	sockets   []string
}

func (s *suite) start(name, binary string, args ...string) (*process, error) {
	p, err := startProcess(name, binary, args...)
	if err == nil {
		s.processes = append(s.processes, p)
	}
	return p, err
}

func (s *suite) cleanup() error {
	var result error
	for i := len(s.processes) - 1; i >= 0; i-- {
		result = errors.Join(result, s.processes[i].stop())
	}
	return errors.Join(result, checkAbsent(s.sockets...))
}

func (s *suite) diagnostics() string {
	var b strings.Builder
	for _, p := range s.processes {
		fmt.Fprintf(&b, "\n--- %s ---\n%s", p.name, p.logs.String())
	}
	return b.String()
}

func startBroker(ctx context.Context, s *suite, binary string) (*process, string, error) {
	for attempt := 0; attempt < 5; attempt++ {
		// Broker rejects port 0. Release an ephemeral wildcard reservation just
		// before launch and retry only a confirmed bind collision.
		listener, err := net.Listen("tcp4", "0.0.0.0:0")
		if err != nil {
			return nil, "", err
		}
		port := listener.Addr().(*net.TCPAddr).Port
		if err := listener.Close(); err != nil {
			return nil, "", err
		}
		p, err := s.start("Broker", binary, strconv.Itoa(port))
		if err != nil {
			return nil, "", err
		}
		readyCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err = waitFor(readyCtx, "Broker startup", []*process{p}, func() (bool, error) {
			return strings.Contains(p.logs.String(), "Broker listening on "), nil
		})
		cancel()
		if err == nil {
			return p, net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), nil
		}
		select {
		case <-p.done:
			if strings.Contains(p.logs.String(), "bind: address already in use") {
				s.processes = s.processes[:len(s.processes)-1]
				continue
			}
		default:
		}
		return nil, "", err
	}
	return nil, "", errors.New("Broker port collided on all five attempts")
}

func positiveScenario(parent context.Context, root, dir string, b binaries, output io.Writer) (result error) {
	ctx, cancel := context.WithTimeout(parent, scenarioTimeout)
	defer cancel()
	s := &suite{}
	defer func() {
		result = errors.Join(result, s.cleanup())
		if result != nil {
			result = fmt.Errorf("positive scenario: %w%s", result, s.diagnostics())
		}
	}()
	broker, address, err := startBroker(ctx, s, b.broker)
	if err != nil {
		return err
	}
	target, err := s.start("Target Service", b.target, "-broker", address)
	if err != nil {
		return err
	}
	if err := waitFor(ctx, "Target Service subscription", s.processes, func() (bool, error) {
		return strings.Contains(target.logs.String(), "subscribed to observations"), nil
	}); err != nil {
		return err
	}
	conn, err := net.DialTimeout("tcp", address, time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	observer := filepath.Base(dir)
	timestamp := time.Now().UnixMilli()
	payload, err := json.Marshal(map[string]any{
		"targetId": "17", "observerId": observer,
		"latitude": 59.9500, "longitude": 30.3110,
		"confidence": 0.8, "timestamp": timestamp,
	})
	if err != nil {
		return err
	}
	want := fmt.Sprintf("observation stored targetId=%q observerId=%q timestamp=%d ", "17", observer, timestamp)
	if err := waitFor(ctx, "Observation delivery", s.processes, func() (bool, error) {
		if strings.Contains(target.logs.String(), want) {
			return true, nil
		}
		if err := conn.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
			return false, err
		}
		_, err := fmt.Fprintf(conn, "PUB observations %s\n", payload)
		return false, err
	}); err != nil {
		return err
	}
	if err := conn.Close(); err != nil {
		return err
	}
	fmt.Fprintln(output, "PASS Broker -> Target Service (matching Observation receipt)")
	commandSocket, eventSocket := filepath.Join(dir, "wm.sock"), filepath.Join(dir, "events.sock")
	s.sockets = []string{commandSocket, eventSocket}
	wm, err := s.start("Window Manager", b.wm, "-socket", commandSocket, "-events", eventSocket)
	if err != nil {
		return err
	}
	if err := waitFor(ctx, "WM sockets", s.processes, func() (bool, error) {
		for _, path := range s.sockets {
			c, err := net.DialTimeout("unix", path, 100*time.Millisecond)
			if err != nil {
				return false, nil
			}
			_ = c.Close()
		}
		return true, nil
	}); err != nil {
		return err
	}
	m, err := s.start("Map", b.tacticalMap, "-config", filepath.Join(root, "app/map/assets/test-map.json"), "-socket", commandSocket, "-events", eventSocket)
	if err != nil {
		return err
	}
	if err := waitFor(ctx, "Map frame", s.processes, func() (bool, error) {
		logs := m.logs.String()
		for _, text := range []string{"Map shown.", "3 visible, 1 outside map", "probability 90%", "probability 55%", "probability 20%"} {
			if !strings.Contains(logs, text) {
				return false, nil
			}
		}
		return true, nil
	}); err != nil {
		return err
	}
	if err := m.signal(syscall.SIGINT); err != nil {
		return err
	}
	stopCtx, stopCancel := context.WithTimeout(ctx, stopTimeout)
	err = m.wait(stopCtx)
	stopCancel()
	if err != nil {
		return fmt.Errorf("Map graceful shutdown: %w", err)
	}
	// Observe CLOSE through the wire, without inspecting WM internals.
	if err := wmCommand(commandSocket, "SHOW", false); err != nil {
		return fmt.Errorf("Map must release its window: %w", err)
	}
	for _, command := range []string{"CREATE", "CLOSE"} {
		if err := wmCommand(commandSocket, command, true); err != nil {
			return fmt.Errorf("WM reuse after Map exit: %w", err)
		}
	}
	for _, p := range []*process{broker, target, wm} {
		if err := p.alive(); err != nil {
			return err
		}
	}
	fmt.Fprintln(output, "PASS Map -> headless Window Manager (frame, markers, CLOSE, reuse)")
	// QUIT releases the backend and lets WM remove its sockets normally.
	if err := wmCommand(commandSocket, "QUIT", true); err != nil {
		return err
	}
	stopCtx, stopCancel = context.WithTimeout(ctx, stopTimeout)
	err = wm.wait(stopCtx)
	stopCancel()
	if err != nil {
		return fmt.Errorf("WM shutdown: %w", err)
	}
	return nil
}

func wmCommand(path, command string, wantOK bool) error {
	conn, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
		return err
	}
	if err := json.NewEncoder(conn).Encode(map[string]string{"command": command, "window_id": "map"}); err != nil {
		return err
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return err
	}
	var response struct {
		OK      *bool  `json:"ok"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(line, &response); err != nil {
		return err
	}
	if response.OK == nil || *response.OK != wantOK {
		return fmt.Errorf("%s: unexpected response %s", command, line)
	}
	if !wantOK && response.Message != "no window; send CREATE first" {
		return fmt.Errorf("%s: expected no-window error, got %s", command, line)
	}
	return nil
}

func unavailableBrokerScenario(parent context.Context, targetBinary string) (result error) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	// Reserve a TCP port without listening: connections are refused, while
	// another parallel run cannot acquire the same address in the meantime.
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
	if err != nil {
		return err
	}
	syscall.CloseOnExec(fd)
	defer syscall.Close(fd)
	if err := syscall.Bind(fd, &syscall.SockaddrInet4{Addr: [4]byte{127, 0, 0, 1}}); err != nil {
		return err
	}
	address, err := syscall.Getsockname(fd)
	if err != nil {
		return err
	}
	port := address.(*syscall.SockaddrInet4).Port
	p, err := startProcess("Target Service without Broker", targetBinary, "-broker", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, p.stop()) }()
	err = p.wait(ctx)
	var exitErr interface{ ExitCode() int }
	if !errors.As(err, &exitErr) || exitErr.ExitCode() <= 0 || !strings.Contains(p.logs.String(), "connect to Broker") {
		return fmt.Errorf("expected failed Broker connection, got %v\n%s", err, p.logs.String())
	}
	return nil
}
