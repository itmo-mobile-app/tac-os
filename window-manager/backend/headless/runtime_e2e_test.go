package headless

import (
	"context"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/itmo-mobile-app/tac-os/window-manager/backend"
	"github.com/itmo-mobile-app/tac-os/window-manager/server"
)

// runtimeEventBackend uses the production headless renderer but lets the test
// inject the same window_close event that a graphical backend emits on user close.
type runtimeEventBackend struct {
	*observedBackend
	closeRequests chan struct{}
}

func (b *runtimeEventBackend) Create(cfg backend.WindowConfig) (backend.Window, error) {
	w, err := b.observedBackend.Create(cfg)
	if err != nil {
		return nil, err
	}

	return &runtimeEventWindow{
		observedWindow: w.(*observedWindow),
		closeRequests:  b.closeRequests,
	}, nil
}

type runtimeEventWindow struct {
	*observedWindow
	closeRequests <-chan struct{}
}

func (w *runtimeEventWindow) PollEvents() ([]string, error) {
	select {
	case <-w.closeRequests:
		return []string{"window_close"}, nil
	default:
		return w.observedWindow.Window.PollEvents()
	}
}

func startRuntimeRunner(
	t *testing.T,
	root, runner, bytecode, commandSocket, eventSocket string,
) (*mapBrokerLog, <-chan error, context.CancelFunc) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	cmd := exec.CommandContext(ctx, runner, bytecode, commandSocket, eventSocket)
	cmd.Dir = root

	logs := &mapBrokerLog{}
	cmd.Stdout, cmd.Stderr = logs, logs

	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start Runtime test runner: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	return logs, done, cancel
}

func waitRuntimeSequence(
	t *testing.T,
	backend *observedBackend,
	done <-chan error,
	minimumCalls int,
) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)

	for len(backend.sequence()) < minimumCalls {
		select {
		case err := <-done:
			t.Fatalf("Runtime exited before issuing %d WM calls: %v", minimumCalls, err)
		default:
		}

		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for WM calls; got %v", backend.sequence())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitRuntimeExit(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Runtime scenario failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Runtime scenario did not exit")
	}
}

func assertRuntimeFrame(t *testing.T, backend *observedBackend, root string) {
	t.Helper()

	backend.Backend.mu.Lock()
	w := backend.Backend.w
	backend.Backend.mu.Unlock()
	if w == nil {
		t.Fatal("Runtime did not create a headless window")
	}

	file, err := os.Open(filepath.Join(root, "app/map/assets/test-map.png"))
	if err != nil {
		t.Fatal(err)
	}
	background, err := png.Decode(file)
	_ = file.Close()
	if err != nil {
		t.Fatal(err)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.visible || w.closed {
		t.Fatal("expected a visible, open window after SHOW")
	}
	if w.front.Bounds().Dx() != 960 || w.front.Bounds().Dy() != 640 {
		t.Fatalf("unexpected frame size: %v", w.front.Bounds())
	}

	if got, want := w.front.RGBAAt(480, 320), (color.RGBA{R: 255, A: 255}); got != want {
		t.Fatalf("Target pixel: got %v, want %v", got, want)
	}

	sourceBackground := color.RGBAModel.Convert(background.At(100, 100)).(color.RGBA)
	wantBackground := color.RGBA{
		R: sourceBackground.R, G: sourceBackground.G,
		B: sourceBackground.B, A: 255,
	}
	if got := w.front.RGBAAt(100, 100); got != wantBackground {
		t.Fatalf("background pixel: got %v, want %v", got, wantBackground)
	}
}

func assertRuntimeWindowClosed(t *testing.T, backend *observedBackend) {
	t.Helper()

	backend.Backend.mu.Lock()
	w := backend.Backend.w
	backend.Backend.mu.Unlock()
	if w == nil {
		t.Fatal("headless window disappeared before close could be checked")
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.closed || w.visible {
		t.Fatal("Window Manager did not close the window")
	}
}

func TestRuntimeWindowManagerE2E(t *testing.T) {
	runner := os.Getenv("TACOS_RUNTIME_E2E_RUNNER")
	bytecode := os.Getenv("TACOS_RUNTIME_E2E_BYTECODE")
	if runner == "" && bytecode == "" {
		t.Skip("Run map_runtime_wm_e2e through CTest to provide runner and bytecode")
	}
	if runner == "" || bytecode == "" {
		t.Fatal("Both TACOS_RUNTIME_E2E_RUNNER and TACOS_RUNTIME_E2E_BYTECODE must be set")
	}

	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}

	socketDir, err := os.MkdirTemp("/tmp", "tac-runtime-wm-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })

	commandSocket := filepath.Join(socketDir, "commands.sock")
	eventSocket := filepath.Join(socketDir, "events.sock")

	recorder := &observedBackend{Backend: New()}
	b := &runtimeEventBackend{
		observedBackend: recorder,
		closeRequests:   make(chan struct{}, 1),
	}

	wm := &server.Server{
		SocketPath:      commandSocket,
		EventSocketPath: eventSocket,
		Backend:         b,
	}
	ctx, cancelWM := context.WithCancel(context.Background())
	wmDone := make(chan error, 1)
	go func() { wmDone <- wm.Run(ctx) }()

	wmStopped := false
	stopWM := func() {
		if wmStopped {
			return
		}
		wmStopped = true
		cancelWM()
		select {
		case err := <-wmDone:
			if err != nil {
				t.Errorf("Window Manager shutdown: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("Window Manager did not stop")
		}
	}
	defer stopWM()

	waitMapBroker(t, "Window Manager sockets", func() bool {
		_, commandErr := os.Stat(commandSocket)
		_, eventErr := os.Stat(eventSocket)
		return commandErr == nil && eventErr == nil
	})

	// First launch: inject a user-close event and verify Runtime receives it.
	logs1, done1, cancel1 := startRuntimeRunner(
		t, root, runner, bytecode, commandSocket, eventSocket,
	)
	defer cancel1()
	waitRuntimeSequence(t, recorder, done1, 6) // through SHOW
	assertRuntimeFrame(t, recorder, root)

	b.closeRequests <- struct{}{}
	waitRuntimeExit(t, done1)
	if !strings.Contains(logs1.String(), "window-close-event-ok") {
		t.Fatalf("Runtime did not receive window_close event:\n%s", logs1.String())
	}
	if !strings.Contains(logs1.String(), "window-close-called") {
		t.Fatalf("Runtime did not call window.close:\n%s", logs1.String())
	}
	assertRuntimeWindowClosed(t, recorder)

	// Second launch: no user event; window.pollEvent times out and window.close
	// sends an explicit CLOSE command. This also proves stale state was cleared.
	logs2, done2, cancel2 := startRuntimeRunner(
		t, root, runner, bytecode, commandSocket, eventSocket,
	)
	defer cancel2()
	waitRuntimeSequence(t, recorder, done2, 13) // second CREATE ... SHOW
	assertRuntimeFrame(t, recorder, root)
	waitRuntimeExit(t, done2)

	if strings.Contains(logs2.String(), "window-close-event-ok") {
		t.Fatalf("second launch unexpectedly received a close event:\n%s", logs2.String())
	}
	if !strings.Contains(logs2.String(), "window-close-called") {
		t.Fatalf("second launch did not call window.close:\n%s", logs2.String())
	}
	assertRuntimeWindowClosed(t, recorder)

	want := []string{
		"CREATE", "BEGIN_FRAME", "DRAW_IMAGE", "DRAW_CIRCLE", "PRESENT", "SHOW", "CLOSE",
		"CREATE", "BEGIN_FRAME", "DRAW_IMAGE", "DRAW_CIRCLE", "PRESENT", "SHOW", "CLOSE",
	}
	if got := recorder.sequence(); !reflect.DeepEqual(got, want) {
		t.Fatalf("WM call sequence:\n got %v\nwant %v", got, want)
	}

	stopWM()
	for _, path := range []string{commandSocket, eventSocket} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("Window Manager left socket behind: %s", path)
		}
	}
	t.Log("real .tc -> VM -> Runtime -> WM/headless: pixels, close event, explicit close and second launch verified")
}
