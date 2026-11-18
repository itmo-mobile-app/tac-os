package headless

import (
	"bytes"
	"context"
	"image"
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

// Test infrastructure from #136 only injects a native-close event and records
// operations. Parsing, Runtime, VM, sockets and all drawing stay in production.
func TestTacticalMapTcE2E(t *testing.T) {
	tacc := os.Getenv("TACOS_MAP_TC_TACC")
	runner := os.Getenv("TACOS_MAP_TC_RUNNER")
	bytecode := os.Getenv("TACOS_MAP_TC_BYTECODE")
	if tacc == "" && runner == "" && bytecode == "" {
		t.Skip("Run the required map_tc_window_e2e CTest to provide the real compiler and VM host")
	}
	if tacc == "" || runner == "" || bytecode == "" {
		t.Fatal("TACOS_MAP_TC_TACC, TACOS_MAP_TC_RUNNER and TACOS_MAP_TC_BYTECODE are required")
	}
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(root, "app/map/tc/map.tc"))
	if err != nil {
		t.Fatal(err)
	}
	socketDir, err := os.MkdirTemp("/tmp", "tac-map-tc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(socketDir) })
	commandSocket := filepath.Join(socketDir, "commands.sock")
	eventSocket := filepath.Join(socketDir, "events.sock")
	recorder := &observedBackend{Backend: New()}
	b := &runtimeEventBackend{observedBackend: recorder, closeRequests: make(chan struct{}, 1)}
	wm := &server.Server{SocketPath: commandSocket, EventSocketPath: eventSocket, Backend: b}
	ctx, cancelWM := context.WithCancel(context.Background())
	wmDone := make(chan error, 1)
	go func() { wmDone <- wm.Run(ctx) }()
	stopped := false
	stopWM := func() {
		if stopped {
			return
		}
		stopped = true
		cancelWM()
		select {
		case err := <-wmDone:
			if err != nil {
				t.Errorf("WM shutdown: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("WM did not stop")
		}
	}
	defer stopWM()
	waitMapBroker(t, "WM sockets", func() bool {
		_, a := os.Stat(commandSocket)
		_, b := os.Stat(eventSocket)
		return a == nil && b == nil
	})

	cases := []struct {
		name, latitude, longitude, probability string
		x, y, alpha                            int
		outside, explicit                      bool
	}{
		{name: "center", x: 480, y: 320, alpha: 236},
		{name: "lower-probability", probability: "0.2", x: 480, y: 320, alpha: 102},
		{name: "zero-probability", probability: "0.0", x: 480, y: 320, alpha: 64},
		{name: "fractional", latitude: "59.950001", longitude: "30.310004", x: 321, y: 320, alpha: 236},
		{name: "north-west", latitude: "59.9510", longitude: "30.3080", x: 0, y: 0, alpha: 236},
		{name: "south-east", latitude: "59.9490", longitude: "30.3140", probability: "1.0", x: 959, y: 639, alpha: 255},
		{name: "inside-last-pixel", latitude: "59.949001", longitude: "30.313999", x: 959, y: 639, alpha: 236},
		{name: "north-outside", latitude: "59.9511", outside: true},
		{name: "south-outside", latitude: "59.9489", outside: true},
		{name: "west-outside", longitude: "30.3079", outside: true},
		{name: "east-outside", longitude: "30.3141", outside: true},
		{name: "explicit-close", x: 480, y: 320, alpha: 236, explicit: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			program := bytecode // First case executes the exact built application.
			if tc.name != "center" {
				text := string(source)
				for _, field := range []struct{ before, after string }{
					{"latitude: Float = 59.9500", "latitude: Float = " + tc.latitude},
					{"longitude: Float = 30.3110", "longitude: Float = " + tc.longitude},
					{"probability: Float = 0.9", "probability: Float = " + tc.probability},
				} {
					if strings.HasSuffix(field.after, "= ") {
						continue
					}
					text = replaceMapTcFixture(t, text, field.before, field.after)
				}
				if tc.explicit {
					text = replaceMapTcFixture(t, text, "closeAfterPolls: Int = 0", "closeAfterPolls: Int = 10")
				}
				dir := t.TempDir()
				input := filepath.Join(dir, "map.tc")
				program = filepath.Join(dir, "map.bc")
				if err := os.WriteFile(input, []byte(text), 0600); err != nil {
					t.Fatal(err)
				}
				compileCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if output, err := exec.CommandContext(compileCtx, tacc, input, program).CombinedOutput(); err != nil {
					t.Fatalf("compile actual .tc fixture: %v\n%s", err, output)
				}
			}

			before := len(recorder.sequence())
			want := []string{"CREATE", "BEGIN_FRAME", "DRAW_IMAGE"}
			if !tc.outside {
				want = append(want, "DRAW_CIRCLE", "DRAW_CIRCLE")
			}
			want = append(want, "PRESENT", "SHOW")
			logs, done, cancel := startRuntimeRunner(t, root, runner, program, commandSocket, eventSocket)
			defer cancel()
			waitRuntimeSequence(t, recorder, done, before+len(want))

			recorder.Backend.mu.Lock()
			w := recorder.Backend.w
			recorder.Backend.mu.Unlock()
			w.mu.Lock()
			actual := image.NewRGBA(w.front.Bounds())
			copy(actual.Pix, w.front.Pix)
			visible := w.visible && !w.closed
			w.mu.Unlock()
			if !visible || actual.Bounds() != image.Rect(0, 0, 960, 640) {
				t.Fatal("Map must show one open 960x640 window")
			}

			// The expected integer positions/alpha above are independent fixtures,
			// not a Go reimplementation of the .tc coordinate calculation.
			oracle := New()
			ow, err := oracle.Create(backend.WindowConfig{Width: 960, Height: 640})
			if err != nil {
				t.Fatal(err)
			}
			draw := ow.(backend.DrawingWindow)
			if err := draw.DrawImage(filepath.Join(root, "app/map/assets/test-map.png"), 0, 0, 960, 640); err != nil {
				t.Fatal(err)
			}
			if !tc.outside {
				if err := draw.DrawCircle(tc.x, tc.y, 13, backend.Color{R: 255, G: 255, B: 255, A: 255}); err != nil {
					t.Fatal(err)
				}
				if err := draw.DrawCircle(tc.x, tc.y, 10, backend.Color{R: 196, G: 72, B: 48, A: uint8(tc.alpha)}); err != nil {
					t.Fatal(err)
				}
			}
			if err := draw.Present(); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(actual.Pix, oracle.w.front.Pix) {
				t.Fatalf("WM frame differs: want marker at (%d,%d), alpha %d, outside=%v", tc.x, tc.y, tc.alpha, tc.outside)
			}
			if tc.name == "center" && os.Getenv("TACOS_MAP_TC_CAPTURE") != "" {
				file, err := os.Create(os.Getenv("TACOS_MAP_TC_CAPTURE"))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(file, actual)
				closeErr := file.Close()
				if err != nil || closeErr != nil {
					t.Fatalf("save actual WM frame: %v / %v", err, closeErr)
				}
			}
			if !tc.explicit {
				b.closeRequests <- struct{}{}
			}
			waitRuntimeExit(t, done)
			assertRuntimeWindowClosed(t, recorder)
			want = append(want, "CLOSE")
			if got := recorder.sequence()[before:]; !reflect.DeepEqual(got, want) {
				t.Fatalf("WM sequence: got %v, want %v", got, want)
			}
			if !strings.Contains(logs.String(), "map-ready\n") || !strings.Contains(logs.String(), "map-closed\n") {
				t.Fatalf("Map did not complete lifecycle:\n%s", logs.String())
			}
			if strings.Contains(logs.String(), "map-close-event") == tc.explicit {
				t.Fatalf("unexpected native-close handling:\n%s", logs.String())
			}
			label := "target-visible"
			if tc.outside {
				label = "target-outside"
			}
			if !strings.Contains(logs.String(), label+"\n") {
				t.Fatalf("wrong visibility decision:\n%s", logs.String())
			}
			select {
			case err := <-wmDone:
				stopped = true
				t.Fatalf("Map stopped WM: %v", err)
			default:
			}
		})
	}
	stopWM()
	for _, path := range []string{commandSocket, eventSocket} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("WM left socket behind: %s", path)
		}
	}
}

func replaceMapTcFixture(t *testing.T, source, before, after string) string {
	t.Helper()
	if strings.Count(source, before) != 1 {
		t.Fatalf("expected exactly one fixture %q", before)
	}
	return strings.Replace(source, before, after, 1)
}
