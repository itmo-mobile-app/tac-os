// Package marker describes the game markers and the result of recognizing them.
//
// Marker format: ArUco dictionary DICT_4X4_100, id = team*PlayersPerTeam + player.
// The package is plain Go and does not depend on OpenCV.
package marker

import "strconv"

const (
	TeamCount      = 6
	PlayersPerTeam = 12
	// Count is the number of game markers; valid ids are 0..Count-1.
	Count = TeamCount * PlayersPerTeam
)

// IsValid reports whether id is a game marker.
func IsValid(id int) bool { return id >= 0 && id < Count }

// Team returns the team 0..TeamCount-1 of a valid marker id.
func Team(id int) int { return id / PlayersPerTeam }

// Player returns the player 0..PlayersPerTeam-1 of a valid marker id.
func Player(id int) int { return id % PlayersPerTeam }

// TargetID is the `targetId` of a Detection or an Observation: the marker id as a decimal string
// (spec/data-model.md).
func TargetID(id int) string { return strconv.Itoa(id) }

// Detection is the result of recognizing one marker in a camera frame.
type Detection struct {
	ID int
	// RelativeAngleDeg is the horizontal angle to the marker relative to the center of the frame:
	// 0 is the center, positive is to the right.
	RelativeAngleDeg float64
	// DistanceM is the distance from the camera to the center of the marker, in meters.
	DistanceM float64
}
