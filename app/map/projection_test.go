package tacticalmap

import (
	"image"
	"math"
	"testing"
)

func TestProjectionOrientationAndBounds(t *testing.T) {
	bounds := Bounds{LatMin: 10, LatMax: 12, LonMin: 20, LonMax: 24}
	for _, test := range []struct {
		name     string
		lat, lon float64
		want     image.Point
	}{
		{"north-west", 12, 20, image.Pt(0, 0)},
		{"north-east", 12, 24, image.Pt(959, 0)},
		{"south-west", 10, 20, image.Pt(0, 639)},
		{"south-east", 10, 24, image.Pt(959, 639)},
		{"center", 11, 22, image.Pt(480, 320)},
		{"off-center", 11.5, 21, image.Pt(240, 160)},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, visible, err := Project(bounds, 960, 640, test.lat, test.lon)
			if err != nil || !visible || got != test.want {
				t.Fatalf("got %v, visible=%v, err=%v; want %v", got, visible, err, test.want)
			}
		})
	}
	for _, point := range [][2]float64{{12.01, 22}, {9.99, 22}, {11, 19.99}, {11, 24.01}} {
		if _, visible, err := Project(bounds, 960, 640, point[0], point[1]); err != nil || visible {
			t.Fatalf("outside point %v: visible=%v err=%v", point, visible, err)
		}
	}
}

func TestProjectionRejectsInvalidInput(t *testing.T) {
	valid := Bounds{LatMin: 10, LatMax: 12, LonMin: 20, LonMax: 24}
	for _, bounds := range []Bounds{
		{10, 10, 20, 24}, {12, 10, 20, 24}, {10, 12, 24, 20},
		{-91, 12, 20, 24}, {10, 12, 20, 181}, {10, math.NaN(), 20, 24},
	} {
		if _, _, err := Project(bounds, 960, 640, 11, 22); err == nil {
			t.Errorf("accepted invalid bounds %+v", bounds)
		}
	}
	for _, dimensions := range [][2]int{{0, 640}, {960, -1}, {8193, 640}} {
		if _, _, err := Project(valid, dimensions[0], dimensions[1], 11, 22); err == nil {
			t.Errorf("accepted invalid dimensions %v", dimensions)
		}
	}
	for _, point := range [][2]float64{{math.NaN(), 22}, {11, math.Inf(1)}, {91, 22}, {11, -181}} {
		if _, _, err := Project(valid, 960, 640, point[0], point[1]); err == nil {
			t.Errorf("accepted invalid point %v", point)
		}
	}
}
