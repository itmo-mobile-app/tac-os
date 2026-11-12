package validation

import (
	"reflect"
	"testing"
)

func TestParseObservation(t *testing.T) {
	valid := `{"targetId":"17","observerId":"A","latitude":0,"longitude":0,"confidence":0.8,"timestamp":1800000000000}`
	tests := []struct {
		name, input string
		wantOK      bool
	}{
		{"valid", valid, true},
		{"zero confidence", `{"targetId":"17","observerId":"A","latitude":0,"longitude":0,"confidence":0,"timestamp":1800000000000}`, true},
		{"invalid latitude", `{"targetId":"17","observerId":"A","latitude":91,"longitude":0,"confidence":0.8,"timestamp":1800000000000}`, false},
		{"invalid longitude", `{"targetId":"17","observerId":"A","latitude":0,"longitude":181,"confidence":0.8,"timestamp":1800000000000}`, false},
		{"invalid confidence", `{"targetId":"17","observerId":"A","latitude":0,"longitude":0,"confidence":1.2,"timestamp":1800000000000}`, false},
		{"missing field", `{"targetId":"17","observerId":"A","latitude":0,"longitude":0,"confidence":0.8}`, false},
		{"extra field", `{"targetId":"17","observerId":"A","latitude":0,"longitude":0,"confidence":0.8,"timestamp":1800000000000,"extra":1}`, false},
		{"duplicate key", `{"targetId":"17","targetId":"18","observerId":"A","latitude":0,"longitude":0,"confidence":0.8,"timestamp":1800000000000}`, false},
		{"fractional timestamp", `{"targetId":"17","observerId":"A","latitude":0,"longitude":0,"confidence":0.8,"timestamp":1.5}`, false},
		{"overflow timestamp", `{"targetId":"17","observerId":"A","latitude":0,"longitude":0,"confidence":0.8,"timestamp":9223372036854775808}`, false},
		{"malformed", `{"targetId":`, false},
		{"trailing", valid + ` {}`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseObservation([]byte(tc.input))
			if (err == nil) != tc.wantOK {
				t.Fatalf("got=%+v err=%v wantOK=%v", got, err, tc.wantOK)
			}
			if tc.wantOK {
				again, err := ParseObservation([]byte(tc.input))
				if err != nil || !reflect.DeepEqual(got, again) {
					t.Fatalf("not deterministic: %+v %+v %v", got, again, err)
				}
			}
		})
	}
}
