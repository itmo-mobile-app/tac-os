package backend

import "errors"

type WindowConfig struct {
	ID     string
	Title  string
	Width  int
	Height int
}
type Color struct {
	R uint8
	G uint8
	B uint8
	A uint8
}

type DrawingWindow interface {
	BeginFrame(color Color) error
	DrawImage(path string, x, y, width, height int) error
	DrawCircle(x, y, radius int, color Color) error
	Present() error
}

type Window interface {
	Show() error
	Close() error
	PollEvents() ([]string, error)
}

type Backend interface {
	Create(WindowConfig) (Window, error)
}

var ErrNotImplemented = errors.New("backend not implemented")
