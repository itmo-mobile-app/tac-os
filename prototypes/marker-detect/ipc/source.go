package ipc

import (
	"fmt"
	"sort"

	"tac-os/marker-detect/marker"
)

// Test modes of the detector, see NewFakeSource.
const (
	FakeTwo               = "two"
	FakeEmpty             = "empty"
	FakeCameraUnavailable = "camera-unavailable"
	FakeDetectionFailed   = "detection-failed"
)

// FakeModes lists the modes accepted by NewFakeSource.
var FakeModes = []string{FakeTwo, FakeEmpty, FakeCameraUnavailable, FakeDetectionFailed}

// NewFakeSource returns a Source with deterministic synthetic results, which needs no camera and no OpenCV. The
// modes are the shared fixtures of the contract tests (see "Contract verification" in the specification):
//
//	two                two ordered detections: 17 (12.5 degrees, 3.2 m) and 23 (-8.4 degrees, 5.7 m)
//	empty              no markers: a success with an empty list
//	camera-unavailable the camera error
//	detection-failed   the frame processing error
func NewFakeSource(mode string) (Source, error) {
	switch mode {
	case FakeTwo:
		return SourceFunc(func() ([]marker.Detection, error) {
			return []marker.Detection{
				{ID: 17, RelativeAngleDeg: 12.5, DistanceM: 3.2},
				{ID: 23, RelativeAngleDeg: -8.4, DistanceM: 5.7},
			}, nil
		}), nil
	case FakeEmpty:
		return SourceFunc(func() ([]marker.Detection, error) { return nil, nil }), nil
	case FakeCameraUnavailable:
		return SourceFunc(func() ([]marker.Detection, error) { return nil, ErrCameraUnavailable }), nil
	case FakeDetectionFailed:
		return SourceFunc(func() ([]marker.Detection, error) { return nil, fmt.Errorf("synthetic detection failure") }), nil
	}
	return nil, fmt.Errorf("unknown fake mode %q, expected one of %v", mode, FakeModes)
}

// Normalize brings raw detector output to the form the protocol requires: one detection per id, sorted by id.
// When the same marker is visible more than once, the nearest copy is kept. The input is not modified.
func Normalize(in []marker.Detection) []marker.Detection {
	nearest := make(map[int]marker.Detection, len(in))
	for _, d := range in {
		if prev, ok := nearest[d.ID]; !ok || d.DistanceM < prev.DistanceM {
			nearest[d.ID] = d
		}
	}
	out := make([]marker.Detection, 0, len(nearest))
	for _, d := range nearest {
		out = append(out, d)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].ID < out[b].ID })
	return out
}
