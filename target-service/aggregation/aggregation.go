// Package aggregation implements the deterministic Observation -> Target
// aggregation algorithm specified in spec/target-aggregation.md.
//
// The core is pure: no wall-clock reads, no Broker/DBMS access. The caller
// supplies server time via nowMs. Equal inputs and nowMs yield equal outputs.
package aggregation

import (
	"errors"
	"math"
	"sort"
)

// Spec constants (spec/target-aggregation.md, "Initial constants").
const (
	HalfLifeMs        = 30000.0
	MaxAgeMs          = 120000
	FutureToleranceMs = 5000
	ClusterRadiusM    = 20.0
	RecalcIntervalMs  = 5000
	earthRadiusM      = 6371000.0
)

// Observation mirrors the six fields of spec/data-model.md.
type Observation struct {
	TargetID   string  `json:"targetId"`
	ObserverID string  `json:"observerId"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	Confidence float64 `json:"confidence"`
	Timestamp  int64   `json:"timestamp"`
}

// Target mirrors the five fields of spec/data-model.md.
type Target struct {
	TargetID    string  `json:"targetId"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Probability float64 `json:"probability"`
	LastSeenAt  int64   `json:"lastSeenAt"`
}

// ErrInvalidObservation is returned for observations rejected by validation.
var ErrInvalidObservation = errors.New("invalid observation")

type storedObservation struct {
	observerID string
	lat, lon   float64
	confidence float64
	normTs     int64
}

type targetState struct {
	observers     map[string]storedObservation
	lastPublished *Target
	staleEmitted  bool
}

// Aggregator is the deterministic aggregation core. Not safe for concurrent
// use; callers must serialize access.
type Aggregator struct {
	targets map[string]*targetState
}

// NewAggregator returns an empty aggregator.
func NewAggregator() *Aggregator {
	return &Aggregator{targets: make(map[string]*targetState)}
}

// Validate checks the six Observation fields per spec.
func Validate(o Observation) error {
	if o.TargetID == "" || o.ObserverID == "" {
		return ErrInvalidObservation
	}
	if !finite(o.Latitude) || o.Latitude < -90 || o.Latitude > 90 {
		return ErrInvalidObservation
	}
	if !finite(o.Longitude) || o.Longitude < -180 || o.Longitude > 180 {
		return ErrInvalidObservation
	}
	if !finite(o.Confidence) || o.Confidence < 0 || o.Confidence > 1 {
		return ErrInvalidObservation
	}
	if o.Timestamp <= 0 {
		return ErrInvalidObservation
	}
	return nil
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// ProcessObservation ingests one observation at server time nowMs.
func (a *Aggregator) ProcessObservation(o Observation, nowMs int64) (*Target, error) {
	if err := Validate(o); err != nil {
		return nil, err
	}
	if o.Timestamp > nowMs+FutureToleranceMs {
		return nil, ErrInvalidObservation
	}
	normTs := o.Timestamp
	if normTs > nowMs {
		normTs = nowMs
	}
	if nowMs-normTs >= MaxAgeMs {
		return nil, ErrInvalidObservation
	}

	st, ok := a.targets[o.TargetID]
	if !ok {
		st = &targetState{observers: make(map[string]storedObservation)}
		a.targets[o.TargetID] = st
	}
	if prev, exists := st.observers[o.ObserverID]; exists && normTs <= prev.normTs {
		return nil, nil
	}
	st.observers[o.ObserverID] = storedObservation{
		observerID: o.ObserverID,
		lat:        o.Latitude,
		lon:        o.Longitude,
		confidence: o.Confidence,
		normTs:     normTs,
	}
	if o.Confidence > 0 {
		st.staleEmitted = false
	}
	return a.computeTarget(o.TargetID, st, nowMs), nil
}

// RecalculateAll recomputes every active target at nowMs.
func (a *Aggregator) RecalculateAll(nowMs int64) ([]Target, bool) {
	ids := make([]string, 0, len(a.targets))
	for id := range a.targets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var out []Target
	staleAny := false
	for _, id := range ids {
		st := a.targets[id]
		if t, emitted := a.computeTargetWithStale(id, st, nowMs); t != nil {
			out = append(out, *t)
			if emitted {
				staleAny = true
			}
		}
	}
	return out, staleAny
}

func (a *Aggregator) computeTarget(id string, st *targetState, nowMs int64) *Target {
	t, _ := a.computeTargetWithStale(id, st, nowMs)
	return t
}

type weighted struct {
	observerID string
	lat, lon   float64
	weight     float64
	normTs     int64
}

func (a *Aggregator) computeTargetWithStale(id string, st *targetState, nowMs int64) (*Target, bool) {
	for oid, rec := range st.observers {
		if nowMs-rec.normTs >= MaxAgeMs {
			delete(st.observers, oid)
		}
	}

	var active []weighted
	for _, rec := range st.observers {
		age := nowMs - rec.normTs
		if age < 0 {
			age = 0
		}
		w := rec.confidence * math.Pow(2.0, -float64(age)/HalfLifeMs)
		if w <= 0 {
			continue
		}
		active = append(active, weighted{
			observerID: rec.observerID,
			lat:        rec.lat,
			lon:        rec.lon,
			weight:     w,
			normTs:     rec.normTs,
		})
	}

	if len(active) == 0 {
		if st.staleEmitted || st.lastPublished == nil {
			return nil, false
		}
		stale := &Target{
			TargetID:    id,
			Latitude:    st.lastPublished.Latitude,
			Longitude:   st.lastPublished.Longitude,
			Probability: 0.0,
			LastSeenAt:  st.lastPublished.LastSeenAt,
		}
		st.lastPublished = stale
		st.staleEmitted = true
		return stale, true
	}

	sort.Slice(active, func(i, j int) bool {
		return active[i].observerID < active[j].observerID
	})

	bestScore := -1.0
	var bestGroup []weighted
	var bestMaxTs int64
	var bestAnchorID string
	haveBest := false

	for i := range active {
		anchor := active[i]
		var group []weighted
		maxTs := int64(math.MinInt64)
		for j := range active {
			d := haversineM(anchor.lat, anchor.lon, active[j].lat, active[j].lon)
			if d <= ClusterRadiusM {
				group = append(group, active[j])
				if active[j].normTs > maxTs {
					maxTs = active[j].normTs
				}
			}
		}
		weights := make([]float64, len(group))
		for k := range group {
			weights[k] = group[k].weight
		}
		score := groupProbability(weights)
		if !haveBest || better(score, maxTs, anchor.observerID, bestScore, bestMaxTs, bestAnchorID) {
			bestScore = score
			bestGroup = group
			bestMaxTs = maxTs
			bestAnchorID = anchor.observerID
			haveBest = true
		}
	}

	if len(bestGroup) == 0 {
		return nil, false
	}

	var sw, slat, slon float64
	lastSeen := int64(math.MinInt64)
	for _, r := range bestGroup {
		sw += r.weight
		slat += r.weight * r.lat
		slon += r.weight * r.lon
		if r.normTs > lastSeen {
			lastSeen = r.normTs
		}
	}
	var lat, lon float64
	if sw > 0 {
		lat = slat / sw
		lon = slon / sw
	}

	t := &Target{
		TargetID:    id,
		Latitude:    lat,
		Longitude:   lon,
		Probability: bestScore,
		LastSeenAt:  lastSeen,
	}
	st.lastPublished = t
	st.staleEmitted = false
	return t, false
}

func better(score float64, maxTs int64, anchorID string, bestScore float64, bestMaxTs int64, bestAnchorID string) bool {
	const eps = 1e-12
	if score > bestScore+eps {
		return true
	}
	if math.Abs(score-bestScore) <= eps {
		if maxTs > bestMaxTs {
			return true
		}
		if maxTs == bestMaxTs {
			if bestAnchorID == "" || anchorID < bestAnchorID {
				return true
			}
		}
	}
	return false
}

func groupProbability(weights []float64) float64 {
	prod := 1.0
	for _, w := range weights {
		prod *= (1.0 - w)
	}
	return 1.0 - prod
}

func haversineM(lat1, lon1, lat2, lon2 float64) float64 {
	const degToRad = math.Pi / 180.0
	phi1 := lat1 * degToRad
	phi2 := lat2 * degToRad
	dPhi := (lat2 - lat1) * degToRad
	dLambda := (lon2 - lon1) * degToRad
	a := math.Sin(dPhi/2)*math.Sin(dPhi/2) +
		math.Cos(phi1)*math.Cos(phi2)*math.Sin(dLambda/2)*math.Sin(dLambda/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusM * c
}
