package pioled

import (
	"context"
	_ "embed" // For embedding font TTF file
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"net/http"
	"time"

	"github.com/lutzky/pitemp/internal/state"

	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"periph.io/x/periph/conn/i2c"
	"periph.io/x/periph/conn/i2c/i2creg"
	"periph.io/x/periph/devices/ssd1306"
	"periph.io/x/periph/devices/ssd1306/image1bit"
	"periph.io/x/periph/host"
)

// HTTPResponse returns an HTTP response of what would be rendered on the
// PiOLED display.
func HTTPResponse(w http.ResponseWriter, _ *http.Request) {
	img := image.NewPaletted(image.Rect(0, 0, 128, 32), color.Palette{color.Black, color.White})
	render(img, color.White)
	png.Encode(w, img)
}

var dev *ssd1306.Dev
var busCloser i2c.BusCloser

var (
	// ClearDisplay determines if display should be cleared when exiting
	ClearDisplay = true

	// UpdateInterval controls how fast the display is updated
	UpdateInterval = 500 * time.Millisecond

	// StaleTime indicates how stale the state has to be for a warning to be shown
	StaleTime = 3 * time.Minute
)

// Initialize initializes the pioled hardware
func Initialize() error {
	if _, err := host.Init(); err != nil {
		return fmt.Errorf("host init failed: %w", err)
	}

	var err error
	busCloser, err = i2creg.Open("")
	if err != nil {
		return fmt.Errorf("failed to open I²C: %w", err)
	}
	opts := ssd1306.Opts{
		W: 128,
		H: 32,

		Sequential: true,
		Rotated:    true,
	}
	dev, err = ssd1306.NewI2C(busCloser, &opts)
	if err != nil {
		return fmt.Errorf("failed to initialize ssd1306: %w", err)
	}
	return nil
}

func display() {
	if dev == nil {
		log.Print("WARNING: display() called while dev=nil")
		return
	}
	img := image1bit.NewVerticalLSB(dev.Bounds())
	render(img, image1bit.On)
	if err := dev.Draw(dev.Bounds(), img, image.Point{}); err != nil {
		log.Fatal(err)
	}
}

// Font is Silkscreen: https://kottke.org/plus/type/silkscreen/
//go:embed slkscr.ttf
var silkscreenTTF []byte
var silkscreenFace font.Face

func init() {
	font, err := truetype.Parse(silkscreenTTF)
	if err != nil {
		log.Fatalf("Failed to parse embedded font TTF: %v", err)
	}
	silkscreenFace = truetype.NewFace(font, &truetype.Options{
		Size:    8,
		Hinting: 1,
	})
}

func render(dst draw.Image, color color.Color) {
	drawer := font.Drawer{
		Dst:  dst,
		Src:  &image.Uniform{color},
		Face: basicfont.Face7x13,
	}

	// Manual adjustment to keep top-text flush with top of screen.
	// Every pixel counts.
	baseY := -2

	lines := []string{
		"waiting for",
		"sensor data",
	}

	s := state.Get()

	if !s.LastSensorUpdate.IsZero() {
		lines = []string{
			// TODO: Use degree symbol °C,
			fmt.Sprintf("Temp: %.0fC", s.Temperature),
			fmt.Sprintf("Humid: %.0f%%", s.Humidity),
		}

		if time.Since(s.LastSensorUpdate) > StaleTime {
			lines[0] += " STALE!"
		}
	}

	for _, line := range lines {
		baseY += drawer.Face.Metrics().Ascent.Ceil()
		drawer.Dot = fixed.P(0, baseY)
		drawer.DrawString(line)
	}

	clockMsg := time.Now().Local().Format("Mon Jan 2 15:04:05")
	drawer.Face = silkscreenFace
	drawer.Dot = fixed.P(0, dst.Bounds().Dy())
	drawer.DrawString(clockMsg)

	{
		y := dst.Bounds().Max.Y - drawer.Face.Metrics().Ascent.Ceil() - 1
		for x := dst.Bounds().Min.X; x < dst.Bounds().Max.X; x++ {
			dst.Set(x, y, color)
		}
	}
}

// Updater will update the display every interval, until the context is
// cancelled.
func Updater(ctx context.Context) {
	for {
		display()

		{
			t := time.NewTimer(UpdateInterval)
			defer t.Stop()
			select {
			case <-ctx.Done():
				cleanup()
				return
			case <-t.C:
			}
		}
	}
}

func cleanup() {
	if ClearDisplay {
		img := image1bit.NewVerticalLSB(dev.Bounds())
		if err := dev.Draw(dev.Bounds(), img, image.Point{}); err != nil {
			log.Printf("ERROR: Failed to clear display: %v", err)
		}
	}
	busCloser.Close()
}
