// Package pose estimates where a square marker of known size is, from the pixel coordinates of its
// four corners in a camera image. It is plain Go and does not depend on OpenCV.
//
// Camera axes: X to the right, Y down, Z forward (the optical axis).
package pose

import (
	"errors"
	"fmt"
	"math"
)

// Camera is a pinhole camera without lens distortion, in pixels.
type Camera struct {
	Fx, Fy float64 // focal lengths
	Cx, Cy float64 // principal point
}

// CameraFromHorizontalFOV is an approximate model of a camera with a known horizontal field of view:
// square pixels, principal point in the center of the frame.
func CameraFromHorizontalFOV(width, height int, hfovDeg float64) Camera {
	fx := (float64(width) / 2) / math.Tan(hfovDeg*math.Pi/360)
	return Camera{Fx: fx, Fy: fx, Cx: float64(width) / 2, Cy: float64(height) / 2}
}

// Point is a pixel coordinate.
type Point struct{ X, Y float64 }

// Position is the position of the marker center in camera axes, in meters.
type Position struct{ X, Y, Z float64 }

// AngleDeg is the horizontal angle to the marker: 0 on the optical axis, positive to the right.
func (p Position) AngleDeg() float64 { return math.Atan2(p.X, p.Z) * 180 / math.Pi }

// Distance is the distance from the camera to the marker center.
func (p Position) Distance() float64 { return math.Sqrt(p.X*p.X + p.Y*p.Y + p.Z*p.Z) }

// ErrDegenerate is returned when the corners cannot belong to a square seen from a real camera.
var ErrDegenerate = errors.New("pose: degenerate marker corners")

// EstimatePosition returns the position of the marker center.
//
// corners are the corners of the black square in the order top-left, top-right, bottom-right,
// bottom-left (the order of the ArUco detector); markerSizeM is the side of that square in meters.
//
// The corners define a homography between the marker plane and the image. In normalized camera
// coordinates it equals [h*r1, h*r2, t] up to scale, where h is half the marker size, r1 and r2 are the
// unit axes of the marker plane and t is the position of its center. The scale is fixed by the fact that
// r1 and r2 have unit length.
func EstimatePosition(corners [4]Point, markerSizeM float64, cam Camera) (Position, error) {
	if !(cam.Fx > 0) || !(cam.Fy > 0) || !(markerSizeM > 0) {
		return Position{}, fmt.Errorf("pose: invalid camera or marker size")
	}
	for _, c := range corners {
		if math.IsNaN(c.X) || math.IsNaN(c.Y) || math.IsInf(c.X, 0) || math.IsInf(c.Y, 0) {
			return Position{}, ErrDegenerate
		}
	}

	// Marker plane coordinates in units of half the marker size: top-left, top-right, bottom-right,
	// bottom-left; x to the right, y down.
	plane := [4][2]float64{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}}

	// Homography G (with G[2][2] = 1) from plane coordinates to normalized image coordinates:
	//   x = (g0*a + g1*b + g2) / (g6*a + g7*b + 1)
	//   y = (g3*a + g4*b + g5) / (g6*a + g7*b + 1)
	var m [8][9]float64
	for i := 0; i < 4; i++ {
		a, b := plane[i][0], plane[i][1]
		x := (corners[i].X - cam.Cx) / cam.Fx
		y := (corners[i].Y - cam.Cy) / cam.Fy
		m[2*i] = [9]float64{a, b, 1, 0, 0, 0, -x * a, -x * b, x}
		m[2*i+1] = [9]float64{0, 0, 0, a, b, 1, -y * a, -y * b, y}
	}
	g, ok := solve8(m)
	if !ok {
		return Position{}, ErrDegenerate
	}

	// Columns of G: the first two are h/tz * r1 and h/tz * r2, the third is t/tz.
	n1 := math.Hypot(math.Hypot(g[0], g[3]), g[6])
	n2 := math.Hypot(math.Hypot(g[1], g[4]), g[7])
	if !(n1 > 0) || !(n2 > 0) {
		return Position{}, ErrDegenerate
	}
	h := markerSizeM / 2
	tz := 2 * h / (n1 + n2)
	pos := Position{X: tz * g[2], Y: tz * g[5], Z: tz}
	if math.IsNaN(pos.X) || math.IsNaN(pos.Y) || math.IsInf(pos.X, 0) || math.IsInf(pos.Y, 0) {
		return Position{}, ErrDegenerate
	}
	return pos, nil
}

// solve8 solves an 8x8 linear system given as an augmented matrix, by Gaussian elimination with
// partial pivoting. It reports false for a (nearly) singular system.
func solve8(m [8][9]float64) ([8]float64, bool) {
	const n = 8
	for col := 0; col < n; col++ {
		pivot := col
		for r := col + 1; r < n; r++ {
			if math.Abs(m[r][col]) > math.Abs(m[pivot][col]) {
				pivot = r
			}
		}
		if math.Abs(m[pivot][col]) < 1e-12 {
			return [8]float64{}, false
		}
		m[col], m[pivot] = m[pivot], m[col]
		for r := col + 1; r < n; r++ {
			f := m[r][col] / m[col][col]
			for c := col; c <= n; c++ {
				m[r][c] -= f * m[col][c]
			}
		}
	}
	var x [8]float64
	for r := n - 1; r >= 0; r-- {
		s := m[r][n]
		for c := r + 1; c < n; c++ {
			s -= m[r][c] * x[c]
		}
		x[r] = s / m[r][r]
	}
	return x, true
}
