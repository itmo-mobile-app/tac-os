package detector

import (
	"math"
	"sync"
	"testing"

	"tac-os/marker-detect/marker"
	"tac-os/marker-detect/pose"
)

const (
	frameW      = 1280
	frameH      = 720
	hfovDeg     = 60.0
	markerSizeM = 0.10

	angleToleranceDeg = 1.0
	distanceTolerance = 0.05 // relative
)

// placement is a marker seen by the camera: its center is at the given angle and distance from the camera
// (in the horizontal plane) and it is rotated by yaw around its vertical axis.
type placement struct {
	id                          int
	angleDeg, distanceM, yawDeg float64
}

func rad(deg float64) float64 { return deg * math.Pi / 180 }

// renderScene draws what the camera would see: a white frame with the markers on it. The picture is
// computed by tracing a ray through every pixel to the marker plane, so it does not depend on OpenCV and on
// the pose code under test.
func renderScene(t *testing.T, cam pose.Camera, ps []placement) GrayFrame {
	t.Helper()
	pix := make([]byte, frameW*frameH)
	for i := range pix {
		pix[i] = 255
	}
	for _, p := range ps {
		src, err := RenderMarker(p.id, 400)
		if err != nil {
			t.Fatalf("RenderMarker(%d): %v", p.id, err)
		}
		drawMarker(pix, src, cam, p)
	}
	return GrayFrame{Width: frameW, Height: frameH, Pix: pix}
}

func drawMarker(pix []byte, src GrayFrame, cam pose.Camera, p placement) {
	a, yaw := rad(p.angleDeg), rad(p.yawDeg)
	center := [3]float64{p.distanceM * math.Sin(a), 0, p.distanceM * math.Cos(a)}
	e1 := [3]float64{math.Cos(yaw), 0, -math.Sin(yaw)} // marker x axis (to the right)
	e2 := [3]float64{0, 1, 0}                          // marker y axis (down)
	e3 := [3]float64{math.Sin(yaw), 0, math.Cos(yaw)}  // normal of the marker plane
	half := markerSizeM / 2

	// Bounding box of the marker in the image.
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, c := range [4][2]float64{{-half, -half}, {half, -half}, {half, half}, {-half, half}} {
		x := center[0] + c[0]*e1[0] + c[1]*e2[0]
		y := center[1] + c[0]*e1[1] + c[1]*e2[1]
		z := center[2] + c[0]*e1[2] + c[1]*e2[2]
		u, v := cam.Cx+cam.Fx*x/z, cam.Cy+cam.Fy*y/z
		minX, maxX = math.Min(minX, u), math.Max(maxX, u)
		minY, maxY = math.Min(minY, v), math.Max(maxY, v)
	}
	x0, x1 := max(int(minX)-2, 0), min(int(maxX)+2, frameW-1)
	y0, y1 := max(int(minY)-2, 0), min(int(maxY)+2, frameH-1)

	centerDotE3 := center[0]*e3[0] + center[1]*e3[1] + center[2]*e3[2]
	const sub = 3 // sub-samples per pixel side, to imitate the averaging of a real sensor
	for row := y0; row <= y1; row++ {
		for col := x0; col <= x1; col++ {
			sum := 0.0
			for sy := 0; sy < sub; sy++ {
				for sx := 0; sx < sub; sx++ {
					u := float64(col) + (float64(sx)+0.5)/sub - 0.5
					v := float64(row) + (float64(sy)+0.5)/sub - 0.5
					d := [3]float64{(u - cam.Cx) / cam.Fx, (v - cam.Cy) / cam.Fy, 1}
					denom := d[0]*e3[0] + d[1]*e3[1] + d[2]*e3[2]
					if denom <= 1e-9 {
						sum += 255
						continue
					}
					k := centerDotE3 / denom
					rel := [3]float64{k*d[0] - center[0], k*d[1] - center[1], k*d[2] - center[2]}
					la := rel[0]*e1[0] + rel[1]*e1[1] + rel[2]*e1[2]
					lb := rel[0]*e2[0] + rel[1]*e2[1] + rel[2]*e2[2]
					sum += sample(src, (la/markerSizeM+0.5)*float64(src.Width)-0.5, (lb/markerSizeM+0.5)*float64(src.Height)-0.5)
				}
			}
			value := byte(sum/(sub*sub) + 0.5)
			if i := row*frameW + col; value < pix[i] {
				pix[i] = value
			}
		}
	}
}

// sample is a bilinear sample of the marker image at pixel coordinates (x, y); outside the marker it is white.
func sample(img GrayFrame, x, y float64) float64 {
	if x < -0.5 || y < -0.5 || x > float64(img.Width)-0.5 || y > float64(img.Height)-0.5 {
		return 255
	}
	x = math.Min(math.Max(x, 0), float64(img.Width-1))
	y = math.Min(math.Max(y, 0), float64(img.Height-1))
	ix, iy := int(x), int(y)
	jx, jy := min(ix+1, img.Width-1), min(iy+1, img.Height-1)
	fx, fy := x-float64(ix), y-float64(iy)
	at := func(c, r int) float64 { return float64(img.Pix[r*img.Width+c]) }
	top := at(ix, iy)*(1-fx) + at(jx, iy)*fx
	bottom := at(ix, jy)*(1-fx) + at(jx, jy)*fx
	return top*(1-fy) + bottom*fy
}

func newDetector(t *testing.T) *Detector {
	t.Helper()
	d, err := New(markerSizeM)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func find(rs []Result, id int) *Result {
	for i := range rs {
		if rs[i].ID == id {
			return &rs[i]
		}
	}
	return nil
}

func checkPlacement(t *testing.T, rs []Result, p placement) {
	t.Helper()
	r := find(rs, p.id)
	if r == nil {
		t.Errorf("marker %d (%+v) was not detected", p.id, p)
		return
	}
	t.Logf("id=%d: angle %+.2f (expected %+.2f), distance %.3f (expected %.3f)", r.ID, r.RelativeAngleDeg, p.angleDeg, r.DistanceM, p.distanceM)
	if math.Abs(r.RelativeAngleDeg-p.angleDeg) > angleToleranceDeg {
		t.Errorf("marker %d: angle %.2f, want %.2f +- %.1f", p.id, r.RelativeAngleDeg, p.angleDeg, angleToleranceDeg)
	}
	if math.Abs(r.DistanceM-p.distanceM) > p.distanceM*distanceTolerance {
		t.Errorf("marker %d: distance %.3f, want %.3f +- %.0f%%", p.id, r.DistanceM, p.distanceM, distanceTolerance*100)
	}
}

func TestSingleMarkers(t *testing.T) {
	cam := pose.CameraFromHorizontalFOV(frameW, frameH, hfovDeg)
	d := newDetector(t)
	cases := []placement{
		{17, 0, 1.0, 0},
		{0, -15, 1.5, 0},
		{71, 20, 2.0, 0},
		{35, 5, 1.2, 30},
	}
	for _, p := range cases {
		rs, err := d.Detect(renderScene(t, cam, []placement{p}), cam)
		if err != nil {
			t.Fatal(err)
		}
		if len(rs) != 1 {
			t.Errorf("%+v: got %d detections, want 1", p, len(rs))
		}
		checkPlacement(t, rs, p)
	}
}

func TestSeveralMarkers(t *testing.T) {
	cam := pose.CameraFromHorizontalFOV(frameW, frameH, hfovDeg)
	d := newDetector(t)
	ps := []placement{{5, -20, 1.5, 0}, {30, 0, 1.5, 0}, {71, 20, 1.5, 0}}
	rs, err := d.Detect(renderScene(t, cam, ps), cam)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 3 {
		t.Fatalf("got %d detections, want 3", len(rs))
	}
	for _, p := range ps {
		checkPlacement(t, rs, p)
	}
	for i := 1; i < len(rs); i++ {
		if rs[i-1].ID >= rs[i].ID {
			t.Errorf("results are not sorted by id: %d before %d", rs[i-1].ID, rs[i].ID)
		}
	}
}

// Id 80 exists in the ArUco dictionary, but it is not a game marker (game ids are 0..71).
func TestMarkerOutsideGameRangeIsIgnored(t *testing.T) {
	cam := pose.CameraFromHorizontalFOV(frameW, frameH, hfovDeg)
	d := newDetector(t)
	rs, err := d.Detect(renderScene(t, cam, []placement{{80, 0, 1.0, 0}, {12, 15, 1.0, 0}}), cam)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 || find(rs, 12) == nil || find(rs, 80) != nil {
		t.Errorf("got %+v, want only marker 12", rs)
	}
}

func TestNothingToDetect(t *testing.T) {
	cam := pose.CameraFromHorizontalFOV(frameW, frameH, hfovDeg)
	d := newDetector(t)
	for name, value := range map[string]byte{"white": 255, "black": 0, "gray": 128} {
		pix := make([]byte, frameW*frameH)
		for i := range pix {
			pix[i] = value
		}
		rs, err := d.Detect(GrayFrame{Width: frameW, Height: frameH, Pix: pix}, cam)
		if err != nil || len(rs) != 0 {
			t.Errorf("%s frame: got %v, %v; want no detections", name, rs, err)
		}
	}
}

func TestInvalidInput(t *testing.T) {
	cam := pose.CameraFromHorizontalFOV(frameW, frameH, hfovDeg)
	d := newDetector(t)
	good := make([]byte, frameW*frameH)

	if _, err := d.Detect(GrayFrame{}, cam); err == nil {
		t.Error("empty frame: no error")
	}
	if _, err := d.Detect(GrayFrame{Width: frameW, Height: frameH, Pix: good[:100]}, cam); err == nil {
		t.Error("frame with a wrong number of bytes: no error")
	}
	if _, err := d.Detect(GrayFrame{Width: frameW, Height: frameH, Pix: good}, pose.Camera{}); err == nil {
		t.Error("camera without focal length: no error")
	}
	if _, err := New(0); err == nil {
		t.Error("zero marker size: no error")
	}
	if _, err := New(-0.1); err == nil {
		t.Error("negative marker size: no error")
	}
}

func TestClosedDetector(t *testing.T) {
	cam := pose.CameraFromHorizontalFOV(frameW, frameH, hfovDeg)
	d, err := New(markerSizeM)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if _, err := d.Detect(GrayFrame{Width: frameW, Height: frameH, Pix: make([]byte, frameW*frameH)}, cam); err == nil {
		t.Error("Detect after Close: no error")
	}
}

func TestRenderMarker(t *testing.T) {
	img, err := RenderMarker(17, 120)
	if err != nil {
		t.Fatal(err)
	}
	if img.Width != 120 || img.Height != 120 || len(img.Pix) != 120*120 {
		t.Fatalf("got %dx%d with %d bytes", img.Width, img.Height, len(img.Pix))
	}
	// The border of an ArUco marker is black.
	for i := 0; i < 120; i++ {
		if img.Pix[i] != 0 || img.Pix[119*120+i] != 0 || img.Pix[i*120] != 0 || img.Pix[i*120+119] != 0 {
			t.Fatal("the border of the marker is not black")
		}
	}
	for _, bad := range []struct{ id, side int }{{-1, 100}, {100, 100}, {5, 0}, {5, -3}} {
		if _, err := RenderMarker(bad.id, bad.side); err == nil {
			t.Errorf("RenderMarker(%d, %d): no error", bad.id, bad.side)
		}
	}
}

// Calls are serialized by the detector, so it can be shared by goroutines (run with -race).
func TestConcurrentUse(t *testing.T) {
	cam := pose.CameraFromHorizontalFOV(frameW, frameH, hfovDeg)
	d := newDetector(t)
	frame := renderScene(t, cam, []placement{{17, 10, 1.5, 0}})

	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 5; i++ {
				rs, err := d.Detect(frame, cam)
				if err != nil || len(rs) != 1 || rs[0].ID != 17 || !marker.IsValid(rs[0].ID) {
					t.Errorf("concurrent Detect: %v, %v", rs, err)
					return
				}
			}
		}()
	}
	wg.Wait()
}
