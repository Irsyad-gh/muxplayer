package app

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

type input struct {
	key          string
	mouse, press bool
	btn, mx, my  int
}

var arrowKeys = map[byte]string{
	'A': "up", 'B': "down", 'C': "right", 'D': "left", 'H': "home", 'F': "end",
}

var pageKeys = map[byte]string{'5': "pgup", '6': "pgdn"}

var controlKeys = map[rune]string{
	13: "enter", 9: "tab", 32: "space", 127: "backspace", 8: "backspace",
	3: "ctrl-c", 27: "esc",
}

func readInput(ch chan<- input) {
	buf := make([]byte, 512)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return
		}
		rest := string(buf[:n])
		for len(rest) > 0 {
			in, used := parseInput(rest)
			rest = rest[used:]
			if in.mouse || in.key != "" {
				ch <- in
			}
		}
	}
}

// parseInput decodes the first event in s and returns how many bytes it used.
func parseInput(s string) (input, int) {
	switch {
	case strings.HasPrefix(s, "\x1b[<"):
		return parseMouse(s)
	case strings.HasPrefix(s, "\x1b[") && len(s) >= 3:
		return parseCSI(s)
	default:
		return parseRune(s)
	}
}

func parseMouse(s string) (input, int) {
	end := strings.IndexAny(s, "Mm")
	if end < 0 {
		return input{}, len(s)
	}
	var btn, x, y int
	fmt.Sscanf(s[3:end], "%d;%d;%d", &btn, &x, &y)
	in := input{mouse: true, btn: btn, mx: x - 1, my: y - 1, press: s[end] == 'M'}
	return in, end + 1
}

func parseCSI(s string) (input, int) {
	if k, ok := arrowKeys[s[2]]; ok {
		return input{key: k}, 3
	}
	if len(s) >= 4 && s[3] == '~' {
		if k, ok := pageKeys[s[2]]; ok {
			return input{key: k}, 4
		}
	}
	return input{}, 2
}

func parseRune(s string) (input, int) {
	r, size := utf8.DecodeRuneInString(s)
	if k, ok := controlKeys[r]; ok {
		return input{key: k}, size
	}
	return input{key: string(r)}, size
}
