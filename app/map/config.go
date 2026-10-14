package tacticalmap

import (
	"encoding/json"
	"fmt"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
)

type Bounds struct {
	LatMin float64 `json:"latMin"`
	LatMax float64 `json:"latMax"`
	LonMin float64 `json:"lonMin"`
	LonMax float64 `json:"lonMax"`
}

// Config defines a north-up, small-area test image. Image is resolved relative
// to its JSON file, not the process working directory.
type Config struct {
	Version int    `json:"version"`
	Image   string `json:"image"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Bounds  Bounds `json:"bounds"`
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func (b Bounds) validate() error {
	if !finite(b.LatMin) || !finite(b.LatMax) || !finite(b.LonMin) || !finite(b.LonMax) ||
		b.LatMin < -90 || b.LatMax > 90 || b.LonMin < -180 || b.LonMax > 180 ||
		b.LatMin >= b.LatMax || b.LonMin >= b.LonMax {
		return fmt.Errorf("invalid geographic bounds: require -90 <= latMin < latMax <= 90 and -180 <= lonMin < lonMax <= 180")
	}
	return nil
}

func (c Config) Validate() error {
	if c.Version != 1 {
		return fmt.Errorf("unsupported map version %d; expected 1", c.Version)
	}
	if c.Width != 960 || c.Height != 640 {
		return fmt.Errorf("map dimensions must be 960x640 for the fixed WM prototype")
	}
	if c.Image == "" {
		return fmt.Errorf("map image path is required")
	}
	return c.Bounds.validate()
}

func LoadConfig(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open map configuration: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Config{}, err
	}
	if info.Size() > 64*1024 {
		return Config{}, fmt.Errorf("map configuration exceeds 64 KiB")
	}
	decoder := json.NewDecoder(io.LimitReader(file, 64*1024))
	decoder.DisallowUnknownFields()
	var cfg Config
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode map configuration %s: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Config{}, fmt.Errorf("map configuration must contain exactly one JSON object")
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	if !filepath.IsAbs(cfg.Image) {
		cfg.Image = filepath.Join(filepath.Dir(path), cfg.Image)
	}
	cfg.Image, err = filepath.Abs(cfg.Image)
	if err != nil {
		return Config{}, fmt.Errorf("resolve map image: %w", err)
	}
	imageFile, err := os.Open(cfg.Image)
	if err != nil {
		return Config{}, fmt.Errorf("open map PNG: %w", err)
	}
	defer imageFile.Close()
	imageConfig, err := png.DecodeConfig(imageFile)
	if err != nil {
		return Config{}, fmt.Errorf("read map PNG dimensions: %w", err)
	}
	if imageConfig.Width != cfg.Width || imageConfig.Height != cfg.Height {
		return Config{}, fmt.Errorf("PNG is %dx%d, metadata is %dx%d", imageConfig.Width, imageConfig.Height, cfg.Width, cfg.Height)
	}
	return cfg, nil
}
