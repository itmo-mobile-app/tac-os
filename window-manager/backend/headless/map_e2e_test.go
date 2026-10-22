package headless

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/itmo-mobile-app/tac-os/window-manager/backend"
	"github.com/itmo-mobile-app/tac-os/window-manager/server"
)

// The recorder delegates every operation to the production headless window.
// Neither JSON decoding, socket responses, rendering nor lifecycle are mocked.
type observedBackend struct {
	*Backend
	mu    sync.Mutex
	calls []string
}

func (b *observedBackend) record(name string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, name)
}

func (b *observedBackend) sequence() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.calls...)
}

func (b *observedBackend) Create(cfg backend.WindowConfig) (backend.Window, error) {
	w, err := b.Backend.Create(cfg)
	if err != nil {
		return nil, err
	}
	b.record("CREATE")
	return &observedWindow{Window: w, DrawingWindow: w.(backend.DrawingWindow), record: b.record}, nil
}

type observedWindow struct {
	backend.Window
	backend.DrawingWindow
	record func(string)
}

func (w *observedWindow) Show() error {
	if err := w.Window.Show(); err != nil {
		return err
	}
	w.record("SHOW")
	return nil
}

func (w *observedWindow) Close() error {
	if err := w.Window.Close(); err != nil {
		return err
	}
	w.record("CLOSE")
	return nil
}

func (w *observedWindow) BeginFrame(c backend.Color) error {
	if err := w.DrawingWindow.BeginFrame(c); err != nil {
		return err
	}
	w.record("BEGIN_FRAME")
	return nil
}

func (w *observedWindow) DrawImage(path string, x, y, width, height int) error {
	if err := w.DrawingWindow.DrawImage(path, x, y, width, height); err != nil {
		return err
	}
	w.record("DRAW_IMAGE")
	return nil
}

func (w *observedWindow) DrawCircle(x, y, radius int, c backend.Color) error {
	if err := w.DrawingWindow.DrawCircle(x, y, radius, c); err != nil {
		return err
	}
	w.record("DRAW_CIRCLE")
	return nil
}

func (w *observedWindow) Present() error {
	if err := w.DrawingWindow.Present(); err != nil {
		return err
	}
	w.record("PRESENT")
	return nil
}

func TestMapClientWindowManagerE2E(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	// Keep socket paths below macOS's Unix-domain socket length limit.
	socketDir, err := os.MkdirTemp("/tmp", "tac-map-e2e-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	commandSocket := filepath.Join(socketDir, "commands.sock")
	eventSocket := filepath.Join(socketDir, "events.sock")

	buildCtx, cancelBuild := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancelBuild()
	binary := filepath.Join(t.TempDir(), "map-demo")
	goBinary := filepath.Join(runtime.GOROOT(), "bin", "go")
	build := exec.CommandContext(buildCtx, goBinary, "build", "-buildvcs=false", "-o", binary, "./cmd/map-demo")
	build.Dir = filepath.Join(root, "app", "map")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build real Map CLI: %v\n%s", err, output)
	}

	b := &observedBackend{Backend: New()}
	wm := &server.Server{SocketPath: commandSocket, EventSocketPath: eventSocket, Backend: b}
	ctx, cancel := context.WithCancel(context.Background())
	wmDone := make(chan error, 1)
	go func() { wmDone <- wm.Run(ctx) }()
	wmStopped := false
	stopWM := func() {
		if wmStopped {
			return
		}
		wmStopped = true
		cancel()
		select {
		case err := <-wmDone:
			if err != nil {
				t.Errorf("WM shutdown: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("WM did not shut down")
		}
	}
	defer stopWM()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(eventSocket); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("WM event socket did not become ready")
		}
		time.Sleep(5 * time.Millisecond)
	}

	var logs bytes.Buffer
	mapProcess := exec.Command(binary, "-config", filepath.Join(root, "app/map/assets/test-map.json"), "-socket", commandSocket, "-events", eventSocket)
	mapProcess.Stdout, mapProcess.Stderr = &logs, &logs
	if err := mapProcess.Start(); err != nil {
		t.Fatal(err)
	}
	mapDone := make(chan error, 1)
	go func() { mapDone <- mapProcess.Wait() }()
	mapStopped := false
	defer func() {
		if !mapStopped {
			_ = mapProcess.Process.Kill()
			<-mapDone
		}
	}()
	want := []string{"CREATE", "BEGIN_FRAME", "DRAW_IMAGE", "DRAW_CIRCLE", "DRAW_CIRCLE", "DRAW_CIRCLE", "DRAW_CIRCLE", "DRAW_CIRCLE", "DRAW_CIRCLE", "PRESENT", "SHOW"}
	deadline = time.Now().Add(10 * time.Second)
	for !reflect.DeepEqual(b.sequence(), want) {
		select {
		case err := <-mapDone:
			mapStopped = true
			t.Fatalf("Map exited before SHOW: %v\n%s", err, logs.String())
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("incomplete command sequence: %v", b.sequence())
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Inspect the actual production front buffer after PRESENT and SHOW.
	b.Backend.mu.Lock()
	w := b.Backend.w
	w.mu.Lock()
	if !w.visible || w.closed || w.front.Bounds() != image.Rect(0, 0, 960, 640) {
		t.Error("expected a visible 960x640 window")
	}
	points := []image.Point{{X: 240, Y: 160}, {X: 480, Y: 320}, {X: 720, Y: 480}}
	// Map's alpha bytes 236,169,102 over the white marker interior.
	wantColors := []color.RGBA{{R: 200, G: 85, B: 63, A: 255}, {R: 216, G: 133, B: 117, A: 255}, {R: 231, G: 182, B: 172, A: 255}}
	for i, point := range points {
		got := w.front.RGBAAt(point.X, point.Y)
		if !nearColor(got, wantColors[i]) {
			t.Errorf("marker %d at %v: got %v, want %v (tolerance 1)", i+1, point, got, wantColors[i])
		}
		if got := w.front.RGBAAt(point.X+12, point.Y); got != (color.RGBA{R: 255, G: 255, B: 255, A: 255}) {
			t.Errorf("marker %d white border: %v", i+1, got)
		}
	}
	backgroundFile, err := os.Open(filepath.Join(root, "app/map/assets/test-map.png"))
	if err != nil {
		w.mu.Unlock()
		b.Backend.mu.Unlock()
		t.Fatal(err)
	}
	background, err := png.Decode(backgroundFile)
	_ = backgroundFile.Close()
	if err != nil {
		w.mu.Unlock()
		b.Backend.mu.Unlock()
		t.Fatal(err)
	}
	for _, point := range []image.Point{{X: 100, Y: 100}, {X: 800, Y: 320}} {
		if got, expected := w.front.RGBAAt(point.X, point.Y), color.RGBAModel.Convert(background.At(point.X, point.Y)).(color.RGBA); got != expected {
			t.Errorf("background at %v: got %v, want %v", point, got, expected)
		}
	}
	w.mu.Unlock()
	b.Backend.mu.Unlock()

	// SIGINT exercises the production Map cancellation path and real CLOSE command.
	if err := mapProcess.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-mapDone:
		mapStopped = true
		if err != nil {
			t.Fatalf("Map shutdown: %v\n%s", err, logs.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Map did not exit after SIGINT")
	}
	want = append(want, "CLOSE")
	if got := b.sequence(); !reflect.DeepEqual(got, want) {
		t.Errorf("command sequence: got %v, want %v", got, want)
	}
	w.mu.Lock()
	if !w.closed || w.visible {
		t.Error("WM did not close the Map window")
	}
	w.mu.Unlock()
	if !strings.Contains(logs.String(), "3 visible, 1 outside map") {
		t.Errorf("unexpected Map output:\n%s", logs.String())
	}
	select {
	case err := <-wmDone:
		wmStopped = true
		t.Fatalf("Map unexpectedly stopped WM: %v", err)
	default:
	}
	stopWM()
	for _, path := range []string{commandSocket, eventSocket} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("WM left socket behind: %s", path)
		}
	}
	t.Log("real Map CLI -> Unix sockets -> production WM/headless: frame, three markers, opacity and CLOSE verified")
}

func nearColor(a, b color.RGBA) bool {
	for _, pair := range [][2]uint8{{a.R, b.R}, {a.G, b.G}, {a.B, b.B}, {a.A, b.A}} {
		difference := int(pair[0]) - int(pair[1])
		if difference < -1 || difference > 1 {
			return false
		}
	}
	return true
}
