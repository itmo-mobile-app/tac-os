package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

type binaries struct {
	broker, target, wm, tacticalMap string
}

func repositoryRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "tests", "system-smoke", "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("run from inside the tac-os checkout")
		}
		dir = parent
	}
}

func offlineEnv() []string {
	return append(os.Environ(), "GOWORK=off", "GOPROXY=off", "GOSUMDB=off", "GOTOOLCHAIN=local", "CGO_ENABLED=0", "GOFLAGS=", "GOOS=", "GOARCH=")
}

func buildComponents(ctx context.Context, root, dir string) (binaries, error) {
	var b binaries
	components := []struct {
		name, directory, pkg string
		output               *string
	}{
		{"broker", "broker", ".", &b.broker},
		{"target-service", "target-service", ".", &b.target},
		{"wm-headless", "tests/system-smoke", "./cmd/wm-headless", &b.wm},
		{"map-demo", "app/map", "./cmd/map-demo", &b.tacticalMap},
	}
	for _, component := range components {
		*component.output = filepath.Join(dir, component.name)
		cmd := exec.CommandContext(ctx, "go", "build", "-mod=readonly", "-buildvcs=false", "-o", *component.output, component.pkg)
		cmd.Dir = filepath.Join(root, component.directory)
		cmd.Env = offlineEnv()
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
		cmd.WaitDelay = time.Second
		output, err := cmd.CombinedOutput()
		if err != nil {
			return b, fmt.Errorf("build %s: %w\n%s", component.name, errors.Join(err, ctx.Err()), output)
		}
	}
	return b, nil
}
