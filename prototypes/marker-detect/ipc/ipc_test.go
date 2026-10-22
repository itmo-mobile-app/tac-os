//go:build unix

package ipc

import (
	"bufio"
	"errors"
	"math"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tac-os/marker-detect/marker"
)

// Wire lines from the specification (spec/camera-detector-protocol.md).
const (
	wireTwo         = `{"ok":true,"detections":[{"id":17,"relativeAngle":12.5,"distance":3.2},{"id":23,"relativeAngle":-8.4,"distance":5.7}]}`
	wireEmpty       = `{"ok":true,"detections":[]}`
	wireCameraError = `{"ok":false,"error":"CAMERA_UNAVAILABLE"}`
	wireDetectError = `{"ok":false,"error":"DETECTION_FAILED"}`
	wireBadRequest  = `{"ok":false,"error":"BAD_REQUEST"}`
	requestDetect   = `{"command":"DETECT"}`
)

// shortDir returns a short temporary directory: the path of a Unix socket is limited to about 100 bytes, and
// t.TempDir() on macOS is longer.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "tacd-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

type testServer struct {
	srv  *Server
	path string
	done chan error // the result of Serve
}

// startServer listens on a fresh socket and serves source until the end of the test.
func startServer(t *testing.T, source Source) *testServer {
	t.Helper()
	path := filepath.Join(shortDir(t), "d.sock")
	l, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	ts := &testServer{srv: NewServer(source), path: path, done: make(chan error, 1)}
	go func() { ts.done <- ts.srv.Serve(l) }()
	t.Cleanup(func() { ts.srv.Close() })
	return ts
}

type client struct {
	t *testing.T
	c net.Conn
	r *bufio.Reader
}

func (ts *testServer) dial(t *testing.T) *client {
	t.Helper()
	c, err := net.Dial("unix", ts.path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return &client{t: t, c: c, r: bufio.NewReader(c)}
}

func (c *client) send(s string) {
	c.t.Helper()
	c.c.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.c.Write([]byte(s)); err != nil {
		c.t.Fatalf("write: %v", err)
	}
}

// recv reads one response line without the newline.
func (c *client) recv() string {
	c.t.Helper()
	c.c.SetReadDeadline(time.Now().Add(5 * time.Second))
	line, err := c.r.ReadString('\n')
	if err != nil {
		c.t.Fatalf("read: %v (got %q)", err, line)
	}
	return strings.TrimSuffix(line, "\n")
}

// ask sends one line and returns the response.
func (c *client) ask(line string) string {
	c.t.Helper()
	c.send(line + "\n")
	return c.recv()
}

// expectClosed checks that the server has closed the connection.
func (c *client) expectClosed() {
	c.t.Helper()
	c.c.SetReadDeadline(time.Now().Add(5 * time.Second))
	if line, err := c.r.ReadString('\n'); err == nil {
		c.t.Fatalf("expected the connection to be closed, got %q", line)
	} else if ne, ok := err.(net.Error); ok && ne.Timeout() {
		c.t.Fatal("the connection was not closed")
	}
}

func fakeServer(t *testing.T, mode string) *testServer {
	t.Helper()
	src, err := NewFakeSource(mode)
	if err != nil {
		t.Fatal(err)
	}
	return startServer(t, src)
}

func TestTwoDetections(t *testing.T) {
	c := fakeServer(t, FakeTwo).dial(t)
	if got := c.ask(requestDetect); got != wireTwo {
		t.Errorf("got  %s\nwant %s", got, wireTwo)
	}
}

func TestEmptyListIsSuccess(t *testing.T) {
	c := fakeServer(t, FakeEmpty).dial(t)
	if got := c.ask(requestDetect); got != wireEmpty {
		t.Errorf("got  %s\nwant %s", got, wireEmpty)
	}
}

func TestCameraUnavailable(t *testing.T) {
	c := fakeServer(t, FakeCameraUnavailable).dial(t)
	if got := c.ask(requestDetect); got != wireCameraError {
		t.Errorf("got  %s\nwant %s", got, wireCameraError)
	}
	// A camera error is not fatal for the connection: the next request is answered as well.
	if got := c.ask(requestDetect); got != wireCameraError {
		t.Errorf("second request: got %s", got)
	}
}

func TestWrappedCameraErrorIsCameraUnavailable(t *testing.T) {
	src := SourceFunc(func() ([]marker.Detection, error) {
		return nil, errors.Join(errors.New("cannot open camera 0"), ErrCameraUnavailable)
	})
	c := startServer(t, src).dial(t)
	if got := c.ask(requestDetect); got != wireCameraError {
		t.Errorf("got %s", got)
	}
}

func TestDetectionFailed(t *testing.T) {
	c := fakeServer(t, FakeDetectionFailed).dial(t)
	if got := c.ask(requestDetect); got != wireDetectError {
		t.Errorf("got  %s\nwant %s", got, wireDetectError)
	}
}

// Two requests over one connection of one running server: the connection is persistent and each request is
// answered with a fresh result.
func TestSequentialRequestsOnOneConnection(t *testing.T) {
	var n atomic.Int32
	src := SourceFunc(func() ([]marker.Detection, error) {
		if n.Add(1) == 1 {
			return []marker.Detection{{ID: 5, RelativeAngleDeg: 1, DistanceM: 2}}, nil
		}
		return nil, nil
	})
	c := startServer(t, src).dial(t)
	if got, want := c.ask(requestDetect), `{"ok":true,"detections":[{"id":5,"relativeAngle":1,"distance":2}]}`; got != want {
		t.Errorf("first: got %s", got)
	}
	if got := c.ask(requestDetect); got != wireEmpty {
		t.Errorf("second: got %s", got)
	}
	if got := n.Load(); got != 2 {
		t.Errorf("the source was called %d times, want 2", got)
	}
}

func TestBadRequests(t *testing.T) {
	cases := []struct{ name, line string }{
		{"empty line", ``},
		{"not JSON", `DETECT`},
		{"truncated JSON", `{"command":"DETECT"`},
		{"array", `["DETECT"]`},
		{"string", `"DETECT"`},
		{"unknown command", `{"command":"STOP"}`},
		{"lower case command", `{"command":"detect"}`},
		{"empty command", `{"command":""}`},
		{"upper case key", `{"Command":"DETECT"}`},
		{"missing command", `{}`},
		{"extra field", `{"command":"DETECT","id":1}`},
		{"duplicate command", `{"command":"DETECT","command":"DETECT"}`},
		{"number command", `{"command":1}`},
		{"null command", `{"command":null}`},
		{"object command", `{"command":{"a":"DETECT"}}`},
		{"data after the object", `{"command":"DETECT"} {"command":"DETECT"}`},
		{"garbage after the object", `{"command":"DETECT"}x`},
	}
	c := fakeServer(t, FakeTwo).dial(t)
	for _, tc := range cases {
		if got := c.ask(tc.line); got != wireBadRequest {
			t.Errorf("%s: got %s, want %s", tc.name, got, wireBadRequest)
		}
		// The connection stays usable after BAD_REQUEST.
		if got := c.ask(requestDetect); got != wireTwo {
			t.Errorf("%s: the next valid request got %s", tc.name, got)
		}
	}
}

func TestWhitespaceAroundRequestIsAccepted(t *testing.T) {
	c := fakeServer(t, FakeEmpty).dial(t)
	if got := c.ask(" \t{ \"command\" : \"DETECT\" }\r"); got != wireEmpty {
		t.Errorf("got %s", got)
	}
}

func TestFrameSizeLimit(t *testing.T) {
	ts := fakeServer(t, FakeEmpty)

	// The longest allowed request is 64 KiB including the newline.
	exact := requestDetect + strings.Repeat(" ", maxFrameBytes-len(requestDetect)-1)
	c := ts.dial(t)
	if got := c.ask(exact); got != wireEmpty {
		t.Errorf("a %d byte request: got %s", len(exact)+1, got)
	}

	// One byte more is rejected and the connection is closed, because the stream cannot be resynchronized.
	c = ts.dial(t)
	c.send(exact + " \n")
	if got := c.recv(); got != wireBadRequest {
		t.Errorf("a too long request: got %s", got)
	}
	c.expectClosed()

	// The server keeps working for other clients.
	if got := ts.dial(t).ask(requestDetect); got != wireEmpty {
		t.Errorf("after the long request: got %s", got)
	}
}

func TestRequestWithoutNewlineBeforeClose(t *testing.T) {
	ts := fakeServer(t, FakeEmpty)
	c := ts.dial(t)
	c.send(requestDetect) // no newline
	c.c.(*net.UnixConn).CloseWrite()
	if got := c.recv(); got != wireBadRequest {
		t.Errorf("got %s", got)
	}
	if got := ts.dial(t).ask(requestDetect); got != wireEmpty {
		t.Errorf("after the unterminated request: got %s", got)
	}
}

func TestInvalidDetectorOutputIsDetectionFailed(t *testing.T) {
	good := marker.Detection{ID: 1, RelativeAngleDeg: 0, DistanceM: 1}
	cases := map[string][]marker.Detection{
		"id above 71":    {{ID: 72, DistanceM: 1}},
		"negative id":    {{ID: -1, DistanceM: 1}},
		"duplicate id":   {good, good},
		"unsorted":       {{ID: 9, DistanceM: 1}, {ID: 3, DistanceM: 1}},
		"NaN angle":      {{ID: 1, RelativeAngleDeg: math.NaN(), DistanceM: 1}},
		"infinite angle": {{ID: 1, RelativeAngleDeg: math.Inf(1), DistanceM: 1}},
		"zero distance":  {{ID: 1, DistanceM: 0}},
		"negative dist":  {{ID: 1, DistanceM: -2}},
		"NaN distance":   {{ID: 1, DistanceM: math.NaN()}},
		"infinite dist":  {{ID: 1, DistanceM: math.Inf(1)}},
	}
	for name, dets := range cases {
		dets := dets
		src := SourceFunc(func() ([]marker.Detection, error) { return dets, nil })
		c := startServer(t, src).dial(t)
		if got := c.ask(requestDetect); got != wireDetectError {
			t.Errorf("%s: got %s, want %s", name, got, wireDetectError)
		}
	}
}

func TestNegativeZeroAngleIsWrittenAsZero(t *testing.T) {
	src := SourceFunc(func() ([]marker.Detection, error) {
		return []marker.Detection{{ID: 2, RelativeAngleDeg: math.Copysign(0, -1), DistanceM: 1.5}}, nil
	})
	c := startServer(t, src).dial(t)
	if got, want := c.ask(requestDetect), `{"ok":true,"detections":[{"id":2,"relativeAngle":0,"distance":1.5}]}`; got != want {
		t.Errorf("got %s", got)
	}
}

// Requests of several connections never reach the source at the same time.
func TestDetectionsAreSerialized(t *testing.T) {
	var running, maxRunning atomic.Int32
	src := SourceFunc(func() ([]marker.Detection, error) {
		cur := running.Add(1)
		for {
			m := maxRunning.Load()
			if cur <= m || maxRunning.CompareAndSwap(m, cur) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		running.Add(-1)
		return nil, nil
	})
	ts := startServer(t, src)

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := net.Dial("unix", ts.path)
			if err != nil {
				t.Errorf("dial: %v", err)
				return
			}
			defer conn.Close()
			r := bufio.NewReader(conn)
			for j := 0; j < 5; j++ {
				conn.SetDeadline(time.Now().Add(5 * time.Second))
				if _, err := conn.Write([]byte(requestDetect + "\n")); err != nil {
					t.Errorf("write: %v", err)
					return
				}
				line, err := r.ReadString('\n')
				if err != nil || strings.TrimSuffix(line, "\n") != wireEmpty {
					t.Errorf("got %q, %v", line, err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if got := maxRunning.Load(); got != 1 {
		t.Errorf("up to %d detections ran at the same time, want 1", got)
	}
}

func TestClientDisconnectDoesNotAffectServer(t *testing.T) {
	ts := fakeServer(t, FakeTwo)
	c := ts.dial(t)
	c.send(requestDetect + "\n")
	c.c.Close() // without reading the answer
	if got := ts.dial(t).ask(requestDetect); got != wireTwo {
		t.Errorf("got %s", got)
	}
}

func TestCloseStopsServerAndRemovesSocket(t *testing.T) {
	ts := fakeServer(t, FakeTwo)
	c := ts.dial(t)
	if got := c.ask(requestDetect); got != wireTwo {
		t.Fatalf("got %s", got)
	}
	if _, err := os.Stat(ts.path); err != nil {
		t.Fatalf("the socket file does not exist: %v", err)
	}

	if err := ts.srv.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := ts.srv.Close(); err != nil {
		t.Fatalf("the second Close: %v", err)
	}
	select {
	case err := <-ts.done:
		if err != nil {
			t.Errorf("Serve returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return")
	}

	c.expectClosed() // the open connection was closed
	if _, err := os.Lstat(ts.path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the socket file must be removed, Lstat: %v", err)
	}
	if c2, err := net.Dial("unix", ts.path); err == nil {
		c2.Close()
		t.Error("a new connection was accepted after Close")
	}
}

// Close waits for a request that is being processed instead of cutting it off silently.
func TestCloseWaitsForRunningDetection(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	src := SourceFunc(func() ([]marker.Detection, error) {
		close(started)
		<-release
		return nil, nil
	})
	ts := startServer(t, src)
	c := ts.dial(t)
	c.send(requestDetect + "\n")
	<-started

	closed := make(chan struct{})
	go func() { ts.srv.Close(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("Close returned while a detection was running")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return after the detection finished")
	}
}

func TestServeAfterCloseReturnsImmediately(t *testing.T) {
	path := filepath.Join(shortDir(t), "d.sock")
	l, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	srv := NewServer(SourceFunc(func() ([]marker.Detection, error) { return nil, nil }))
	srv.Close()
	if err := srv.Serve(l); err != nil {
		t.Errorf("Serve: %v", err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the socket file must be removed, Lstat: %v", err)
	}
}

func TestListenSocketIsPrivate(t *testing.T) {
	path := filepath.Join(shortDir(t), "d.sock")
	l, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSocket == 0 {
		t.Errorf("%s is not a socket: %v", path, fi.Mode())
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("socket permissions %o, want 600", perm)
	}
}

func TestListenReplacesStaleSocket(t *testing.T) {
	path := filepath.Join(shortDir(t), "d.sock")
	// A socket file that nobody listens on any more: a listener that does not unlink its file when closed.
	old, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	old.(*net.UnixListener).SetUnlinkOnClose(false)
	old.Close()
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("the stale socket file is missing: %v", err)
	}

	l, err := Listen(path)
	if err != nil {
		t.Fatalf("Listen over a stale socket: %v", err)
	}
	l.Close()
}

func TestListenDoesNotTouchActiveSocket(t *testing.T) {
	ts := fakeServer(t, FakeTwo)
	if l, err := Listen(ts.path); err == nil {
		l.Close()
		t.Fatal("Listen succeeded on a socket that is in use")
	}
	// The first server still works and its socket file is intact.
	if got := ts.dial(t).ask(requestDetect); got != wireTwo {
		t.Errorf("got %s", got)
	}
}

func TestListenDoesNotRemoveRegularFile(t *testing.T) {
	path := filepath.Join(shortDir(t), "important")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if l, err := Listen(path); err == nil {
		l.Close()
		t.Fatal("Listen succeeded over a regular file")
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != "data" {
		t.Errorf("the file was changed: %q, %v", b, err)
	}
}

func TestListenRejectsBadPath(t *testing.T) {
	for _, path := range []string{"", "/tmp/" + strings.Repeat("x", 120) + ".sock"} {
		if l, err := Listen(path); err == nil {
			l.Close()
			t.Errorf("Listen(%q) succeeded", path)
		}
	}
	if l, err := Listen("/nonexistent-dir-for-test/d.sock"); err == nil {
		l.Close()
		t.Error("Listen in a missing directory succeeded")
	}
}

func TestNewFakeSourceRejectsUnknownMode(t *testing.T) {
	if _, err := NewFakeSource("three"); err == nil {
		t.Error("an unknown mode was accepted")
	}
	for _, mode := range FakeModes {
		if _, err := NewFakeSource(mode); err != nil {
			t.Errorf("mode %q: %v", mode, err)
		}
	}
}

func TestFakeFixturesAreValid(t *testing.T) {
	src, _ := NewFakeSource(FakeTwo)
	dets, err := src.Detect()
	if err != nil {
		t.Fatal(err)
	}
	if err := validate(dets); err != nil {
		t.Error(err)
	}
}

func TestNormalize(t *testing.T) {
	in := []marker.Detection{
		{ID: 30, RelativeAngleDeg: 1, DistanceM: 4},
		{ID: 7, RelativeAngleDeg: 2, DistanceM: 5},
		{ID: 30, RelativeAngleDeg: 3, DistanceM: 2}, // the same marker, nearer
		{ID: 30, RelativeAngleDeg: 4, DistanceM: 9},
	}
	snapshot := append([]marker.Detection(nil), in...)

	got := Normalize(in)
	want := []marker.Detection{
		{ID: 7, RelativeAngleDeg: 2, DistanceM: 5},
		{ID: 30, RelativeAngleDeg: 3, DistanceM: 2},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Normalize = %v, want %v", got, want)
	}
	for i := range in {
		if in[i] != snapshot[i] {
			t.Error("Normalize modified its input")
		}
	}
	if err := validate(got); err != nil {
		t.Errorf("the result is not valid: %v", err)
	}
	if got := Normalize(nil); got == nil || len(got) != 0 {
		t.Errorf("Normalize(nil) = %#v, want an empty non-nil slice", got)
	}
}

func TestParseRequestAcceptsOnlyDetect(t *testing.T) {
	if err := parseRequest([]byte(`{"command":"DETECT"}` + "\n")); err != nil {
		t.Errorf("a valid request was rejected: %v", err)
	}
}
