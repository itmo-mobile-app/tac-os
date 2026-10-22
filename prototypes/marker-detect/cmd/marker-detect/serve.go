package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"gocv.io/x/gocv"

	"tac-os/marker-detect/detector"
	"tac-os/marker-detect/ipc"
	"tac-os/marker-detect/marker"
)

// staleFrames is the number of frames that are thrown away before a frame is used. The camera driver may keep
// frames that were captured while nobody was reading, and a DETECT request must analyze the current picture.
const staleFrames = 3

// shutdownTimeout bounds the wait for a request that is still being processed when the process is stopped.
const shutdownTimeout = 3 * time.Second

// runServe is the mode of the detector that is started by the Runtime: it listens on a Unix-domain socket and
// answers DETECT requests (see spec/camera-detector-protocol.md).
func runServe(opt options) int {
	var (
		source      ipc.Source
		closeSource = func() {}
	)
	if opt.fake != "" {
		s, err := ipc.NewFakeSource(opt.fake)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		source = s
	} else {
		d, err := detector.New(opt.markerSizeM)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		cs := &cameraSource{
			open:    func() (videoReader, error) { return openCamera(opt.camera) },
			det:     d,
			hfovDeg: opt.hfovDeg,
		}
		source = cs
		closeSource = func() {
			cs.Close()
			d.Close()
		}
	}

	listener, err := ipc.Listen(opt.socket)
	if err != nil {
		closeSource()
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	server := ipc.NewServer(source)
	server.ErrorLog = log.New(os.Stderr, "marker-detect: ", 0)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	fmt.Fprintf(os.Stderr, "marker-detect: listening on %s\n", opt.socket)

	code := 0
	select {
	case <-stop:
	case err := <-serveErr:
		if err != nil {
			fmt.Fprintf(os.Stderr, "marker-detect: %v\n", err)
			code = 1
		}
	}

	// Closing the server also removes the socket file. If a request is stuck (a camera that does not answer),
	// the process stops anyway: the operating system releases the camera.
	done := make(chan struct{})
	go func() {
		server.Close()
		closeSource()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(shutdownTimeout):
		fmt.Fprintln(os.Stderr, "marker-detect: shutdown timed out, exiting")
	}
	return code
}

// videoReader is the part of gocv.VideoCapture that cameraSource uses.
type videoReader interface {
	Grab(skip int) error
	Read(m *gocv.Mat) bool
	Close() error
}

func openCamera(index int) (videoReader, error) {
	camera, err := gocv.OpenVideoCapture(index)
	if err != nil {
		return nil, err
	}
	if !camera.IsOpened() {
		camera.Close()
		return nil, fmt.Errorf("cannot open camera %d (on macOS allow camera access for your terminal in System Settings)", index)
	}
	return camera, nil
}

// cameraSource is the ipc.Source of the real detector: one DETECT request captures one frame of the camera
// and recognizes the markers in it. The camera is opened by the first request and not before, so the helper
// starts at once and a missing camera is reported as an error of that request.
type cameraSource struct {
	open    func() (videoReader, error)
	det     *detector.Detector
	hfovDeg float64

	mu     sync.Mutex
	camera videoReader // nil while the camera is not open
	closed bool
}

func (s *cameraSource) Detect() ([]marker.Detection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("the camera source is closed")
	}

	if s.camera == nil {
		camera, err := s.open()
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ipc.ErrCameraUnavailable, err)
		}
		s.camera = camera
	}

	frame := gocv.NewMat()
	defer frame.Close()
	if err := s.camera.Grab(staleFrames); err != nil || !s.camera.Read(&frame) || frame.Empty() {
		// Release the camera, so that the next request tries to open it again (it may have been unplugged).
		s.dropCamera()
		return nil, fmt.Errorf("%w: cannot read a frame", ipc.ErrCameraUnavailable)
	}

	gray := gocv.NewMat()
	defer gray.Close()
	if frame.Channels() == 1 {
		frame.CopyTo(&gray)
	} else {
		gocv.CvtColor(frame, &gray, gocv.ColorBGRToGray)
	}

	results, err := detectGray(s.det, gray, s.hfovDeg)
	if err != nil {
		return nil, err
	}
	dets := make([]marker.Detection, 0, len(results))
	for _, r := range results {
		dets = append(dets, r.Detection)
	}
	return ipc.Normalize(dets), nil
}

// Close releases the camera. Detect fails afterwards.
func (s *cameraSource) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.dropCamera()
}

func (s *cameraSource) dropCamera() {
	if s.camera != nil {
		s.camera.Close()
		s.camera = nil
	}
}
