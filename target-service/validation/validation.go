// Package validation provides strict parsing of the shared Observation JSON shape.
// It does not aggregate observations or infer target positions.
package validation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

// Observation matches the six fields of spec/data-model.md.
type Observation struct {
	TargetID   string  `json:"targetId"`
	ObserverID string  `json:"observerId"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	Confidence float64 `json:"confidence"`
	Timestamp  int64   `json:"timestamp"`
}

// ParseObservation rejects unknown/missing fields, duplicate keys, invalid
// numeric types, out-of-range values, and trailing JSON.
func ParseObservation(data []byte) (Observation, error) {
	var zero Observation
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return zero, fmt.Errorf("read JSON: %w", err)
	}
	if tok != json.Delim('{') {
		return zero, errors.New("observation must be an object")
	}
	seen := map[string]bool{}
	raw := map[string]json.RawMessage{}
	allowed := map[string]bool{"targetId": true, "observerId": true, "latitude": true, "longitude": true, "confidence": true, "timestamp": true}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return zero, err
		}
		key, ok := tok.(string)
		if !ok {
			return zero, errors.New("invalid key")
		}
		if !allowed[key] {
			return zero, fmt.Errorf("unknown field %q", key)
		}
		if seen[key] {
			return zero, fmt.Errorf("duplicate field %q", key)
		}
		seen[key] = true
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return zero, err
		}
		raw[key] = v
	}
	if _, err := dec.Token(); err != nil {
		return zero, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return zero, errors.New("trailing JSON")
	}
	for key := range allowed {
		if !seen[key] {
			return zero, fmt.Errorf("missing field %q", key)
		}
	}
	var o Observation
	if err := json.Unmarshal(data, &o); err != nil {
		return zero, err
	}
	if strings.TrimSpace(o.TargetID) == "" || strings.TrimSpace(o.ObserverID) == "" {
		return zero, errors.New("empty identifier")
	}
	if !finite(o.Latitude) || o.Latitude < -90 || o.Latitude > 90 {
		return zero, errors.New("invalid latitude")
	}
	if !finite(o.Longitude) || o.Longitude < -180 || o.Longitude > 180 {
		return zero, errors.New("invalid longitude")
	}
	if !finite(o.Confidence) || o.Confidence < 0 || o.Confidence > 1 {
		return zero, errors.New("invalid confidence")
	}
	_ = raw
	return o, nil
}
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
