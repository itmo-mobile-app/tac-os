package main

import (
	"errors"
	"image"
	"math"
	"testing"

	"gocv.io/x/gocv"

	"tac-os/marker-detect/detector"
	"tac-os/marker-detect/ipc"
)

// fakeCamera is a videoReader that shows a prepared picture. It needs no camera device.
type fakeCamera struct {
	frame    gocv.Mat // BGR picture returned by every Read
	readFail bool
	grabbed  []int
	closed   bool
}

func (f *fakeCamera) Grab(skip int) error { f.grabbed = append(f.grabbed, skip); return nil }

func (f *fakeCamera) Read(m *gocv.Mat) bool {
	if f.readFail {
		return false
	}
	f.frame.CopyTo(m)
	return true
}

func (f *fakeCamera) Close() error { f.closed = true; return nil }

// markerPicture draws marker id with a white margin on a square BGR picture of 800x800 pixels. The black
// square is 600 pixels wide and in the center of the picture.
func markerPicture(t *testing.T, id int) gocv.Mat {
	t.Helper()
	marker, err := detector.RenderMarker(id, 600)
	if err != nil {
		t.Fatal(err)
	}
	const side, margin = 800, 100
	gray := gocv.NewMatWithSize(side, side, gocv.MatTypeCV8UC1)
	defer gray.Close()
	gray.SetTo(gocv.NewScalar(255, 255, 255, 0))
	m, err := gocv.NewMatFromBytes(marker.Height, marker.Width, gocv.MatTypeCV8UC1, marker.Pix)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	roi := gray.Region(image.Rect(margin, margin, margin+marker.Width, margin+marker.Height))
	defer roi.Close()
	m.CopyTo(&roi)

	bgr := gocv.NewMat()
	gocv.CvtColor(gray, &bgr, gocv.ColorGrayToBGR)
	return bgr
}

func newTestSource(t *testing.T, open func() (videoReader, error)) *cameraSource {
	t.Helper()
	d, err := detector.New(0.10)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return &cameraSource{open: open, det: d, hfovDeg: 60}
}

func TestCameraSourceDetectsMarker(t *testing.T) {
	cam := &fakeCamera{frame: markerPicture(t, 17)}
	defer cam.frame.Close()
	opens := 0
	src := newTestSource(t, func() (videoReader, error) { opens++; return cam, nil })

	for i := 0; i < 2; i++ {
		dets, err := src.Detect()
		if err != nil {
			t.Fatal(err)
		}
		if len(dets) != 1 || dets[0].ID != 17 {
			t.Fatalf("detections %+v, want marker 17", dets)
		}
		// The marker is in the center of the frame: 600 px at hfov 60 degrees and 800 px width, 0.10 m marker.
		want := 0.10 * (800 / 2 / math.Tan(30*math.Pi/180)) / 600
		if math.Abs(dets[0].RelativeAngleDeg) > 1 || math.Abs(dets[0].DistanceM-want) > 0.05*want {
			t.Errorf("angle %.2f distance %.3f, want about 0 and %.3f", dets[0].RelativeAngleDeg, dets[0].DistanceM, want)
		}
	}
	if opens != 1 {
		t.Errorf("the camera was opened %d times, want once for all requests", opens)
	}
	if len(cam.grabbed) != 2 || cam.grabbed[0] != staleFrames {
		t.Errorf("Grab calls %v, want %d frames skipped before each request", cam.grabbed, staleFrames)
	}
}

func TestCameraSourceWithoutMarkers(t *testing.T) {
	blank := gocv.NewMatWithSize(480, 640, gocv.MatTypeCV8UC3)
	defer blank.Close()
	blank.SetTo(gocv.NewScalar(255, 255, 255, 0))
	src := newTestSource(t, func() (videoReader, error) { return &fakeCamera{frame: blank}, nil })

	dets, err := src.Detect()
	if err != nil {
		t.Fatal(err)
	}
	if len(dets) != 0 {
		t.Errorf("detections %+v, want none", dets)
	}
}

func TestCameraSourceNoCamera(t *testing.T) {
	opens := 0
	src := newTestSource(t, func() (videoReader, error) { opens++; return nil, errors.New("no such device") })

	for i := 1; i <= 2; i++ {
		if _, err := src.Detect(); !errors.Is(err, ipc.ErrCameraUnavailable) {
			t.Fatalf("request %d: error %v, want ErrCameraUnavailable", i, err)
		}
		if opens != i {
			t.Errorf("after request %d the camera was opened %d times, want %d: every request tries again", i, opens, i)
		}
	}
}

func TestCameraSourceReadFailureReleasesCamera(t *testing.T) {
	cam := &fakeCamera{frame: gocv.NewMat(), readFail: true}
	defer cam.frame.Close()
	opens := 0
	src := newTestSource(t, func() (videoReader, error) { opens++; return cam, nil })

	if _, err := src.Detect(); !errors.Is(err, ipc.ErrCameraUnavailable) {
		t.Fatalf("error %v, want ErrCameraUnavailable", err)
	}
	if !cam.closed {
		t.Error("the camera that cannot be read was not released")
	}
	if _, err := src.Detect(); !errors.Is(err, ipc.ErrCameraUnavailable) || opens != 2 {
		t.Errorf("second request: error %v, opens %d; want the camera to be opened again", err, opens)
	}
}

func TestCameraSourceClose(t *testing.T) {
	cam := &fakeCamera{frame: markerPicture(t, 3)}
	defer cam.frame.Close()
	src := newTestSource(t, func() (videoReader, error) { return cam, nil })
	if _, err := src.Detect(); err != nil {
		t.Fatal(err)
	}

	src.Close()
	if !cam.closed {
		t.Error("Close did not release the camera")
	}
	if _, err := src.Detect(); err == nil || errors.Is(err, ipc.ErrCameraUnavailable) {
		t.Errorf("Detect after Close returned %v, want a plain error", err)
	}
	src.Close() // again, without a panic
}
