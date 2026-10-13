package headless

import (
	"sync"

	"github.com/itmo-mobile-app/tac-os/system/window-manager/backend"
)

type Backend struct {
	mu sync.Mutex
	w  *window
}

type window struct {
	mu      sync.Mutex
	visible bool
	closed  bool
}

func New() *Backend { return &Backend{} }

func (b *Backend) Create(_ backend.WindowConfig) (backend.Window, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.w != nil && !b.w.closed {
		return nil, ErrWindowExists
	}
	b.w = &window{}
	return b.w, nil
}

func (w *window) Show() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return ErrWindowClosed
	}
	w.visible = true
	return nil
}

func (w *window) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	w.visible = false
	return nil
}

func (w *window) PollEvents() ([]string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return nil, nil
}
