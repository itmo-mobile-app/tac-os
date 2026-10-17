package tacticalmap

// Target mirrors spec/data-model.md. Probability is supplied by Target Service;
// Map only displays it. Timestamps are Unix milliseconds.
type Target struct {
	TargetID    string  `json:"targetId"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Probability float64 `json:"probability"`
	LastSeenAt  int64   `json:"lastSeenAt"`
}

// DemoTargets returns three visible synthetic targets and one
// outside the test map. These are not real player locations.
func DemoTargets() []Target {
	return []Target{
		{TargetID: "t1", Latitude: 59.9505, Longitude: 30.3095, Probability: 0.90, LastSeenAt: 1792022400000},
		{TargetID: "t2", Latitude: 59.9500, Longitude: 30.3110, Probability: 0.55, LastSeenAt: 1792022400000},
		{TargetID: "t3", Latitude: 59.9495, Longitude: 30.3125, Probability: 0.20, LastSeenAt: 1792022400000},
		{TargetID: "outside", Latitude: 59.9520, Longitude: 30.3110, Probability: 0.80, LastSeenAt: 1792022400000},
	}
}
