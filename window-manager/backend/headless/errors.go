package headless

import "errors"

var (
	ErrWindowExists = errors.New("a window already exists")
	ErrWindowClosed = errors.New("window is closed")
)
