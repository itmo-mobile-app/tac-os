package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/itmo-mobile-app/tac-os/window-manager/backend"
	"github.com/itmo-mobile-app/tac-os/window-manager/protocol"
)

type testWindow struct {
	closeEvents chan struct{}
	frames      int
	images      int
	circles     int
	presents    int
}

func (w *testWindow) Show() error  { return nil }
func (w *testWindow) Close() error { return nil }

func (w *testWindow) PollEvents() ([]string, error) {
	select {
	case <-w.closeEvents:
		return []string{"window_close"}, nil
	default:
		return nil, nil
	}
}

func (w *testWindow) BeginFrame(_ backend.Color) error {
	w.frames++
	return nil
}

func (w *testWindow) DrawImage(_ string, _, _, _, _ int) error {
	w.images++
	return nil
}

func (w *testWindow) DrawCircle(_, _, _ int, _ backend.Color) error {
	w.circles++
	return nil
}

func (w *testWindow) Present() error {
	w.presents++
	return nil
}

var _ backend.DrawingWindow = (*testWindow)(nil)

type testBackend struct{ window *testWindow }

func (b *testBackend) Create(_ backend.WindowConfig) (backend.Window, error) {
	return b.window, nil
}

func dialSocket(t *testing.T, socket string) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("unix", socket, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func command(t *testing.T, socket, name string) protocol.Response {
	t.Helper()
	conn := dialSocket(t, socket)
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if err := json.NewEncoder(conn).Encode(protocol.Request{Command: name, WindowID: "map"}); err != nil {
		t.Fatal(err)
	}
	var response protocol.Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		t.Fatal(err)
	}
	return response
}
func commandRequest(t *testing.T, socket string, req protocol.Request) protocol.Response {
	t.Helper()

	conn := dialSocket(t, socket)
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		t.Fatal(err)
	}

	var response protocol.Response
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		t.Fatal(err)
	}

	return response
}

func TestCommandsAndEventsRemainResponsiveToPartialClients(t *testing.T) {
	dir := t.TempDir()
	socket := filepath.Join(dir, "commands.sock")
	events := filepath.Join(dir, "events.sock")
	w := &testWindow{closeEvents: make(chan struct{}, 1)}
	wm := &Server{SocketPath: socket, EventSocketPath: events, Backend: &testBackend{window: w}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- wm.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("server exited: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Error("server did not shut down")
		}
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(events); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("event socket did not appear")
		}
		time.Sleep(time.Millisecond)
	}

	eventConn := dialSocket(t, events)
	_ = eventConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	decoder := json.NewDecoder(eventConn)
	var subscribed protocol.Response
	if err := decoder.Decode(&subscribed); err != nil {
		t.Fatal(err)
	}
	if !subscribed.OK {
		t.Fatalf("event subscription failed: %+v", subscribed)
	}
	for _, name := range []string{"CREATE", "SHOW", "CLOSE", "CREATE", "SHOW"} {
		if response := command(t, socket, name); !response.OK {
			t.Fatalf("%s failed: %+v", name, response)
		}
	}

	stalled := dialSocket(t, socket)
	if _, err := fmt.Fprint(stalled, "{\"command\":"); err != nil {
		t.Fatal(err)
	}
	w.closeEvents <- struct{}{}
	var event protocol.Response
	_ = eventConn.SetReadDeadline(time.Now().Add(time.Second))
	if err := decoder.Decode(&event); err != nil {
		t.Fatalf("close event blocked by incomplete client: %v", err)
	}
	if event.Event != "window_close" || event.WindowID != "map" {
		t.Fatalf("unexpected window event: %+v", event)
	}
}
func TestDrawingCommands(t *testing.T) {
	dir := t.TempDir()
	socket := filepath.Join(dir, "drawing.sock")
	events := filepath.Join(dir, "events.sock")

	w := &testWindow{closeEvents: make(chan struct{}, 1)}
	wm := &Server{
		SocketPath:      socket,
		EventSocketPath: events,
		Backend:         &testBackend{window: w},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- wm.Run(ctx)
	}()

	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("server exited: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Error("server did not shut down")
		}
	})

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(socket); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("command socket did not appear")
		}
		time.Sleep(time.Millisecond)
	}
	createResp := commandRequest(t, socket, protocol.Request{
		Command:  "CREATE",
		WindowID: "map",
		Width:    640,
		Height:   480,
		Title:    "Test Map",
	})
	if !createResp.OK {
		t.Fatalf("CREATE failed: %+v", createResp)
	}
	color := &protocol.Color{R: 20, G: 30, B: 40, A: 255}
	x, y := 100, 120

	requests := []protocol.Request{
		{Command: "BEGIN_FRAME", WindowID: "map", Color: color},
		{
			Command: "DRAW_IMAGE", WindowID: "map",
			ImagePath: "background.png",
			X:         &x, Y: &y, Width: 100, Height: 80,
		},
		{
			Command: "DRAW_CIRCLE", WindowID: "map",
			X: &x, Y: &y, Radius: 15, Color: color,
		},
		{Command: "PRESENT", WindowID: "map"},
	}

	for _, req := range requests {
		resp := commandRequest(t, socket, req)
		if !resp.OK {
			t.Errorf("%s failed: %+v", req.Command, resp)
		}
	}

	if w.frames != 1 || w.images != 1 || w.circles != 1 || w.presents != 1 {
		t.Errorf(
			"unexpected calls: frames=%d images=%d circles=%d presents=%d",
			w.frames, w.images, w.circles, w.presents,
		)
	}
}

func TestDrawingCommandsRejectInvalidParameters(t *testing.T) {
	dir := t.TempDir()
	socket := filepath.Join(dir, "invalid.sock")
	events := filepath.Join(dir, "events.sock")

	w := &testWindow{closeEvents: make(chan struct{}, 1)}
	wm := &Server{
		SocketPath:      socket,
		EventSocketPath: events,
		Backend:         &testBackend{window: w},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- wm.Run(ctx)
	}()

	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("server exited: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Error("server did not shut down")
		}
	})

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(socket); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("command socket did not appear")
		}
		time.Sleep(time.Millisecond)
	}

	x, y := 10, 20
	requests := []protocol.Request{
		{Command: "BEGIN_FRAME", WindowID: "map"},
		{
			Command: "DRAW_IMAGE", WindowID: "map",
			X: &x, Y: &y, Width: 10, Height: 10,
		},
		{
			Command: "DRAW_CIRCLE", WindowID: "map",
			X: &x, Y: &y, Radius: 0,
			Color: &protocol.Color{R: 255, A: 255},
		},
	}

	for _, req := range requests {
		resp := commandRequest(t, socket, req)
		if resp.OK {
			t.Errorf("%s unexpectedly succeeded", req.Command)
		}
	}
}
