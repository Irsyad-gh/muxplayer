// Package cover grabs artwork with ffmpeg and pulls UI colours out of it.
package cover

// note: shelling out to ffmpeg is not elegant, but it handles every format we care about

import (
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"muxplayer/internal/diag"
	"muxplayer/internal/term"
)

type RGB [3]float64

func Pack(c RGB) int32 { return int32(int(c[0])<<16 | int(c[1])<<8 | int(c[2])) }

func Mix(a, b RGB, t float64) RGB {
	return RGB{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t, a[2] + (b[2]-a[2])*t}
}

func Lum(c RGB) float64 { return .299*c[0] + .587*c[1] + .114*c[2] }

// Lift keeps the accent readable on a dark, transparent background.
func Lift(c RGB) RGB {
	l := Lum(c)
	if l >= 120 {
		return c
	}
	return Mix(c, RGB{255, 255, 255}, (120-l)/120*.85)
}

func Hash(s string) RGB {
	h := fnv.New32a()
	h.Write([]byte(s))
	hue := float64(h.Sum32()%360) / 60
	x := 1 - math.Abs(math.Mod(hue, 2)-1)
	r, g, b := 0.0, 0.0, 0.0
	switch int(hue) {
	case 0:
		r, g = 1, x
	case 1:
		r, g = x, 1
	case 2:
		g, b = 1, x
	case 3:
		g, b = x, 1
	case 4:
		r, b = x, 1
	default:
		r, b = 1, x
	}
	return Mix(RGB{r * 255, g * 255, b * 255}, RGB{255, 255, 255}, .25)
}

var audioExt = map[string]bool{
	".mp3": true, ".flac": true, ".ogg": true, ".opus": true, ".wav": true,
	".m4a": true, ".aac": true, ".wma": true, ".ape": true,
}

var coverNames = []string{"cover.jpg", "folder.jpg", "cover.png", "folder.png", "front.jpg"}

func IsAudio(p string) bool {
	return audioExt[strings.ToLower(filepath.Ext(p))]
}

// coverFill scales the cover down from the largest size that fits, leaving a
// little breathing room around it.
const coverFill = 0.85

// Size returns the cover's device-pixel buffer size for a panel rw cells
// wide and h cells tall. The buffer is a pre-distorted square: it is stretched
// by the inverse of the cell aspect so, drawn with half blocks, it appears as
// a true 1:1 square.
func Size(rw, h int, aspect float64) (pw, ph int) {
	if aspect < term.MinAspect || aspect > term.MaxAspect {
		aspect = term.DefaultAspect
	}
	maxRows := h - 11
	if maxRows < 4 {
		return 0, 0
	}
	fit := min(float64(rw), float64(maxRows)*aspect)
	pw = int(math.Round(fit * coverFill))
	pw -= pw % 2
	if pw < 8 {
		return 0, 0
	}
	rows := min(max(int(math.Round(float64(pw)/aspect)), 1), maxRows)
	return pw, rows * 2
}

// vf builds the ffmpeg filter that centre-crops the artwork to a square
// and then stretches it to the target device-pixel size.
func vf(pw, ph int) string {
	return fmt.Sprintf("crop=min(iw\\,ih):min(iw\\,ih),scale=%d:%d", pw, ph)
}

func Load(path string, w, h int) []byte {
	if pix := ffmpegCover(path, w, h); pix != nil {
		diag.Debugf("cover: extracted embedded artwork from %s (%dx%d)", filepath.Base(path), w, h)
		return pix
	}
	for _, name := range coverNames {
		p := filepath.Join(filepath.Dir(path), name)
		if !fileExists(p) {
			continue
		}
		if pix := ffmpegCover(p, w, h); pix != nil {
			diag.Debugf("cover: using folder artwork %s", p)
			return pix
		}
	}
	diag.Debugf("no cover art for %s", strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	return nil
}

func ffmpegCover(src string, w, h int) []byte {
	args := []string{
		"-v", "quiet", "-i", src, "-an", "-frames:v", "1",
		"-vf", vf(w, h), "-f", "rawvideo", "-pix_fmt", "rgb24", "-",
	}
	out, err := exec.Command("ffmpeg", args...).Output()
	if err != nil || len(out) != w*h*3 {
		return nil
	}
	return out
}

// AvgColor returns the mean colour of an rgb24 pixel buffer.
func AvgColor(pix []byte) RGB {
	if len(pix) < 3 {
		return RGB{}
	}
	var r, g, b float64
	for i := 0; i+2 < len(pix); i += 3 {
		r, g, b = r+float64(pix[i]), g+float64(pix[i+1]), b+float64(pix[i+2])
	}
	n := float64(len(pix) / 3)
	return RGB{r / n, g / n, b / n}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
