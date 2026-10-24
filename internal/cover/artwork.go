// Artwork reuse for the MPRIS interface: controllers cannot read pixel
// buffers, so they share artwork files instead — either a folder image or the
// track's embedded front cover extracted once to the user cache.
package cover

import (
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

var (
	artMu   sync.Mutex
	artDone = map[string]string{} // track path → extracted art path ("" failed)
)

// Artwork returns the path of a reusable artwork image for src: an artwork
// named like folder.jpg next to it, or src's embedded front cover extracted to
// ~/.cache/muxplayer/art. It returns "" when neither is available.
func Artwork(src string) string {
	if p, ok := siblingArtwork(src); ok {
		return p
	}
	artMu.Lock()
	defer artMu.Unlock()
	if p, ok := artDone[src]; ok {
		return p
	}
	dst := extractArt(src)
	artDone[src] = dst // failures are cached too, so missed files stop churning ffmpeg
	return dst
}

// siblingArtwork finds a folder-cover style image living next to src.
func siblingArtwork(src string) (string, bool) {
	for _, name := range coverNames {
		if p := filepath.Join(filepath.Dir(src), name); fileExists(p) {
			return p, true
		}
	}
	return "", false
}

// extractArt writes src's first video stream as a JPEG in the cache and
// returns its path, "" on failure.
func extractArt(src string) string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	dir = filepath.Join(dir, "muxplayer", "art")
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(src))
	dst := filepath.Join(dir, fmt.Sprintf("%x.jpg", h.Sum32()))
	if fileExists(dst) {
		return dst
	}
	args := []string{
		"-v", "quiet", "-i", src, "-an", "-map", "0:v:0",
		"-frames:v", "1", "-q:v", "3", dst,
	}
	if err = exec.Command("ffmpeg", args...).Run(); err != nil {
		return ""
	}
	return dst
}
