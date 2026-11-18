package aggregation

import (
	"math"
	"testing"
)

const t0 int64 = 1800000000000

const eps = 1e-9

func near(a, b float64) bool { return math.Abs(a-b) <= eps }

func obs(observer string, conf, lat, lon float64, ts int64) Observation {
	return Observation{
		TargetID:   "17",
		ObserverID: observer,
		Latitude:   lat,
		Longitude:  lon,
		Confidence: conf,
		Timestamp:  ts,
	}
}

func assertTarget(t *testing.T, got *Target, wantTargetID string, wantP, wantLat, wantLon float64, wantLast int64) {
	t.Helper()
	if got == nil {
		t.Fatalf("expected Target, got nil")
	}
	if got.TargetID != wantTargetID {
		t.Fatalf("targetId = %q, want %q", got.TargetID, wantTargetID)
	}
	if !near(got.Probability, wantP) {
		t.Fatalf("probability = %v, want %v", got.Probability, wantP)
	}
	if !near(got.Latitude, wantLat) {
		t.Fatalf("latitude = %v, want %v", got.Latitude, wantLat)
	}
	if !near(got.Longitude, wantLon) {
		t.Fatalf("longitude = %v, want %v", got.Longitude, wantLon)
	}
	if got.LastSeenAt != wantLast {
		t.Fatalf("lastSeenAt = %d, want %d", got.LastSeenAt, wantLast)
	}
}

func TestOneObserver(t *testing.T) {
	a := NewAggregator()
	got, err := a.ProcessObservation(obs("A", 0.8, 0, 0, t0), t0)
	if err != nil {
		t.Fatal(err)
	}
	assertTarget(t, got, "17", 0.8, 0, 0, t0)
}

func TestTwoIndependentObservers(t *testing.T) {
	a := NewAggregator()
	_, err := a.ProcessObservation(obs("A", 0.8, 0, 0, t0), t0)
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.ProcessObservation(obs("B", 0.6, 0, 0, t0), t0)
	if err != nil {
		t.Fatal(err)
	}
	assertTarget(t, got, "17", 0.92, 0, 0, t0)
}

func TestDuplicateReplay(t *testing.T) {
	a := NewAggregator()
	_, _ = a.ProcessObservation(obs("A", 0.8, 0, 0, t0), t0)
	_, _ = a.ProcessObservation(obs("B", 0.6, 0, 0, t0), t0)
	got, err := a.ProcessObservation(obs("A", 0.8, 0, 0, t0), t0)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected nil for duplicate, got %+v", got)
	}
	tgts, _ := a.RecalculateAll(t0)
	if len(tgts) != 1 || !near(tgts[0].Probability, 0.92) {
		t.Fatalf("state changed after duplicate: %+v", tgts)
	}
}

func TestOlderReportIgnored(t *testing.T) {
	a := NewAggregator()
	_, _ = a.ProcessObservation(obs("A", 0.8, 0, 0, t0), t0)
	got, err := a.ProcessObservation(obs("A", 0.9, 0, 0, t0-1), t0)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected nil for older report, got %+v", got)
	}
	tgts, _ := a.RecalculateAll(t0)
	if len(tgts) != 1 || !near(tgts[0].Probability, 0.8) {
		t.Fatalf("state changed after older report: %+v", tgts)
	}
}

func TestNewerReplacement(t *testing.T) {
	a := NewAggregator()
	_, _ = a.ProcessObservation(obs("A", 0.8, 0, 0, t0), t0)
	got, err := a.ProcessObservation(obs("A", 0.5, 0, 0, t0+1000), t0+1000)
	if err != nil {
		t.Fatal(err)
	}
	assertTarget(t, got, "17", 0.5, 0, 0, t0+1000)
}

func TestHalfLife(t *testing.T) {
	a := NewAggregator()
	_, _ = a.ProcessObservation(obs("A", 0.8, 0, 0, t0), t0)
	_, _ = a.ProcessObservation(obs("B", 0.6, 0, 0, t0), t0)
	tgts, _ := a.RecalculateAll(t0 + 30000)
	if len(tgts) != 1 {
		t.Fatalf("expected one target, got %d", len(tgts))
	}
	assertTarget(t, &tgts[0], "17", 0.58, 0, 0, t0)
}

func TestConflictingLocation(t *testing.T) {
	a := NewAggregator()
	_, _ = a.ProcessObservation(obs("A", 0.8, 0, 0, t0), t0)
	_, _ = a.ProcessObservation(obs("B", 0.6, 0, 0, t0), t0)
	got, err := a.ProcessObservation(obs("C", 0.9, 0, 0.001, t0), t0)
	if err != nil {
		t.Fatal(err)
	}
	assertTarget(t, got, "17", 0.92, 0, 0, t0)
}

func TestExpired(t *testing.T) {
	a := NewAggregator()
	_, _ = a.ProcessObservation(obs("A", 0.8, 0, 0, t0), t0)
	first, _ := a.ProcessObservation(obs("B", 0.6, 0, 0, t0), t0)
	if first == nil {
		t.Fatal("expected first target")
	}
	lastLat, lastLon, lastTs := first.Latitude, first.Longitude, first.LastSeenAt
	tgts, stale := a.RecalculateAll(t0 + 120000)
	if len(tgts) != 1 {
		t.Fatalf("expected one stale target, got %d", len(tgts))
	}
	if !stale {
		t.Fatal("expected stale flag")
	}
	if !near(tgts[0].Probability, 0) || !near(tgts[0].Latitude, lastLat) ||
		!near(tgts[0].Longitude, lastLon) || tgts[0].LastSeenAt != lastTs {
		t.Fatalf("stale target mismatch: %+v", tgts[0])
	}
	again, staleAgain := a.RecalculateAll(t0 + 125000)
	if staleAgain || len(again) != 0 {
		t.Fatalf("expected no repeated stale, got %+v stale=%v", again, staleAgain)
	}
}

func TestInvalidConfidence(t *testing.T) {
	a := NewAggregator()
	_, err := a.ProcessObservation(obs("A", 1.2, 0, 0, t0), t0)
	if err == nil {
		t.Fatal("expected error for confidence 1.2")
	}
	tgts, _ := a.RecalculateAll(t0)
	if len(tgts) != 0 {
		t.Fatalf("expected no targets, got %+v", tgts)
	}
}

func TestFutureTimestamp(t *testing.T) {
	a := NewAggregator()
	_, err := a.ProcessObservation(obs("A", 0.8, 0, 0, t0+5001), t0)
	if err == nil {
		t.Fatal("expected rejection for future timestamp")
	}
	tgts, _ := a.RecalculateAll(t0)
	if len(tgts) != 0 {
		t.Fatalf("expected no targets, got %+v", tgts)
	}
}

func TestClockSkewWithinToleranceNormalized(t *testing.T) {
	a := NewAggregator()
	got, err := a.ProcessObservation(obs("A", 0.8, 0, 0, t0+1000), t0)
	if err != nil {
		t.Fatalf("clock skew within tolerance must be accepted: %v", err)
	}
	assertTarget(t, got, "17", 0.8, 0, 0, t0)
}

func TestExpiryBoundary(t *testing.T) {
	a := NewAggregator()
	if _, err := a.ProcessObservation(obs("A", 0.8, 0, 0, t0), t0+119999); err != nil {
		t.Fatalf("age=119999 should be accepted, got %v", err)
	}
	a2 := NewAggregator()
	if _, err := a2.ProcessObservation(obs("A", 0.8, 0, 0, t0), t0+120000); err == nil {
		t.Fatal("age=120000 should be rejected")
	}
}

func TestZeroConfidenceReplacesPrevious(t *testing.T) {
	a := NewAggregator()
	_, _ = a.ProcessObservation(obs("A", 0.8, 0, 0, t0), t0)
	got, err := a.ProcessObservation(obs("A", 0, 0, 0, t0+1), t0+1)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || !near(got.Probability, 0) {
		t.Fatalf("expected P=0 after zero-confidence replacement, got %+v", got)
	}
	tgts, stale := a.RecalculateAll(t0 + 5000)
	if len(tgts) != 0 || stale {
		t.Fatalf("expected no repeated stale, got %+v stale=%v", tgts, stale)
	}
}

func TestZeroConfidenceOnlyNeverPublished(t *testing.T) {
	a := NewAggregator()
	got, err := a.ProcessObservation(obs("A", 0, 0, 0, t0), t0)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected no publication for zero-confidence-only, got %+v", got)
	}
	tgts, stale := a.RecalculateAll(t0)
	if len(tgts) != 0 || stale {
		t.Fatalf("expected empty, got %+v stale=%v", tgts, stale)
	}
}

func TestRecalculateEmpty(t *testing.T) {
	a := NewAggregator()
	tgts, stale := a.RecalculateAll(t0)
	if len(tgts) != 0 || stale {
		t.Fatalf("expected empty, got %+v stale=%v", tgts, stale)
	}
}

func TestTieBreakByScore(t *testing.T) {
	a := NewAggregator()
	_, _ = a.ProcessObservation(obs("A", 0.9, 0, 0, t0), t0)
	got, _ := a.ProcessObservation(obs("B", 0.5, 10, 10, t0), t0)
	assertTarget(t, got, "17", 0.9, 0, 0, t0)
}

func TestTieBreakByMaxTimestamp(t *testing.T) {
	a := NewAggregator()
	_, _ = a.ProcessObservation(obs("A", 0.8, 0, 0, t0), t0)
	_, _ = a.ProcessObservation(obs("B", 0.6, 0, 0, t0), t0)
	got, _ := a.ProcessObservation(obs("C", 0.92, 10, 10, t0+1000), t0+1000)
	assertTarget(t, got, "17", 0.92, 10, 10, t0+1000)
}

func TestTieBreakByAnchorID(t *testing.T) {
	a := NewAggregator()
	_, _ = a.ProcessObservation(obs("B", 0.9, 10, 10, t0), t0)
	got, _ := a.ProcessObservation(obs("A", 0.9, 0, 0, t0), t0)
	assertTarget(t, got, "17", 0.9, 0, 0, t0)
}

func TestZeroConfidence(t *testing.T) {
	a := NewAggregator()
	_, _ = a.ProcessObservation(obs("A", 0.8, 0, 0, t0), t0)
	got, err := a.ProcessObservation(obs("B", 0, 0, 0, t0), t0)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || !near(got.Probability, 0.8) {
		t.Fatalf("expected P=0.8, got %+v", got)
	}
}

func TestOutOfRangeCoordinates(t *testing.T) {
	a := NewAggregator()
	cases := []Observation{
		obs("A", 0.8, 91, 0, t0),
		obs("A", 0.8, -91, 0, t0),
		obs("A", 0.8, 0, 181, t0),
		obs("A", 0.8, 0, -181, t0),
	}
	for _, o := range cases {
		if _, err := a.ProcessObservation(o, t0); err == nil {
			t.Fatalf("expected rejection for %+v", o)
		}
	}
}

func TestNonFinite(t *testing.T) {
	a := NewAggregator()
	cases := []Observation{
		obs("A", 0.8, math.NaN(), 0, t0),
		obs("A", 0.8, 0, math.Inf(1), t0),
		obs("A", math.NaN(), 0, 0, t0),
	}
	for _, o := range cases {
		if _, err := a.ProcessObservation(o, t0); err == nil {
			t.Fatalf("expected rejection for %+v", o)
		}
	}
}

func TestValidateRejectsEmptyIDs(t *testing.T) {
	a := NewAggregator()
	for _, o := range []Observation{
		{TargetID: "", ObserverID: "A", Confidence: 0.8, Timestamp: t0},
		{TargetID: "17", ObserverID: "", Confidence: 0.8, Timestamp: t0},
	} {
		if _, err := a.ProcessObservation(o, t0); err == nil {
			t.Fatalf("expected rejection for %+v", o)
		}
	}
}

func TestValidateRejectsNonPositiveTimestamp(t *testing.T) {
	a := NewAggregator()
	for _, ts := range []int64{0, -1} {
		o := obs("A", 0.8, 0, 0, ts)
		if _, err := a.ProcessObservation(o, t0); err == nil {
			t.Fatalf("expected rejection for ts=%d", ts)
		}
	}
}

func TestExpiredOnArrival(t *testing.T) {
	a := NewAggregator()
	if _, err := a.ProcessObservation(obs("A", 0.8, 0, 0, t0), t0+120000); err == nil {
		t.Fatal("expected rejection for expired-on-arrival")
	}
}

func TestStaleOnce(t *testing.T) {
	a := NewAggregator()
	_, _ = a.ProcessObservation(obs("A", 0.8, 0, 0, t0), t0)
	tgts, stale := a.RecalculateAll(t0 + 120000)
	if !stale || len(tgts) != 1 || !near(tgts[0].Probability, 0) {
		t.Fatalf("expected one stale, got %+v stale=%v", tgts, stale)
	}
	got, err := a.ProcessObservation(obs("B", 0.6, 0, 0, t0+120001), t0+120001)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || !near(got.Probability, 0.6) {
		t.Fatalf("expected fresh P=0.6, got %+v", got)
	}
	tgts, stale = a.RecalculateAll(t0 + 240001)
	if !stale || len(tgts) != 1 || !near(tgts[0].Probability, 0) {
		t.Fatalf("expected second stale, got %+v stale=%v", tgts, stale)
	}
}

func TestHaversineSanity(t *testing.T) {
	d := haversineM(0, 0, 0.001, 0)
	if d < 110 || d > 112 {
		t.Fatalf("haversine sanity failed: %v", d)
	}
}
