package tacticalmap

import (
	"image"
	"image/color"
	"math"
	"testing"
)

func testConfig(t *testing.T) Config {
	t.Helper()
	cfg, err := LoadConfig("assets/test-map.json")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestFramePositionsProbabilityAndOutsideTarget(t *testing.T) {
	cfg := testConfig(t)
	commands, markers, err := BuildFrame(cfg, DemoTargets())
	if err != nil {
		t.Fatal(err)
	}
	if len(markers) != 3 || len(commands) != 9 {
		t.Fatalf("markers=%d commands=%d", len(markers), len(commands))
	}
	want := []image.Point{{X: 240, Y: 160}, {X: 480, Y: 320}, {X: 720, Y: 480}}
	for i, marker := range markers {
		if marker.Pixel != want[i] {
			t.Errorf("%s at %v, want %v", marker.Target.TargetID, marker.Pixel, want[i])
		}
	}
	img, err := RenderPreview(cfg, commands)
	if err != nil {
		t.Fatal(err)
	}
	var previous uint8
	for i, point := range want {
		pixel := color.NRGBAModel.Convert(img.At(point.X, point.Y)).(color.NRGBA)
		if i > 0 && pixel.G <= previous {
			t.Errorf("lower probability must be visibly paler: previous green=%d, current=%d", previous, pixel.G)
		}
		previous = pixel.G
	}
	if commands[0].Command != "BEGIN_FRAME" || commands[len(commands)-1].Command != "PRESENT" {
		t.Fatal("incorrect frame boundaries")
	}
}

func TestFrameRejectsInvalidTargets(t *testing.T) {
	cfg := testConfig(t)
	for _, mutate := range []func([]Target){
		func(targets []Target) { targets[0].Probability = math.NaN() },
		func(targets []Target) { targets[0].Probability = -0.1 },
		func(targets []Target) { targets[0].Probability = 1.1 },
		func(targets []Target) { targets[0].Latitude = math.Inf(1) },
		func(targets []Target) { targets[0].TargetID = "" },
		func(targets []Target) { targets[1].TargetID = targets[0].TargetID },
	} {
		targets := DemoTargets()
		mutate(targets)
		if _, _, err := BuildFrame(cfg, targets); err == nil {
			t.Error("invalid target accepted")
		}
	}
}

func TestPreviewClipsEdgeMarkersAndChecksFrame(t *testing.T) {
	cfg := testConfig(t)
	targets := []Target{{TargetID: "edge", Latitude: cfg.Bounds.LatMin, Longitude: cfg.Bounds.LonMax, Probability: 0}}
	commands, markers, err := BuildFrame(cfg, targets)
	if err != nil {
		t.Fatal(err)
	}
	if markers[0].Pixel != image.Pt(959, 639) {
		t.Fatalf("wrong edge position: %v", markers[0].Pixel)
	}
	if _, err := RenderPreview(cfg, commands); err != nil {
		t.Fatal(err)
	}
	if _, err := RenderPreview(cfg, commands[:len(commands)-1]); err == nil {
		t.Fatal("accepted missing PRESENT")
	}
	if _, err := RenderPreview(cfg, commands[1:]); err == nil {
		t.Fatal("accepted missing BEGIN_FRAME")
	}
}
