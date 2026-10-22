package headless

import (
	"errors"
	"image"
	"image/color"
	_ "image/png"
	"os"
	"sync"

	"github.com/itmo-mobile-app/tac-os/window-manager/backend"
)

type Backend struct {
	mu sync.Mutex
	w  *window
}

type window struct {
	mu      sync.Mutex
	visible bool
	closed  bool
	back    *image.RGBA
	front   *image.RGBA
}

func New() *Backend { return &Backend{} }

func (b *Backend) Create(cfg backend.WindowConfig) (backend.Window, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.w != nil && !b.w.closed {
		return nil, ErrWindowExists
	}

	if cfg.Width <= 0 {
		cfg.Width = 960
	}
	if cfg.Height <= 0 {
		cfg.Height = 640
	}

	b.w = &window{
		back:  image.NewRGBA(image.Rect(0, 0, cfg.Width, cfg.Height)),
		front: image.NewRGBA(image.Rect(0, 0, cfg.Width, cfg.Height)),
	}
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

func (w *window) BeginFrame(c backend.Color) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return ErrWindowClosed
	}

	fill(w.back, rgba(c))
	return nil
}

func (w *window) DrawImage(path string, x, y, width, height int) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return ErrWindowClosed
	}
	if path == "" || width <= 0 || height <= 0 {
		return errors.New("invalid image parameters")
	}

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	src, _, err := image.Decode(file)
	if err != nil {
		return err
	}

	dst := image.Rect(x, y, x+width, y+height).Intersect(w.back.Bounds())
	if dst.Empty() {
		return nil
	}

	for py := dst.Min.Y; py < dst.Max.Y; py++ {
		for px := dst.Min.X; px < dst.Max.X; px++ {
			sx := (px - x) * src.Bounds().Dx() / width
			sy := (py - y) * src.Bounds().Dy() / height
			sc := src.At(src.Bounds().Min.X+sx, src.Bounds().Min.Y+sy)
			blend(w.back, px, py, sc)
		}
	}
	return nil
}

func (w *window) DrawCircle(x, y, radius int, c backend.Color) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return ErrWindowClosed
	}
	if radius <= 0 {
		return errors.New("radius must be positive")
	}

	bounds := w.back.Bounds()
	r2 := radius * radius

	minX := max(x-radius, bounds.Min.X)
	maxX := min(x+radius, bounds.Max.X-1)
	minY := max(y-radius, bounds.Min.Y)
	maxY := min(y+radius, bounds.Max.Y-1)

	for py := minY; py <= maxY; py++ {
		for px := minX; px <= maxX; px++ {
			dx, dy := px-x, py-y
			if dx*dx+dy*dy <= r2 {
				blend(w.back, px, py, rgba(c))
			}
		}
	}
	return nil
}

func (w *window) Present() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return ErrWindowClosed
	}

	copy(w.front.Pix, w.back.Pix)
	return nil
}

func rgba(c backend.Color) color.NRGBA {
	return color.NRGBA{R: c.R, G: c.G, B: c.B, A: c.A}
}

func fill(img *image.RGBA, c color.NRGBA) {
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			img.Set(x, y, c)
		}
	}
}

func blend(dst *image.RGBA, x, y int, srcColor color.Color) {
	sr, sg, sb, sa := srcColor.RGBA()
	if sa == 0 {
		return
	}

	dr, dg, db, da := dst.At(x, y).RGBA()
	const maxAlpha = uint32(65535)

	outA := sa + da*(maxAlpha-sa)/maxAlpha
	if outA == 0 {
		dst.SetRGBA(x, y, color.RGBA{})
		return
	}

	outR := sr + dr*(maxAlpha-sa)/maxAlpha
	outG := sg + dg*(maxAlpha-sa)/maxAlpha
	outB := sb + db*(maxAlpha-sa)/maxAlpha

	dst.SetRGBA(x, y, color.RGBA{
		R: uint8(outR >> 8),
		G: uint8(outG >> 8),
		B: uint8(outB >> 8),
		A: uint8(outA >> 8),
	})
}
