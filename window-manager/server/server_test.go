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
