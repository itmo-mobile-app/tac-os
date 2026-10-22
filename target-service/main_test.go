package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type safeBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *safeBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}
func (s *safeBuffer) String() string { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

const valid = `{"targetId":"t1","observerId":"o1","latitude":55.7,"longitude":37.6,"confidence":0.8,"timestamp":1760000000000}`

func TestObservationValidation(t *testing.T) {
	if _, err := parseObservation(valid); err != nil {
		t.Fatal(err)
	}
	cases := []string{
		`{}`, `not json`, `[]`,
		`{"targetId":"","observerId":"o1","latitude":55.7,"longitude":37.6,"confidence":0.8,"timestamp":1760000000000}`,
		`{"targetId":"t1","observerId":"o1","latitude":91,"longitude":37.6,"confidence":0.8,"timestamp":1760000000000}`,
		`{"targetId":"t1","observerId":"o1","latitude":55.7,"longitude":37.6,"confidence":1.2,"timestamp":1760000000000}`,
		`{"targetId":"t1","observerId":"o1","latitude":55.7,"longitude":37.6,"confidence":0.8,"timestamp":0}`,
		`{"targetId":"t1","observerId":"o1","latitude":"55.7","longitude":37.6,"confidence":0.8,"timestamp":1760000000000}`,
	}
	for _, s := range cases {
		if _, err := parseObservation(s); err == nil {
			t.Errorf("expected error for %s", s)
		}
	}
}

func TestHandleMessage(t *testing.T) {
	store := newStore()
	var output bytes.Buffer
	logger := log.New(&output, "", 0)
	handleMessage("MSG observations "+valid, store, logger)
	if len(store.latest) != 1 || store.latest["t1"].ObserverID != "o1" {
		t.Fatalf("unexpected state: %+v", store.latest)
	}
	handleMessage("MSG observations {}", store, logger)
	handleMessage("MSG targets.updated "+valid, store, logger)
	if len(store.latest) != 1 {
		t.Fatal("invalid messages modified state")
	}
	if !strings.Contains(output.String(), "invalid observation") {
		t.Fatal("missing validation error log")
	}
}

func TestUnavailableBroker(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = run(ctx, addr, log.New(&bytes.Buffer{}, "", 0), nil)
	if err == nil || !strings.Contains(err.Error(), "connect to Broker") {
		t.Fatalf("expected connection error, got %v", err)
	}
}

// Integration test launches the real Broker from ../broker (not a mock).
func TestIntegrationRealBroker(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	brokerDir := filepath.Join("..", "broker")
	if _, err := os.Stat(filepath.Join(brokerDir, "main.go")); err != nil {
		t.Skip("real Broker not found at ../broker")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	brokerBin := filepath.Join(t.TempDir(), "test-broker")
	if runtime.GOOS == "windows" {
		brokerBin += ".exe"
	}
	build := exec.Command("go", "build", "-o", brokerBin, ".")
	build.Dir = brokerDir
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build Broker: %v: %s", err, output)
	}
	cmd := exec.Command(brokerBin, fmt.Sprint(port))
	var logs bytes.Buffer
	cmd.Stdout = &logs
	cmd.Stderr = &logs
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	var pub net.Conn
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		pub, err = net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("Broker did not start: %v, logs: %s", err, logs.String())
	}
	defer pub.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	done := make(chan error, 1)
	var targetLogs safeBuffer
	go func() { done <- run(ctx, addr, log.New(&targetLogs, "", 0), ready) }()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("Target Service exited: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("Target Service did not subscribe")
	}
	// TCP writes on distinct connections can be scheduled independently: publish
	// repeatedly until receipt is observed, avoiding a fragile startup race.
	for i := 0; i < 20; i++ {
		if _, err := fmt.Fprintf(pub, "PUB observations %s\n", valid); err != nil {
			t.Fatal(err)
		}
		time.Sleep(50 * time.Millisecond)
		if strings.Contains(targetLogs.String(), "observation stored targetId=") {
			break
		}
	}
	if !strings.Contains(targetLogs.String(), "observation stored targetId=") {
		t.Fatalf("did not receive observation: %s", targetLogs.String())
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown timeout")
	}
}
