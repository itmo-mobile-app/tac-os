package headless

import (
	"image/color"
	"testing"

	"github.com/itmo-mobile-app/tac-os/window-manager/backend"
)

func TestBeginFrameAndPresent(t *testing.T) {
	b := New()
	w, err := b.Create(backend.WindowConfig{Width: 10, Height: 10})
	if err != nil {
		t.Fatal(err)
	}

	dw := w.(backend.DrawingWindow)

	err = dw.BeginFrame(backend.Color{R: 10, G: 20, B: 30, A: 255})
	if err != nil {
		t.Fatal(err)
	}

	if err := dw.Present(); err != nil {
		t.Fatal(err)
	}

	got := w.(*window).front.At(5, 5)
	want := color.RGBA{R: 10, G: 20, B: 30, A: 255}

	if got != want {
		t.Fatalf("pixel = %v, want %v", got, want)
	}
}

func TestDrawCircle(t *testing.T) {
	b := New()
	w, err := b.Create(backend.WindowConfig{Width: 10, Height: 10})
	if err != nil {
		t.Fatal(err)
	}

	dw := w.(backend.DrawingWindow)

	if err := dw.BeginFrame(backend.Color{A: 255}); err != nil {
		t.Fatal(err)
	}

	if err := dw.DrawCircle(5, 5, 2, backend.Color{
		R: 255, A: 255,
	}); err != nil {
		t.Fatal(err)
	}

	if err := dw.Present(); err != nil {
		t.Fatal(err)
	}

	img := w.(*window).front

	if got := img.RGBAAt(5, 5); got != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("center pixel = %v, want red", got)
	}

	if got := img.RGBAAt(0, 0); got != (color.RGBA{A: 255}) {
		t.Fatalf("outside pixel = %v, want opaque black", got)
	}
}

func TestDrawCircleClipsToWindow(t *testing.T) {
	b := New()
	w, err := b.Create(backend.WindowConfig{Width: 10, Height: 10})
	if err != nil {
		t.Fatal(err)
	}

	dw := w.(backend.DrawingWindow)

	if err := dw.BeginFrame(backend.Color{A: 255}); err != nil {
		t.Fatal(err)
	}

	if err := dw.DrawCircle(0, 0, 3, backend.Color{
		G: 255, A: 255,
	}); err != nil {
		t.Fatal(err)
	}

	if err := dw.Present(); err != nil {
		t.Fatal(err)
	}

	img := w.(*window).front

	if got := img.RGBAAt(0, 0); got != (color.RGBA{G: 255, A: 255}) {
		t.Fatalf("corner pixel = %v, want green", got)
	}

	if got := img.RGBAAt(9, 9); got != (color.RGBA{A: 255}) {
		t.Fatalf("far pixel = %v, want opaque black", got)
	}
}

func TestDrawCircleRejectsInvalidRadius(t *testing.T) {
	b := New()
	w, err := b.Create(backend.WindowConfig{Width: 10, Height: 10})
	if err != nil {
		t.Fatal(err)
	}

	dw := w.(backend.DrawingWindow)

	if err := dw.DrawCircle(5, 5, 0, backend.Color{A: 255}); err == nil {
		t.Fatal("expected error for zero radius")
	}
}
