package tacticalmap

import (
	"fmt"
	"image"
	"math"
)

// Project maps geographic coordinates to raster pixels. Outside points are
// skipped; only an inside point exactly on the east/south edge uses the final pixel.
func Project(bounds Bounds, width, height int, latitude, longitude float64) (image.Point, bool, error) {
	if err := bounds.validate(); err != nil {
		return image.Point{}, false, err
	}
	if width <= 0 || height <= 0 || width > 8192 || height > 8192 {
		return image.Point{}, false, fmt.Errorf("image dimensions must be in [1, 8192]")
	}
	if !finite(latitude) || !finite(longitude) || latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 {
		return image.Point{}, false, fmt.Errorf("target coordinates must be finite latitude/longitude in degrees")
	}
	if latitude < bounds.LatMin || latitude > bounds.LatMax || longitude < bounds.LonMin || longitude > bounds.LonMax {
		return image.Point{}, false, nil
	}
	x := int(math.Round((longitude - bounds.LonMin) / (bounds.LonMax - bounds.LonMin) * float64(width)))
	y := int(math.Round((bounds.LatMax - latitude) / (bounds.LatMax - bounds.LatMin) * float64(height)))
	if x >= width {
		x = width - 1
	}
	if y >= height {
		y = height - 1
	}
	return image.Pt(x, y), true, nil
}
