// Package detector recognizes game markers in a grayscale camera frame (issue #34).
//
// It combines the ArUco detector of OpenCV (through gocv) with the pose estimation of package pose and
// returns, for every game marker, its id, the horizontal angle relative to the center of the frame and the
// distance. The public interface does not expose OpenCV types.
package detector

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"gocv.io/x/gocv"

	"tac-os/marker-detect/marker"
	"tac-os/marker-detect/pose"
)

// dictionarySize is the number of markers in DICT_4X4_100.
const dictionarySize = 100

// cornerRefineSubpix is CORNER_REFINE_SUBPIX of OpenCV: sub-pixel corners make the distance more accurate.
const cornerRefineSubpix = 1

// GrayFrame is an 8-bit grayscale image with tightly packed rows: len(Pix) == Width*Height.
type GrayFrame struct {
	Width, Height int
	Pix           []byte
}

// Result is a detection together with the corners of the marker in the frame (for a debug preview).
type Result struct {
	marker.Detection
	Corners [4]pose.Point // top-left, top-right, bottom-right, bottom-left, in pixels
}

// Detector recognizes markers. It is safe for concurrent use, but calls are serialized.
type Detector struct {
	mu          sync.Mutex
	markerSizeM float64
	aruco       gocv.ArucoDetector
	closed      bool
}

// New creates a detector for printed markers whose black square has the given side in meters.
func New(markerSizeM float64) (*Detector, error) {
	if !(markerSizeM > 0) {
		return nil, errors.New("detector: marker size must be positive")
	}
	params := gocv.NewArucoDetectorParameters()
	params.SetCornerRefinementMethod(cornerRefineSubpix)
	dictionary := gocv.GetPredefinedDictionary(gocv.ArucoDict4x4_100)
	return &Detector{
		markerSizeM: markerSizeM,
		aruco:       gocv.NewArucoDetectorWithParams(dictionary, params),
	}, nil
}

// Close releases the OpenCV objects. The detector must not be used afterwards.
func (d *Detector) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	return d.aruco.Close()
}

// Detect returns the game markers found in the frame, sorted by id. Markers with ids outside
// 0..marker.Count-1 are ignored. An error is returned for an invalid frame or camera.
func (d *Detector) Detect(frame GrayFrame, cam pose.Camera) ([]Result, error) {
	if frame.Width <= 0 || frame.Height <= 0 || len(frame.Pix) != frame.Width*frame.Height {
		return nil, fmt.Errorf("detector: invalid frame %dx%d with %d bytes", frame.Width, frame.Height, len(frame.Pix))
	}
	if !(cam.Fx > 0) || !(cam.Fy > 0) {
		return nil, errors.New("detector: invalid camera model")
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, errors.New("detector: closed")
	}

	// The Mat owns its memory (gocv.NewMatFromBytes would only point into Go memory).
	img := gocv.NewMatWithSize(frame.Height, frame.Width, gocv.MatTypeCV8UC1)
	defer img.Close()
	data, err := img.DataPtrUint8()
	if err != nil {
		return nil, fmt.Errorf("detector: %w", err)
	}
	copy(data, frame.Pix)

	corners, ids, _ := d.aruco.DetectMarkers(img)

	results := make([]Result, 0, len(ids))
	for i, id := range ids {
		if !marker.IsValid(id) || len(corners[i]) != 4 {
			continue
		}
		var pts [4]pose.Point
		for k, c := range corners[i] {
			pts[k] = pose.Point{X: float64(c.X), Y: float64(c.Y)}
		}
		pos, err := pose.EstimatePosition(pts, d.markerSizeM, cam)
		if err != nil {
			continue
		}
		results = append(results, Result{
			Detection: marker.Detection{ID: id, RelativeAngleDeg: pos.AngleDeg(), DistanceM: pos.Distance()},
			Corners:   pts,
		})
	}
	sort.Slice(results, func(a, b int) bool { return results[a].ID < results[b].ID })
	return results, nil
}

// RenderMarker draws the marker with the given id as a square image of sidePx pixels: the black square
// without a white margin. id must be in the ArUco dictionary (0..99); game ids are 0..marker.Count-1.
func RenderMarker(id, sidePx int) (GrayFrame, error) {
	if id < 0 || id >= dictionarySize || sidePx <= 0 {
		return GrayFrame{}, fmt.Errorf("detector: invalid marker id %d or size %d", id, sidePx)
	}
	img := gocv.NewMat()
	defer img.Close()
	// Older gocv versions return nothing here and newer ones return an error, so the result is checked below.
	gocv.ArucoGenerateImageMarker(gocv.ArucoDict4x4_100, id, sidePx, img, 1)
	if img.Empty() || img.Rows() != sidePx || img.Cols() != sidePx {
		return GrayFrame{}, errors.New("detector: cannot render the marker")
	}
	return GrayFrame{Width: img.Cols(), Height: img.Rows(), Pix: img.ToBytes()}, nil
}
