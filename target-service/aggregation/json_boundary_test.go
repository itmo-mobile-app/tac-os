package aggregation_test

import (
	"testing"

	"tac-target-service/aggregation"
	"tac-target-service/validation"
)

const jt0 int64 = 1800000000000

// JSON boundary tests: malformed JSON, missing fields, wrong types, and unknown
// fields are all rejected by validation.ParseObservation before reaching
// aggregation. The aggregation package itself consumes parsed structs only.
func TestJSONBoundaryRejects(t *testing.T) {
	valid := `{"targetId":"17","observerId":"A","latitude":0,"longitude":0,"confidence":0.8,"timestamp":1800000000000}`
	if _, err := validation.ParseObservation([]byte(valid)); err != nil {
		t.Fatalf("valid JSON rejected: %v", err)
	}

	bad := []struct {
		name string
		raw  string
	}{
		{"malformed", `{"targetId":`},
		{"not an object", `[]`},
		{"empty", ``},
		{"missing targetId", `{"observerId":"A","latitude":0,"longitude":0,"confidence":0.8,"timestamp":1800000000000}`},
		{"missing observerId", `{"targetId":"17","latitude":0,"longitude":0,"confidence":0.8,"timestamp":1800000000000}`},
		{"missing latitude", `{"targetId":"17","observerId":"A","longitude":0,"confidence":0.8,"timestamp":1800000000000}`},
		{"missing longitude", `{"targetId":"17","observerId":"A","latitude":0,"confidence":0.8,"timestamp":1800000000000}`},
		{"missing confidence", `{"targetId":"17","observerId":"A","latitude":0,"longitude":0,"timestamp":1800000000000}`},
		{"missing timestamp", `{"targetId":"17","observerId":"A","latitude":0,"longitude":0,"confidence":0.8}`},
		{"latitude as string", `{"targetId":"17","observerId":"A","latitude":"0","longitude":0,"confidence":0.8,"timestamp":1800000000000}`},
		{"timestamp as float", `{"targetId":"17","observerId":"A","latitude":0,"longitude":0,"confidence":0.8,"timestamp":1.5}`},
		{"targetId as number", `{"targetId":17,"observerId":"A","latitude":0,"longitude":0,"confidence":0.8,"timestamp":1800000000000}`},
		{"confidence as bool", `{"targetId":"17","observerId":"A","latitude":0,"longitude":0,"confidence":true,"timestamp":1800000000000}`},
		{"unknown field", `{"targetId":"17","observerId":"A","latitude":0,"longitude":0,"confidence":0.8,"timestamp":1800000000000,"extra":1}`},
		{"duplicate key", `{"targetId":"17","targetId":"18","observerId":"A","latitude":0,"longitude":0,"confidence":0.8,"timestamp":1800000000000}`},
		{"trailing JSON", valid + ` {}`},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := validation.ParseObservation([]byte(tc.raw)); err == nil {
				t.Fatalf("expected parse error for %q", tc.raw)
			}
		})
	}
}

// End-to-end: valid JSON flows through validation into aggregation.
func TestJSONValidFlowsIntoAggregation(t *testing.T) {
	raw := `{"targetId":"17","observerId":"A","latitude":0,"longitude":0,"confidence":0.8,"timestamp":1800000000000}`
	o, err := validation.ParseObservation([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	agg := aggregation.NewAggregator()
	got, err := agg.ProcessObservation(aggregation.Observation{
		TargetID:   o.TargetID,
		ObserverID: o.ObserverID,
		Latitude:   o.Latitude,
		Longitude:  o.Longitude,
		Confidence: o.Confidence,
		Timestamp:  o.Timestamp,
	}, jt0)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected a target")
	}
	if got.Probability < 0.79 || got.Probability > 0.81 {
		t.Fatalf("unexpected probability: %v", got.Probability)
	}
	if got.LastSeenAt != jt0 {
		t.Fatalf("unexpected lastSeenAt: %d", got.LastSeenAt)
	}
}
