package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSystemSmoke(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	buildCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	b, err := buildComponents(buildCtx, root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	newDir := func(t *testing.T) string {
		t.Helper()
		dir, err := os.MkdirTemp("/tmp", "tac-smoke-test-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.RemoveAll(dir); err != nil {
				t.Error(err)
			}
		})
		return dir
	}
	t.Run("parallel", func(t *testing.T) {
		for i := 0; i < 2; i++ {
			t.Run(fmt.Sprint(i), func(t *testing.T) {
				t.Parallel()
				var output bytes.Buffer
				if err := positiveScenario(context.Background(), root, newDir(t), b, &output); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(output.String(), "PASS Map -> headless Window Manager") {
					t.Fatal(output.String())
				}
				if err := unavailableBrokerScenario(context.Background(), b.target); err != nil {
					t.Fatal(err)
				}
			})
		}
	})
	t.Run("repeat", func(t *testing.T) {
		if err := positiveScenario(context.Background(), root, newDir(t), b, io.Discard); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("cancel-after-start", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		output := &cancelOnReceipt{cancel: cancel}
		err := positiveScenario(ctx, root, newDir(t), b, output)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
		if !output.called {
			t.Fatal("cancelled before exercising real running services")
		}
	})
	t.Run("failure-with-map-running", func(t *testing.T) {
		dir := newDir(t)
		s := &suite{sockets: []string{filepath.Join(dir, "wm.sock"), filepath.Join(dir, "events.sock")}}
		t.Cleanup(func() {
			if err := s.cleanup(); err != nil {
				t.Error(err)
			}
		})
		ctx, cancel := context.WithTimeout(context.Background(), scenarioTimeout)
		defer cancel()
		_, addr, err := startBroker(ctx, s, b.broker)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.start("Target", b.target, "-broker", addr); err != nil {
			t.Fatal(err)
		}
		if _, err := s.start("WM", b.wm, "-socket", s.sockets[0], "-events", s.sockets[1]); err != nil {
			t.Fatal(err)
		}
		if err := waitFor(ctx, "WM startup", s.processes, func() (bool, error) {
			_, err := os.Stat(s.sockets[1])
			return err == nil, nil
		}); err != nil {
			t.Fatal(err)
		}
		m, err := s.start("Map", b.tacticalMap, "-config", filepath.Join(root, "app/map/assets/test-map.json"), "-socket", s.sockets[0], "-events", s.sockets[1])
		if err != nil {
			t.Fatal(err)
		}
		if err := waitFor(ctx, "Map shown", s.processes, func() (bool, error) {
			return strings.Contains(m.logs.String(), "Map shown."), nil
		}); err != nil {
			t.Fatal(err)
		}
		// Simulate a failed assertion while all four services are running.
		if err := s.cleanup(); err != nil {
			t.Fatal(err)
		}
		for _, p := range s.processes {
			select {
			case <-p.done:
				if err := syscall.Kill(p.cmd.Process.Pid, 0); !errors.Is(err, syscall.ESRCH) {
					t.Errorf("%s PID still exists: %v", p.name, err)
				}
			default:
				t.Errorf("%s was not waited for", p.name)
			}
		}
	})
	t.Run("cli-signals", func(t *testing.T) {
		binary := filepath.Join(t.TempDir(), "system-smoke")
		cliBuildCtx, cancelBuild := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancelBuild()
		build := exec.CommandContext(cliBuildCtx, "go", "build", "-mod=readonly", "-o", binary, ".")
		build.Env = offlineEnv()
		if output, err := build.CombinedOutput(); err != nil {
			t.Fatalf("build CLI: %v\n%s", err, output)
		}
		for _, signal := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
			t.Run(signal.String(), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				p, err := startProcess("interrupted CLI", binary)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := p.stop(); err != nil {
						t.Error(err)
					}
				})
				if err := waitFor(ctx, "CLI resources", []*process{p}, func() (bool, error) {
					return strings.Contains(p.logs.String(), "Resources: "), nil
				}); err != nil {
					t.Fatal(err)
				}
				if err := p.signal(signal); err != nil {
					t.Fatal(err)
				}
				if err := p.wait(ctx); err == nil || !strings.Contains(p.logs.String(), "context canceled") {
					t.Fatalf("expected handled cancellation, got %v\n%s", err, p.logs.String())
				}
				firstLine := strings.SplitN(p.logs.String(), "\n", 2)[0]
				dir := strings.TrimSpace(strings.TrimPrefix(firstLine, "Resources: "))
				if err := checkAbsent(dir); err != nil {
					t.Fatal(err)
				}
			})
		}
	})
}

type cancelOnReceipt struct {
	cancel context.CancelFunc
	called bool
}

func (w *cancelOnReceipt) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "PASS Broker -> Target Service") {
		w.called = true
		w.cancel()
	}
	return len(p), nil
}

func TestRunCancelledCleansDirectory(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var output bytes.Buffer
	if err := run(ctx, root, &output); err == nil {
		t.Fatal("cancelled build succeeded")
	}
	dir := strings.TrimSpace(strings.TrimPrefix(output.String(), "Resources: "))
	if !strings.HasPrefix(dir, "/tmp/tac-smoke-") {
		t.Fatalf("missing resource path: %s", output.String())
	}
	if err := checkAbsent(dir); err != nil {
		t.Fatal(err)
	}
}
