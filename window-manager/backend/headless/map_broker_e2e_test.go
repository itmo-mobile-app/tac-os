package headless

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/itmo-mobile-app/tac-os/window-manager/server"
)

type mapBrokerLog struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *mapBrokerLog) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *mapBrokerLog) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func waitMapBroker(t *testing.T, description string, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for !ready() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for " + description)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func buildMapBrokerBinary(t *testing.T, root, directory, name string) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), name)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(runtime.GOROOT(), "bin/go"), "build", "-race", "-buildvcs=false", "-o", binary, ".")
	cmd.Dir = filepath.Join(root, directory)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v\n%s", name, err, output)
	}
	return binary
}

type liveMapProcess struct {
	backend *observedBackend
	cmd     *exec.Cmd
	done    chan error
	logs    mapBrokerLog
	stopped bool
}

func startLiveMap(t *testing.T, root, binary, broker string, fixture bool) *liveMapProcess {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "map-broker-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	ctx, cancel := context.WithCancel(context.Background())
	b := &observedBackend{Backend: New()}
	wm := &server.Server{Backend: b, SocketPath: filepath.Join(dir, "cmd.sock"), EventSocketPath: filepath.Join(dir, "events.sock")}
	wmDone := make(chan error, 1)
	go func() { wmDone <- wm.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-wmDone:
			if err != nil {
				t.Errorf("WM shutdown: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("WM shutdown timed out")
		}
	})
	waitMapBroker(t, "WM sockets", func() bool {
		_, a := os.Stat(wm.SocketPath)
		_, b := os.Stat(wm.EventSocketPath)
		return a == nil && b == nil
	})
	args := []string{"-config", filepath.Join(root, "app/map/assets/test-map.json"), "-socket", wm.SocketPath, "-events", wm.EventSocketPath, "-broker", broker}
	if fixture {
		args = append(args, "-observations", filepath.Join(root, "app/map/assets/observations.jsonl"))
	}
	m := &liveMapProcess{backend: b, done: make(chan error, 1)}
	m.cmd = exec.Command(binary, args...)
	m.cmd.Stdout, m.cmd.Stderr = &m.logs, &m.logs
	if err := m.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { m.done <- m.cmd.Wait() }()
	t.Cleanup(func() {
		if !m.stopped {
			_ = m.cmd.Process.Kill()
			<-m.done
		}
		if t.Failed() {
			t.Log(m.logs.String())
		}
	})
	waitMapBroker(t, "Map SHOW", func() bool { return strings.Contains(m.logs.String(), "Map shown") })
	return m
}

func (m *liveMapProcess) window() *window {
	m.backend.Backend.mu.Lock()
	defer m.backend.Backend.mu.Unlock()
	return m.backend.Backend.w
}

func (m *liveMapProcess) assertRunning(t *testing.T) {
	t.Helper()
	select {
	case err := <-m.done:
		m.stopped = true
		t.Fatalf("Map unexpectedly exited: %v\n%s", err, m.logs.String())
	default:
	}
}

func (m *liveMapProcess) close(t *testing.T) {
	t.Helper()
	if err := m.cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-m.done:
		m.stopped = true
		if err != nil {
			t.Fatalf("Map CLOSE: %v\n%s", err, m.logs.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Map did not close")
	}
	w := m.window()
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.closed || w.visible {
		t.Fatal("WM window was not closed")
	}
}

// Real Broker process -> real Map CLI -> real WM server and production renderer.
// No protocol handlers, command responses or rendering results are replaced.
func TestMapBrokerWindowManagerE2E(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	mapBinary := buildMapBrokerBinary(t, root, "app/map/cmd/map-demo", "map-demo")
	brokerBinary := buildMapBrokerBinary(t, root, "broker", "broker")
	port, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := port.Addr().String()
	portNumber := port.Addr().(*net.TCPAddr).Port
	_ = port.Close()
	broker := exec.Command(brokerBinary, fmt.Sprint(portNumber))
	var brokerLogs mapBrokerLog
	broker.Stdout, broker.Stderr = &brokerLogs, &brokerLogs
	if err := broker.Start(); err != nil {
		t.Fatal(err)
	}
	brokerStopped := false
	t.Cleanup(func() {
		if !brokerStopped {
			_ = broker.Process.Kill()
			_ = broker.Wait()
		}
	})
	var wire net.Conn
	waitMapBroker(t, "real Broker", func() bool {
		wire, err = net.DialTimeout("tcp", address, 100*time.Millisecond)
		return err == nil
	})
	defer wire.Close()
	_ = wire.SetDeadline(time.Now().Add(25 * time.Second))
	reader := bufio.NewReader(wire)
	// SUB then PUB on the same connection proves the real subscription is installed.
	// The readiness payload is opaque to Broker and is consumed before Map starts.
	if _, err := fmt.Fprint(wire, "SUB observations\nPUB observations {\"ready\":true}\n"); err != nil {
		t.Fatal(err)
	}
	if line, err := reader.ReadString('\n'); err != nil || line != "MSG observations {\"ready\":true}\n" {
		t.Fatalf("Broker subscription barrier: %q, %v", line, err)
	}
	m := startLiveMap(t, root, mapBinary, address, true)
	line, err := reader.ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "MSG observations ") {
		t.Fatalf("Map publication: %q, %v", line, err)
	}
	var got, want map[string]json.RawMessage
	fixture, err := os.ReadFile(filepath.Join(root, "app/map/assets/observations.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "MSG observations ")), &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(fixture, &want); err != nil {
		t.Fatal(err)
	}
	if len(got) != 6 || !reflect.DeepEqual(got, want) {
		t.Fatalf("Observation changed or acquired extra fields: %s", line)
	}
	// Map sends SUB before PUB on its one TCP connection. Receipt of its publication
	// therefore also proves its targets.updated subscription is ready, without sleeps.
	w := m.window()
	imageFile, err := os.Open(filepath.Join(root, "app/map/assets/test-map.png"))
	if err != nil {
		t.Fatal(err)
	}
	background, err := png.Decode(imageFile)
	_ = imageFile.Close()
	if err != nil {
		t.Fatal(err)
	}
	pixel := func(p image.Point) color.RGBA {
		w.mu.Lock()
		defer w.mu.Unlock()
		return w.front.RGBAAt(p.X, p.Y)
	}
	backgroundAt := func(p image.Point) color.RGBA { return color.RGBAModel.Convert(background.At(p.X, p.Y)).(color.RGBA) }
	publish := func(payload string) {
		t.Helper()
		if _, err := fmt.Fprintf(wire, "PUB targets.updated %s\n", payload); err != nil {
			t.Fatal(err)
		}
	}
	first, second, center := image.Pt(240, 160), image.Pt(720, 480), image.Pt(480, 320)
	publish(`{"targetId":"17","latitude":59.9505,"longitude":30.3095,"probability":0.9,"lastSeenAt":1792255200000}`)
	waitMapBroker(t, "first actual marker", func() bool { return nearColor(pixel(first), color.RGBA{R: 200, G: 85, B: 63, A: 255}) })
	publish(`{"targetId":"17","latitude":59.9495,"longitude":30.3125,"probability":0.2,"lastSeenAt":1792255200001}`)
	waitMapBroker(t, "moved and faded marker, with old pixels erased", func() bool {
		return nearColor(pixel(second), color.RGBA{R: 231, G: 182, B: 172, A: 255}) && pixel(first) == backgroundAt(first)
	})
	publish(`{"targetId":"18","latitude":59.9500,"longitude":30.3110,"probability":0.55,"lastSeenAt":1792255200002}`)
	waitMapBroker(t, "second independent target", func() bool { return nearColor(pixel(center), color.RGBA{R: 216, G: 133, B: 117, A: 255}) })
	publish(`{"targetId":"17","latitude":59.9500,"longitude":30.3110,"probability":2,"lastSeenAt":1792255200003}`)
	waitMapBroker(t, "invalid Target rejection", func() bool { return strings.Contains(m.logs.String(), "Ignored invalid Target") })
	if !nearColor(pixel(second), color.RGBA{R: 231, G: 182, B: 172, A: 255}) {
		t.Fatal("invalid message changed the marker")
	}
	publish(`{"targetId":"17","latitude":59.9520,"longitude":30.3110,"probability":0.2,"lastSeenAt":1792255200004}`)
	waitMapBroker(t, "outside target removed from the frame", func() bool { return pixel(second) == backgroundAt(second) })
	w.mu.Lock()
	lastFrame := append([]byte(nil), w.front.Pix...)
	w.mu.Unlock()
	_ = broker.Process.Kill()
	_ = broker.Wait()
	brokerStopped = true
	waitMapBroker(t, "Broker disconnect notice", func() bool { return strings.Contains(m.logs.String(), "Broker unavailable") })
	m.assertRunning(t)
	w.mu.Lock()
	preserved := bytes.Equal(lastFrame, w.front.Pix) && w.visible && !w.closed
	w.mu.Unlock()
	if !preserved || m.window() != w {
		t.Fatal("Broker failure changed the last frame or recreated the window")
	}
	counts := make(map[string]int)
	for _, call := range m.backend.sequence() {
		counts[call]++
	}
	if counts["CREATE"] != 1 || counts["SHOW"] != 1 || counts["PRESENT"] < 5 {
		t.Fatalf("window was restarted or not redrawn: %v", counts)
	}
	m.close(t)
	t.Log("real Broker <-> Map CLI <-> WM: exact Observation, live pixels, opacity, no restart, invalid input, disconnect and CLOSE verified")

	// The now-stopped Broker also exercises connection refusal on initial startup.
	t.Run("BrokerUnavailableAtStartup", func(t *testing.T) {
		offline := startLiveMap(t, root, mapBinary, address, false)
		waitMapBroker(t, "initial connection failure", func() bool { return strings.Contains(offline.logs.String(), "Broker unavailable") })
		offline.assertRunning(t)
		offline.close(t)
	})
}
