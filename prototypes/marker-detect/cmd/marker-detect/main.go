// Command marker-detect is the command-line prototype of marker recognition for the Analyzer (issue #34).
//
// It reads frames from the laptop camera (or one image file), recognizes game markers and prints, for each
// of them: id, team, player, the angle relative to the center of the frame and the distance.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/signal"
	"syscall"

	"gocv.io/x/gocv"

	"tac-os/marker-detect/detector"
	"tac-os/marker-detect/marker"
	"tac-os/marker-detect/pose"
)

type options struct {
	camera      int
	hfovDeg     float64
	markerSizeM float64
	show        bool
	imagePath   string
	genID       int
	genPath     string
}

func main() { os.Exit(run()) }

func run() int {
	var opt options
	flag.IntVar(&opt.camera, "camera", 0, "camera index")
	flag.Float64Var(&opt.hfovDeg, "hfov", 60, "approximate horizontal field of view of the camera, degrees")
	flag.Float64Var(&opt.markerSizeM, "marker-size", 0.10, "side of the black square of the printed marker, meters")
	flag.BoolVar(&opt.show, "show", false, "show a preview window with the detected markers (q or Esc to quit)")
	flag.StringVar(&opt.imagePath, "image", "", "recognize one image file and exit")
	flag.IntVar(&opt.genID, "gen-marker", -1, "write a printable marker with this id (0..71) to the file given by -out and exit")
	flag.StringVar(&opt.genPath, "out", "marker.png", "output file for -gen-marker")
	flag.Parse()

	if !(opt.hfovDeg > 10 && opt.hfovDeg < 170) || !(opt.markerSizeM > 0) {
		fmt.Fprintln(os.Stderr, "-hfov must be between 10 and 170 degrees and -marker-size must be positive")
		return 2
	}

	switch {
	case opt.genID >= 0:
		return generateMarker(opt.genID, opt.genPath)
	case opt.imagePath != "":
		return runOnImage(opt)
	default:
		return runOnCamera(opt)
	}
}

func generateMarker(id int, path string) int {
	if !marker.IsValid(id) {
		fmt.Fprintf(os.Stderr, "marker id must be in 0..%d (id = team * %d + player)\n", marker.Count-1, marker.PlayersPerTeam)
		return 1
	}
	frame, err := detector.RenderMarker(id, 600)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	// White margin around the marker: the detector needs it to find the black border.
	const margin = 100
	img := image.NewGray(image.Rect(0, 0, frame.Width+2*margin, frame.Height+2*margin))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	for y := 0; y < frame.Height; y++ {
		copy(img.Pix[(y+margin)*img.Stride+margin:], frame.Pix[y*frame.Width:(y+1)*frame.Width])
	}

	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := f.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("wrote %s (id %d, team %d, player %d). The black square must be exactly the size passed as -marker-size.\n",
		path, id, marker.Team(id), marker.Player(id))
	return 0
}

func runOnImage(opt options) int {
	img := gocv.IMRead(opt.imagePath, gocv.IMReadGrayScale)
	defer img.Close()
	if img.Empty() {
		fmt.Fprintf(os.Stderr, "cannot read image %s\n", opt.imagePath)
		return 1
	}

	d, err := detector.New(opt.markerSizeM)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer d.Close()

	results, err := detectGray(d, img, opt.hfovDeg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	printResults(results)
	return 0
}

func runOnCamera(opt options) int {
	camera, err := gocv.OpenVideoCapture(opt.camera)
	if err != nil || !camera.IsOpened() {
		fmt.Fprintf(os.Stderr, "cannot open camera %d (on macOS allow camera access for your terminal in System Settings): %v\n", opt.camera, err)
		return 1
	}
	defer camera.Close()

	d, err := detector.New(opt.markerSizeM)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer d.Close()

	var window *gocv.Window
	if opt.show {
		window = gocv.NewWindow("marker-detect")
		defer window.Close()
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	frame := gocv.NewMat()
	defer frame.Close()
	gray := gocv.NewMat()
	defer gray.Close()

	for {
		select {
		case <-stop:
			return 0
		default:
		}

		if !camera.Read(&frame) || frame.Empty() {
			fmt.Fprintln(os.Stderr, "cannot read a frame from the camera")
			return 1
		}
		gocv.CvtColor(frame, &gray, gocv.ColorBGRToGray)

		results, err := detectGray(d, gray, opt.hfovDeg)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		printResults(results)

		if window != nil {
			drawResults(&frame, results)
			window.IMShow(frame)
			if key := window.WaitKey(1); key == 27 || key == 'q' {
				return 0
			}
		}
	}
}

// detectGray runs the detector on a single-channel image.
func detectGray(d *detector.Detector, gray gocv.Mat, hfovDeg float64) ([]detector.Result, error) {
	frame := detector.GrayFrame{Width: gray.Cols(), Height: gray.Rows(), Pix: gray.ToBytes()}
	return d.Detect(frame, pose.CameraFromHorizontalFOV(frame.Width, frame.Height, hfovDeg))
}

func printResults(results []detector.Result) {
	for _, r := range results {
		// Round first, so that a tiny negative angle is printed as +0.0 and not as -0.0.
		angle := math.Round(r.RelativeAngleDeg*10) / 10
		if angle == 0 {
			angle = 0
		}
		fmt.Printf("id=%d team=%d player=%d angle=%+.1f deg distance=%.2f m\n",
			r.ID, marker.Team(r.ID), marker.Player(r.ID), angle, r.DistanceM)
	}
}

// drawResults outlines the detected markers on the preview frame.
func drawResults(frame *gocv.Mat, results []detector.Result) {
	green := color.RGBA{R: 0, G: 255, B: 0, A: 255}
	for _, r := range results {
		var pts [4]image.Point
		for i, c := range r.Corners {
			pts[i] = image.Pt(int(math.Round(c.X)), int(math.Round(c.Y)))
		}
		for i := range pts {
			gocv.Line(frame, pts[i], pts[(i+1)%4], green, 2)
		}
		gocv.PutText(frame, fmt.Sprint(r.ID), pts[0], gocv.FontHersheySimplex, 0.7, green, 2)
	}
}
