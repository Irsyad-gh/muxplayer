package app

// note: input parsing is a pile of small special cases, but it works

import (
	"fmt"
	"os"
	"strconv"
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

var pageKeys = map[string]string{"5": "pgup", "6": "pgdn"}

var controlKeys = map[rune]string{
	13: "enter", 9: "tab", 32: "space", 127: "backspace", 8: "backspace",
	3: "ctrl-c", 21: "ctrl-u", 27: "esc",
	29: "ctrl-+", 31: "ctrl--", // Ctrl+= and Ctrl+- on legacy terminals
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
	end := -1
	for i := 2; i < len(s); i++ {
		if s[i] >= 0x40 && s[i] <= 0x7e {
			end = i
			break
		}
	}
	if end < 0 {
		return input{}, 2
	}
	final, used := s[end], end+1
	params := strings.Split(s[2:end], ";")

	switch final {
	case 'u':
		// Kitty keyboard protocol: CSI codepoint;modifier u.
		if len(params) >= 2 {
			if k := keyForCodepoint(csiNumber(params[0])); k != "" && ctrlMod(params[len(params)-1]) {
				return input{key: "ctrl-" + k}, used
			}
		}
	case '~':
		if len(params) == 1 {
			if k, ok := pageKeys[params[0]]; ok {
				return input{key: k}, used
			}
		}
		// xterm modifyOtherKeys: CSI 27;modifier;codepoint ~.
		if len(params) >= 3 && csiNumber(params[0]) == 27 {
			if k := keyForCodepoint(csiNumber(params[2])); k != "" && ctrlMod(params[1]) {
				return input{key: "ctrl-" + k}, used
			}
		}
	default:
		if arrow, ok := arrowKeys[final]; ok {
			if len(params) >= 2 && ctrlMod(params[len(params)-1]) {
				return input{key: "ctrl-" + arrow}, used
			}
			return input{key: arrow}, used
		}
	}
	return input{}, used
}

// csiNumber reads the first number of a CSI parameter, ignoring kitty-style
// subparameters such as 5:1.
func csiNumber(p string) int {
	if i := strings.IndexByte(p, ':'); i >= 0 {
		p = p[:i]
	}
	n, _ := strconv.Atoi(p)
	return n
}

// ctrlMod reports whether a 1-based terminal modifier includes Ctrl.
func ctrlMod(p string) bool {
	n := csiNumber(p)
	return n > 0 && (n-1)&4 != 0
}

// keyForCodepoint maps the punctuation variants that mean plus or minus.
func keyForCodepoint(n int) string {
	switch n {
	case 43, 61, 107: // +, =, keypad +
		return "+"
	case 45, 95, 109: // -, _, keypad -
		return "-"
	}
	return ""
}

func parseRune(s string) (input, int) {
	r, size := utf8.DecodeRuneInString(s)
	if k, ok := controlKeys[r]; ok {
		return input{key: k}, size
	}
	return input{key: string(r)}, size
}
