package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

const stopTimeout = 3 * time.Second

type logBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	// Keep diagnostics bounded even when a child fails noisily.
	const limit = 64 * 1024
	if len(p) >= limit {
		b.b.Reset()
		_, _ = b.b.Write(p[len(p)-limit:])
	} else {
		if b.b.Len()+len(p) > limit {
			b.b.Next(b.b.Len() + len(p) - limit)
		}
		_, _ = b.b.Write(p)
	}
	return len(p), nil
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

type process struct {
	name string
	cmd  *exec.Cmd
	logs logBuffer
	done chan struct{}
	err  error // Written once before closing done.
}

func startProcess(name, binary string, args ...string) (*process, error) {
	p := &process{name: name, cmd: exec.Command(binary, args...), done: make(chan struct{})}
	p.cmd.Stdout, p.cmd.Stderr = &p.logs, &p.logs
	// Children have their own groups so cleanup also reaches descendants.
	p.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	p.cmd.WaitDelay = time.Second
	if err := p.cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", name, err)
	}
	go func() {
		p.err = p.cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

func (p *process) alive() error {
	select {
	case <-p.done:
		return fmt.Errorf("%s exited unexpectedly (%v)\n%s", p.name, p.err, p.logs.String())
	default:
		return nil
	}
}

func (p *process) wait(ctx context.Context) error {
	select {
	case <-p.done:
		return p.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *process) signal(signal syscall.Signal) error {
	err := syscall.Kill(-p.cmd.Process.Pid, signal)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}

// stop is idempotent and uses its own deadline, including after cancellation.
func (p *process) stop() error {
	select {
	case <-p.done:
		return nil
	default:
	}
	if err := p.signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("stop %s: %w", p.name, err)
	}
	timer := time.NewTimer(stopTimeout)
	defer timer.Stop()
	select {
	case <-p.done:
		return nil // Broker normally exits from SIGTERM without a handler.
	case <-timer.C:
	}
	if err := p.signal(syscall.SIGKILL); err != nil {
		return fmt.Errorf("kill %s: %w", p.name, err)
	}
	select {
	case <-p.done:
		return fmt.Errorf("%s required SIGKILL\n%s", p.name, p.logs.String())
	case <-time.After(stopTimeout):
		return fmt.Errorf("%s did not exit after SIGKILL", p.name)
	}
}

func waitFor(ctx context.Context, description string, processes []*process, ready func() (bool, error)) error {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%s: %w", description, err)
		}
		for _, p := range processes {
			if err := p.alive(); err != nil {
				return fmt.Errorf("%s: %w", description, err)
			}
		}
		ok, err := ready()
		if err != nil {
			return fmt.Errorf("%s: %w", description, err)
		}
		if ok {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s: %w", description, ctx.Err())
		case <-ticker.C:
		}
	}
}

func checkAbsent(paths ...string) error {
	for _, path := range paths {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("resource remains or cannot be checked: %s (%v)", path, err)
		}
	}
	return nil
}
