//go:build sdl

package sdl

import (
	"errors"
	"image"
	"image/color"
	"image/draw"
	_ "image/png"
	"os"
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
	win, err := sdl.CreateWindow(
		cfg.Title,
		sdl.WINDOWPOS_CENTERED, sdl.WINDOWPOS_CENTERED,
		int32(cfg.Width), int32(cfg.Height),
		sdl.WINDOW_HIDDEN,
	)
	if err != nil {
		sdl.Quit()
		return nil, err
	}
	surface, err := win.GetSurface()
	if err != nil {
		win.Destroy()
		sdl.Quit()
		return nil, err
	}
	return &window{
		win:     win,
		back:    image.NewRGBA(image.Rect(0, 0, cfg.Width, cfg.Height)),
		surface: surface,
	}, nil
}

type window struct {
	mu      sync.Mutex
	win     *sdl.Window
	surface *sdl.Surface
	back    *image.RGBA
	closed  bool
}

func (w *window) Show() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("window is closed")
	}
	w.win.Show()
	return w.win.UpdateSurface()
}

func (w *window) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	// SDL_DestroyWindow has no failure result. go-sdl2's Destroy nevertheless returns any SDL error
	// text set during teardown, e.g. "Parameter 'texture' is invalid" from a texture-backed window
	// surface (Wayland), although the window has been destroyed. That is not a close failure, and
	// returning it made WM exit after the native close control instead of staying alive.
	_ = w.win.Destroy()
	sdl.Quit()
	return nil
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

func (w *window) BeginFrame(c backend.Color) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("window is closed")
	}
	draw.Draw(w.back, w.back.Bounds(), &image.Uniform{C: color.NRGBA{
		R: c.R, G: c.G, B: c.B, A: c.A,
	}}, image.Point{}, draw.Src)
	return nil
}

func (w *window) DrawImage(path string, x, y, width, height int) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("window is closed")
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
	for py := dst.Min.Y; py < dst.Max.Y; py++ {
		for px := dst.Min.X; px < dst.Max.X; px++ {
			sx := src.Bounds().Min.X + (px-x)*src.Bounds().Dx()/width
			sy := src.Bounds().Min.Y + (py-y)*src.Bounds().Dy()/height
			sc := color.NRGBAModel.Convert(src.At(sx, sy)).(color.NRGBA)
			blend(w.back, px, py, sc)
		}
	}
	return nil
}

func (w *window) DrawCircle(x, y, radius int, c backend.Color) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("window is closed")
	}
	if radius <= 0 {
		return errors.New("radius must be positive")
	}
	bounds := w.back.Bounds()
	r2 := radius * radius
	for py := max(y-radius, bounds.Min.Y); py <= min(y+radius, bounds.Max.Y-1); py++ {
		for px := max(x-radius, bounds.Min.X); px <= min(x+radius, bounds.Max.X-1); px++ {
			dx, dy := px-x, py-y
			if dx*dx+dy*dy <= r2 {
				blend(w.back, px, py, color.NRGBA{
					R: c.R, G: c.G, B: c.B, A: c.A,
				})
			}
		}
	}
	return nil
}

func (w *window) Present() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("window is closed")
	}

	if err := w.surface.Lock(); err != nil {
		return err
	}
	defer w.surface.Unlock()

	pixels := w.surface.Pixels()
	pitch := int(w.surface.Pitch)
	bpp := int(w.surface.Format.BytesPerPixel)

	for y := 0; y < w.back.Bounds().Dy(); y++ {
		for x := 0; x < w.back.Bounds().Dx(); x++ {
			c := color.NRGBAModel.Convert(w.back.At(x, y)).(color.NRGBA)
			offset := y*pitch + x*bpp
			if bpp == 4 {
				v := sdl.MapRGBA(w.surface.Format, c.R, c.G, c.B, c.A)
				pixels[offset] = byte(v)
				pixels[offset+1] = byte(v >> 8)
				pixels[offset+2] = byte(v >> 16)
				pixels[offset+3] = byte(v >> 24)
			}
		}
	}
	return w.win.UpdateSurface()
}

func blend(dst *image.RGBA, x, y int, src color.NRGBA) {
	if src.A == 0 {
		return
	}
	dstColor := color.NRGBAModel.Convert(dst.At(x, y)).(color.NRGBA)
	a := uint32(src.A)
	da := uint32(dstColor.A)
	outA := a + da*(255-a)/255
	if outA == 0 {
		dst.Set(x, y, color.NRGBA{})
		return
	}
	channel := func(s, d uint8) uint8 {
		n := uint32(s)*a + uint32(d)*da*(255-a)/255
		return uint8(n / outA)
	}
	dst.Set(x, y, color.NRGBA{
		R: channel(src.R, dstColor.R),
		G: channel(src.G, dstColor.G),
		B: channel(src.B, dstColor.B),
		A: uint8(outA),
	})
}
