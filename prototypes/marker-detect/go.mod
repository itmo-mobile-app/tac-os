module tac-os/marker-detect

go 1.21

// The version of gocv must match the installed OpenCV (see "Versions" in the README):
// gocv v0.43.0 officially supports OpenCV 4.12.0; it was also built and tested with OpenCV 4.14.0.
// After changing it run `go mod tidy`.
require gocv.io/x/gocv v0.43.0
