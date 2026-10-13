package backend

import "errors"

type WindowConfig struct {
	ID     string
	Title  string
	Width  int
	Height int
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
