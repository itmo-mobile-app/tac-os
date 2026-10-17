package tacticalmap

import (
	"fmt"
	"image"
	"math"
)

// Color is straight (not premultiplied) RGBA, with all channels in [0, 255].
type Color struct {
	R uint8 `json:"r"`
	G uint8 `json:"g"`
	B uint8 `json:"b"`
	A uint8 `json:"a"`
}

// Command extends the existing WM JSON envelope. Drawing fields are the Map-side
// proposal documented in spec/map-rendering.md; the supplied WM does not yet implement them.
type Command struct {
	Command   string `json:"command"`
	WindowID  string `json:"window_id,omitempty"`
	Title     string `json:"title,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	ImagePath string `json:"image_path,omitempty"`
	X         *int   `json:"x,omitempty"`
	Y         *int   `json:"y,omitempty"`
	Radius    int    `json:"radius,omitempty"`
	Color     *Color `json:"color,omitempty"`
}

type Marker struct {
	Target Target
	Pixel  image.Point
}

func integer(value int) *int { return &value }

// BuildFrame contains the Map logic. It produces the same commands for both the
// WM client and the local PNG preview; WM never receives geographic/domain data.
func BuildFrame(cfg Config, targets []Target) ([]Command, []Marker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, nil, err
	}
	commands := []Command{
		{Command: "BEGIN_FRAME", WindowID: "map", Color: &Color{R: 225, G: 232, B: 213, A: 255}},
		{Command: "DRAW_IMAGE", WindowID: "map", ImagePath: cfg.Image, X: integer(0), Y: integer(0), Width: cfg.Width, Height: cfg.Height},
	}
	var markers []Marker
	seen := make(map[string]bool)
	for _, target := range targets {
		if target.TargetID == "" || seen[target.TargetID] {
			return nil, nil, fmt.Errorf("target ID must be nonempty and unique: %q", target.TargetID)
		}
		seen[target.TargetID] = true
		if !finite(target.Probability) || target.Probability < 0 || target.Probability > 1 {
			return nil, nil, fmt.Errorf("target %s: probability must be in [0, 1]", target.TargetID)
		}
		point, visible, err := Project(cfg.Bounds, cfg.Width, cfg.Height, target.Latitude, target.Longitude)
		if err != nil {
			return nil, nil, fmt.Errorf("target %s: %w", target.TargetID, err)
		}
		if !visible {
			continue
		}
		markers = append(markers, Marker{Target: target, Pixel: point})
		alpha := uint8(math.Round(255 * (0.25 + 0.75*target.Probability)))
		commands = append(commands,
			Command{Command: "DRAW_CIRCLE", WindowID: "map", X: integer(point.X), Y: integer(point.Y), Radius: 13, Color: &Color{R: 255, G: 255, B: 255, A: 255}},
			Command{Command: "DRAW_CIRCLE", WindowID: "map", X: integer(point.X), Y: integer(point.Y), Radius: 10, Color: &Color{R: 196, G: 72, B: 48, A: alpha}},
		)
	}
	commands = append(commands, Command{Command: "PRESENT", WindowID: "map"})
	return commands, markers, nil
}
