package tacticalmap

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

const validObservation = `{"targetId":"17","observerId":"device-1","latitude":59.9505,"longitude":30.3095,"confidence":0.85,"timestamp":1792255200000}`
const validTarget = `{"targetId":"17","latitude":59.9505,"longitude":30.3095,"probability":0.9,"lastSeenAt":1792255200000}`

func TestBrokerRecordsRejectInvalidJSONAndFields(t *testing.T) {
	for _, tc := range []struct {
		name, input string
	}{
		{"missing", strings.Replace(validObservation, `"observerId":"device-1",`, "", 1)},
		{"IPC envelope", strings.Replace(validObservation, `{`, `{"type":"OBSERVATION",`, 1)},
		{"duplicate", strings.Replace(validObservation, `{`, `{"latitude":0,`, 1)},
		{"case mismatch", strings.Replace(validObservation, `"latitude"`, `"Latitude"`, 1)},
		{"null", strings.Replace(validObservation, `59.9505`, `null`, 1)},
		{"string coordinate", strings.Replace(validObservation, `59.9505`, `"59.9505"`, 1)},
		{"latitude", strings.Replace(validObservation, `59.9505`, `91`, 1)},
		{"longitude", strings.Replace(validObservation, `30.3095`, `-181`, 1)},
		{"confidence", strings.Replace(validObservation, `0.85`, `1.01`, 1)},
		{"nondecimal ID", strings.Replace(validObservation, `"17"`, `"t1"`, 1)},
		{"empty observer", strings.Replace(validObservation, `"device-1"`, `""`, 1)},
		{"fractional timestamp", strings.Replace(validObservation, `1792255200000`, `1.5`, 1)},
		{"timestamp overflow", strings.Replace(validObservation, `1792255200000`, `9223372036854775808`, 1)},
		{"two objects", validObservation + validObservation},
		{"array", "[" + validObservation + "]"},
		{"truncated", validObservation[:len(validObservation)-1]},
		{"nonfinite", strings.Replace(validObservation, `59.9505`, `1e999`, 1)},
		{"invalid UTF-8", strings.Replace(validObservation, `device-1`, string([]byte{255}), 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseObservation([]byte(tc.input)); err == nil {
				t.Fatal("invalid Observation accepted")
			}
		})
	}
	for _, data := range []string{
		strings.Replace(validTarget, `0.9`, `-0.1`, 1),
		strings.Replace(validTarget, `0.9`, `null`, 1),
		strings.Replace(validTarget, `"17"`, `""`, 1),
		strings.Replace(validTarget, `1792255200000`, `"1792255200000"`, 1),
		strings.Replace(validTarget, `{`, `{"lastSeenAt":0,`, 1),
		strings.Replace(validTarget, `"longitude"`, `"Longitude"`, 1),
		strings.Replace(validTarget, `{`, `{"confidence":0.8,`, 1),
	} {
		if _, err := parseTarget([]byte(data)); err == nil {
			t.Errorf("invalid Target accepted: %s", data)
		}
	}
}

func TestObservationFixtureAndExactPayload(t *testing.T) {
	observations, err := LoadObservations(strings.NewReader("\n" + validObservation + "\n" + validObservation + "\n"))
	if err != nil || len(observations) != 2 {
		t.Fatalf("fixture: %v, %v", observations, err)
	}
	encoded, err := json.Marshal(observations[0])
	if err != nil || string(encoded) != validObservation {
		t.Fatalf("six-field payload changed: %s, %v", encoded, err)
	}
	if o, err := parseObservation([]byte(strings.Replace(validObservation, `1792255200000`, `9223372036854775807`, 1))); err != nil || o.Timestamp != math.MaxInt64 {
		t.Fatalf("int64 timestamp precision lost: %+v, %v", o, err)
	}
	if _, err := LoadObservations(strings.NewReader(validObservation + "\n{}\n")); err == nil {
		t.Fatal("invalid second line accepted")
	}
	if _, err := LoadObservations(strings.NewReader(strings.Repeat("x", maxBrokerFrame))); err == nil {
		t.Fatal("oversized fixture line accepted")
	}
	if target, err := parseTarget([]byte(validTarget)); err != nil || target.Probability != 0.9 {
		t.Fatalf("valid Target rejected: %+v, %v", target, err)
	}
}
