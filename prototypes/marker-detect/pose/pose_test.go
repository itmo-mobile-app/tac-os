package pose

import (
	"math"
	"math/rand"
	"testing"
)

const (
	frameW     = 1280
	frameH     = 720
	hfovDeg    = 60.0
	markerSize = 0.10
)

// scene is a marker seen by the camera: its center is at the given angle and distance from the camera
// (in the horizontal plane), shifted vertically by dy, and rotated by yaw around its vertical axis.
type scene struct {
	angleDeg, distance, yawDeg, dy float64
}

// project computes where the corners of the marker appear in the image.
func project(cam Camera, s scene) [4]Point {
	a := s.angleDeg * math.Pi / 180
	yaw := s.yawDeg * math.Pi / 180
	cx3, cz3 := s.distance*math.Sin(a), s.distance*math.Cos(a)
	h := markerSize / 2
	local := [4][2]float64{{-h, -h}, {h, -h}, {h, h}, {-h, h}} // top-left, top-right, bottom-right, bottom-left
	var out [4]Point
	for i, l := range local {
		x := cx3 + l[0]*math.Cos(yaw)
		z := cz3 - l[0]*math.Sin(yaw)
		y := s.dy + l[1]
		out[i] = Point{cam.Cx + cam.Fx*x/z, cam.Cy + cam.Fy*y/z}
	}
	return out
}

func TestEstimatePositionExact(t *testing.T) {
	cam := CameraFromHorizontalFOV(frameW, frameH, hfovDeg)
	cases := []scene{
		{0, 1.0, 0, 0},
		{-15, 1.5, 0, 0},
		{20, 2.0, 0, 0},
		{5, 1.2, 30, 0},
		{-30, 0.8, -45, 0},
		{10, 3.0, 60, 0},
		{12, 1.0, 20, 0.15},
		{0, 1.0, 0, -0.2},
	}
	for _, s := range cases {
		pos, err := EstimatePosition(project(cam, s), markerSize, cam)
		if err != nil {
			t.Errorf("%+v: %v", s, err)
			continue
		}
		if d := math.Abs(pos.AngleDeg() - math.Atan2(math.Sin(s.angleDeg*math.Pi/180)*s.distance, math.Cos(s.angleDeg*math.Pi/180)*s.distance)*180/math.Pi); d > 1e-6 {
			t.Errorf("%+v: angle %.8f, want %.8f", s, pos.AngleDeg(), s.angleDeg)
		}
		wantDist := math.Sqrt(s.distance*s.distance + s.dy*s.dy)
		if math.Abs(pos.Distance()-wantDist) > 1e-6 {
			t.Errorf("%+v: distance %.8f, want %.8f", s, pos.Distance(), wantDist)
		}
		if math.Abs(pos.Y-s.dy) > 1e-6 {
			t.Errorf("%+v: Y %.8f, want %.8f", s, pos.Y, s.dy)
		}
	}
}

// With corner errors of a few tenths of a pixel the result must stay close to the truth.
func TestEstimatePositionWithNoise(t *testing.T) {
	cam := CameraFromHorizontalFOV(frameW, frameH, hfovDeg)
	s := scene{angleDeg: 10, distance: 1.5, yawDeg: 20}
	clean := project(cam, s)
	rng := rand.New(rand.NewSource(1))

	const trials = 500
	const sigma = 0.3 // pixels
	var worstAngle, worstRel, sumRel float64
	for n := 0; n < trials; n++ {
		noisy := clean
		for i := range noisy {
			noisy[i].X += rng.NormFloat64() * sigma
			noisy[i].Y += rng.NormFloat64() * sigma
		}
		pos, err := EstimatePosition(noisy, markerSize, cam)
		if err != nil {
			t.Fatal(err)
		}
		worstAngle = math.Max(worstAngle, math.Abs(pos.AngleDeg()-s.angleDeg))
		rel := math.Abs(pos.Distance()-s.distance) / s.distance
		worstRel = math.Max(worstRel, rel)
		sumRel += rel
	}
	t.Logf("worst angle error %.3f deg; relative distance error: mean %.2f%%, worst %.2f%%",
		worstAngle, sumRel/trials*100, worstRel*100)
	if worstAngle > 1.0 {
		t.Errorf("angle error %.3f deg is too large", worstAngle)
	}
	// Observed worst case is about 5% (typical errors are several times smaller); the limit leaves a margin.
	if worstRel > 0.07 {
		t.Errorf("distance error %.2f%% is too large", worstRel*100)
	}
}

func TestEstimatePositionRejectsBadInput(t *testing.T) {
	cam := CameraFromHorizontalFOV(frameW, frameH, hfovDeg)
	good := project(cam, scene{distance: 1})

	same := [4]Point{{100, 100}, {100, 100}, {100, 100}, {100, 100}}
	line := [4]Point{{100, 100}, {200, 100}, {300, 100}, {400, 100}}
	nan := good
	nan[2].X = math.NaN()
	inf := good
	inf[0].Y = math.Inf(1)

	cases := []struct {
		name    string
		corners [4]Point
		size    float64
		cam     Camera
	}{
		{"all corners equal", same, markerSize, cam},
		{"collinear corners", line, markerSize, cam},
		{"NaN corner", nan, markerSize, cam},
		{"Inf corner", inf, markerSize, cam},
		{"zero marker size", good, 0, cam},
		{"negative marker size", good, -1, cam},
		{"zero focal length", good, markerSize, Camera{}},
	}
	for _, c := range cases {
		if _, err := EstimatePosition(c.corners, c.size, c.cam); err == nil {
			t.Errorf("%s: no error", c.name)
		}
	}
}

// A larger field of view means a shorter focal length, so the same corners mean a shorter distance.
func TestFieldOfViewScalesDistance(t *testing.T) {
	corners := project(CameraFromHorizontalFOV(frameW, frameH, 60), scene{distance: 1})
	narrow, err := EstimatePosition(corners, markerSize, CameraFromHorizontalFOV(frameW, frameH, 50))
	if err != nil {
		t.Fatal(err)
	}
	wide, err := EstimatePosition(corners, markerSize, CameraFromHorizontalFOV(frameW, frameH, 70))
	if err != nil {
		t.Fatal(err)
	}
	if !(narrow.Distance() > 1.0 && wide.Distance() < 1.0) {
		t.Errorf("distance for 50 deg = %.3f, for 70 deg = %.3f; want > 1 and < 1", narrow.Distance(), wide.Distance())
	}
}

func TestPositionAngleSign(t *testing.T) {
	if got := (Position{X: 1, Z: 1}).AngleDeg(); math.Abs(got-45) > 1e-9 {
		t.Errorf("right of the axis: %v, want 45", got)
	}
	if got := (Position{X: -1, Z: 1}).AngleDeg(); math.Abs(got+45) > 1e-9 {
		t.Errorf("left of the axis: %v, want -45", got)
	}
}
