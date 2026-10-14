//go:build sdl

package sdl

import (
	"errors"
	"sync"

	"github.com/veandco/go-sdl2/sdl"

	"github.com/itmo-mobile-app/tac-os/window-manager/backend"
)

type Backend struct{}

func New() *Backend { return &Backend{} }

func (b *Backend) Create(cfg backend.WindowConfig) (backend.Window, error) {
	if cfg.Width <= 0 {
		cfg.Width = 960
	}
	if cfg.Height <= 0 {
		cfg.Height = 640
	}
	if cfg.Title == "" {
		cfg.Title = "Tactical Map"
	}
	if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
		return nil, err
	}
	win, err := sdl.CreateWindow(cfg.Title, sdl.WINDOWPOS_CENTERED, sdl.WINDOWPOS_CENTERED, int32(cfg.Width), int32(cfg.Height), sdl.WINDOW_SHOWN)
	if err != nil {
		sdl.Quit()
		return nil, err
	}
	return &window{win: win}, nil
}

type window struct {
	mu     sync.Mutex
	win    *sdl.Window
	closed bool
}

func (w *window) Show() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("window is closed")
	}
	w.win.Show()
	return nil
}

func (w *window) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	err := w.win.Destroy()
	sdl.Quit()
	return err
}

func (w *window) PollEvents() ([]string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil, nil
	}
	var events []string
	for {
		ev := sdl.PollEvent()
		if ev == nil {
			break
		}
		switch e := ev.(type) {
		case *sdl.QuitEvent:
			events = append(events, "window_close")
			return events, nil
		case *sdl.WindowEvent:
			if e.Event == sdl.WINDOWEVENT_CLOSE {
				events = append(events, "window_close")
				return events, nil
			}
		}
	}
	return events, nil
}
