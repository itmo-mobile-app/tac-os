//go:build !sdl

package sdl

import (
	"errors"

	"github.com/itmo-mobile-app/tac-os/system/window-manager/backend"
)

var ErrSDLBuildTag = errors.New("SDL backend not compiled; rebuild with -tags sdl")

type Backend struct{}

func New() *Backend { return &Backend{} }

func (b *Backend) Create(_ backend.WindowConfig) (backend.Window, error) {
	return nil, ErrSDLBuildTag
}
