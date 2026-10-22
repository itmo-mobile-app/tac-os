// Package ipc implements the private Unix-domain socket protocol between the C++ Runtime and the marker
// detector (issue #69). The wire format is fixed in spec/camera-detector-protocol.md: one UTF-8 JSON object
// per line, a single DETECT request and a success or error response.
//
// The package does not depend on OpenCV. Where detections come from is hidden behind Source, so the protocol
// is tested without a camera.
package ipc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"

	"tac-os/marker-detect/marker"
)

// maxFrameBytes is the largest request or response, including the terminating newline.
const maxFrameBytes = 64 << 10

// Wire error codes, see "Failed response" in the specification.
const (
	codeCameraUnavailable = "CAMERA_UNAVAILABLE"
	codeDetectionFailed   = "DETECTION_FAILED"
	codeBadRequest        = "BAD_REQUEST"
)

// ErrCameraUnavailable is returned by a Source when the camera cannot be opened or read. The server reports
// it as CAMERA_UNAVAILABLE. Any other error of a Source is reported as DETECTION_FAILED.
var ErrCameraUnavailable = errors.New("camera unavailable")

// Source supplies the detections for one DETECT request: it captures one current camera frame and analyzes it.
// The server calls Detect for one request at a time.
//
// The result must follow the protocol: unique ids sorted in ascending order, ids in 0..71, finite angles and
// finite distances greater than zero. Normalize helps to get there. The server checks the result and reports
// DETECTION_FAILED instead of sending data that breaks the contract.
type Source interface {
	Detect() ([]marker.Detection, error)
}

// SourceFunc makes a Source from a function.
type SourceFunc func() ([]marker.Detection, error)

// Detect calls f.
func (f SourceFunc) Detect() ([]marker.Detection, error) { return f() }

// parseRequest accepts exactly {"command":"DETECT"}. Keys and the command are case-sensitive, unknown or
// duplicate fields, other value types and data after the object are rejected. encoding/json cannot be used
// for this directly: it matches keys case-insensitively and silently keeps the last of duplicate keys.
func parseRequest(line []byte) error {
	dec := json.NewDecoder(bytes.NewReader(line))

	tok, err := dec.Token()
	if err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return errors.New("the request must be a JSON object")
	}

	command, haveCommand := "", false
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
		key, _ := tok.(string) // inside an object a token that is not a delimiter is always a key
		if key != "command" {
			return fmt.Errorf("unknown field %q", key)
		}
		if haveCommand {
			return errors.New(`duplicate field "command"`)
		}
		tok, err = dec.Token()
		if err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
		value, ok := tok.(string)
		if !ok {
			return errors.New(`field "command" must be a string`)
		}
		command, haveCommand = value, true
	}
	if _, err := dec.Token(); err != nil { // the closing brace
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return errors.New("unexpected data after the JSON object")
	}

	if !haveCommand {
		return errors.New(`missing field "command"`)
	}
	if command != "DETECT" {
		return fmt.Errorf("unknown command %q", command)
	}
	return nil
}

// validate checks the detections against the protocol before they are sent.
func validate(dets []marker.Detection) error {
	for i, d := range dets {
		if !marker.IsValid(d.ID) {
			return fmt.Errorf("detection %d: id %d is outside 0..%d", i, d.ID, marker.Count-1)
		}
		if math.IsNaN(d.RelativeAngleDeg) || math.IsInf(d.RelativeAngleDeg, 0) {
			return fmt.Errorf("detection %d (id %d): angle is not finite", i, d.ID)
		}
		if math.IsNaN(d.DistanceM) || math.IsInf(d.DistanceM, 0) || !(d.DistanceM > 0) {
			return fmt.Errorf("detection %d (id %d): distance must be finite and greater than 0", i, d.ID)
		}
		if i > 0 && dets[i-1].ID >= d.ID {
			return fmt.Errorf("detection %d: ids must be unique and in ascending order", i)
		}
	}
	return nil
}

type detectionJSON struct {
	ID            int     `json:"id"`
	RelativeAngle float64 `json:"relativeAngle"`
	Distance      float64 `json:"distance"`
}

type successJSON struct {
	OK         bool            `json:"ok"`
	Detections []detectionJSON `json:"detections"`
}

type failureJSON struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
}

// encodeSuccess returns the success response line for validated detections.
func encodeSuccess(dets []marker.Detection) []byte {
	out := successJSON{OK: true, Detections: make([]detectionJSON, 0, len(dets))} // [] and not null
	for _, d := range dets {
		angle := d.RelativeAngleDeg
		if angle == 0 {
			angle = 0 // never "-0"
		}
		out.Detections = append(out.Detections, detectionJSON{ID: d.ID, RelativeAngle: angle, Distance: d.DistanceM})
	}
	return encodeLine(out)
}

// encodeFailure returns the error response line for a wire error code.
func encodeFailure(code string) []byte {
	return encodeLine(failureJSON{OK: false, Error: code})
}

func encodeLine(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		// Unreachable: the values are validated and the types are fixed.
		panic("ipc: cannot encode a response: " + err.Error())
	}
	return append(b, '\n')
}
