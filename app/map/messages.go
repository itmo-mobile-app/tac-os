package tacticalmap

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf8"
)

// Observation is the six-field Broker payload, without the Analyzer IPC envelope.
type Observation struct {
	TargetID   string  `json:"targetId"`
	ObserverID string  `json:"observerId"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	Confidence float64 `json:"confidence"`
	Timestamp  int64   `json:"timestamp"`
}

func validCoordinates(latitude, longitude float64) bool {
	return finite(latitude) && latitude >= -90 && latitude <= 90 &&
		finite(longitude) && longitude >= -180 && longitude <= 180
}

func (o Observation) validate() error {
	if o.TargetID == "" || o.ObserverID == "" {
		return fmt.Errorf("targetId and observerId must be nonempty")
	}
	for _, c := range o.TargetID {
		if c < '0' || c > '9' {
			return fmt.Errorf("Observation targetId must be a decimal marker ID string")
		}
	}
	if !validCoordinates(o.Latitude, o.Longitude) || !finite(o.Confidence) || o.Confidence < 0 || o.Confidence > 1 {
		return fmt.Errorf("invalid Observation coordinates or confidence")
	}
	return nil
}

// decodeRecord rejects missing, extra, case-mismatched, duplicate and null fields.
// Checking keys explicitly also avoids encoding/json's case-insensitive matching.
func decodeRecord(data []byte, fields []string, value any) error {
	if !utf8.Valid(data) {
		return fmt.Errorf("record must be UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	first, err := d.Token()
	if err != nil || first != json.Delim('{') {
		return fmt.Errorf("expected a JSON object")
	}
	remaining := make(map[string]bool, len(fields))
	for _, field := range fields {
		remaining[field] = true
	}
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return err
		}
		name, ok := key.(string)
		if !ok || !remaining[name] {
			return fmt.Errorf("unexpected or duplicate field %q", key)
		}
		delete(remaining, name)
		var raw json.RawMessage
		if err := d.Decode(&raw); err != nil {
			return err
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("field %s must not be null", name)
		}
	}
	if _, err := d.Token(); err != nil {
		return err
	}
	if len(remaining) != 0 {
		return fmt.Errorf("record has missing fields")
	}
	if _, err := d.Token(); err != io.EOF {
		return fmt.Errorf("expected exactly one JSON object")
	}
	return json.Unmarshal(data, value)
}

func parseObservation(data []byte) (Observation, error) {
	var o Observation
	if err := decodeRecord(data, []string{"targetId", "observerId", "latitude", "longitude", "confidence", "timestamp"}, &o); err != nil {
		return o, err
	}
	return o, o.validate()
}

func parseTarget(data []byte) (Target, error) {
	var target Target
	if err := decodeRecord(data, []string{"targetId", "latitude", "longitude", "probability", "lastSeenAt"}, &target); err != nil {
		return target, err
	}
	if target.TargetID == "" || !validCoordinates(target.Latitude, target.Longitude) ||
		!finite(target.Probability) || target.Probability < 0 || target.Probability > 1 {
		return target, fmt.Errorf("invalid Target ID, coordinates or probability")
	}
	return target, nil
}

// LoadObservations reads a finite JSONL fixture for this Go prototype's CLI.
// It validates the entire input before anything is published; blank lines are ignored.
func LoadObservations(r io.Reader) ([]Observation, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), maxBrokerFrame)
	var observations []Observation
	for line := 1; scanner.Scan(); line++ {
		if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
			continue
		}
		o, err := parseObservation(scanner.Bytes())
		if err != nil {
			return nil, fmt.Errorf("Observation line %d: %w", line, err)
		}
		observations = append(observations, o)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read Observations: %w", err)
	}
	return observations, nil
}
