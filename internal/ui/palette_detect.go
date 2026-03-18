package ui

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/lucasb-eyer/go-colorful"
)

const (
	sampleForeground = 16
	sampleBackground = 17
	sampleCount      = 18
)

// DetectedColors holds optional RGB values detected from the terminal.
type DetectedColors struct {
	Colors   [sampleCount]colorful.Color
	Detected [sampleCount]bool
}

// SupportsTrueColor returns true if the terminal responded with RGB color
// values, implying it can also render them.
// SupportsTrueColor reports whether the terminal is likely to support
// 24-bit color output. COLORTERM is the authoritative signal; when it is
// absent (common over SSH), we infer support from successful OSC palette
// detection.
func (d DetectedColors) SupportsTrueColor() bool {
	colorterm := os.Getenv("COLORTERM")
	if colorterm == "truecolor" || colorterm == "24bit" {
		return true
	}

	return slices.Contains(d.Detected[:], true)
}

type colorResult struct {
	index int
	color colorful.Color
}

// DetectTerminalColors queries the terminal for foreground, background,
// and all 16 ANSI palette colors via OSC sequences. A DA1 request is
// appended as a sentinel. The function returns after all responses arrive,
// the DA1 sentinel is received, or the timeout expires.
func DetectTerminalColors(timeout time.Duration) DetectedColors {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return DetectedColors{}
	}

	fd := tty.Fd()
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		tty.Close()
		return DetectedColors{}
	}

	defer func() {
		term.Restore(fd, oldState)
		tty.Close()
	}()

	return detectFrom(tty, timeout)
}

// detectFrom runs the OSC query/response cycle over any ReadWriter.
// It spawns a reader goroutine that blocks on rw.Read; the caller must
// close rw after detectFrom returns to unblock the goroutine.
// Split out from DetectTerminalColors for testability.
func detectFrom(rw io.ReadWriter, timeout time.Duration) DetectedColors {
	var query strings.Builder
	query.WriteString("\x1b]10;?\x07") // foreground
	query.WriteString("\x1b]11;?\x07") // background
	for i := range 16 {
		fmt.Fprintf(&query, "\x1b]4;%d;?\x07", i)
	}
	query.WriteString("\x1b[c") // DA1 sentinel

	if _, err := rw.Write([]byte(query.String())); err != nil {
		return DetectedColors{}
	}

	ch := make(chan colorResult, sampleCount)
	done := make(chan struct{})

	go readOSCResponses(rw, ch, done)

	var detected DetectedColors
	timer := time.After(timeout)
	received := 0

	collect := func(r colorResult) {
		if r.index >= 0 && r.index < sampleCount {
			detected.Colors[r.index] = r.color
			detected.Detected[r.index] = true
			received++
		}
	}

	for {
		select {
		case r := <-ch:
			collect(r)
		case <-done:
			// Drain any remaining buffered results
			for {
				select {
				case r := <-ch:
					collect(r)
				default:
					return detected
				}
			}
		case <-timer:
			// Drain any results already buffered
			for {
				select {
				case r := <-ch:
					collect(r)
				default:
					return detected
				}
			}
		}
	}
}

// readOSCResponses reads from the reader, parses OSC color responses,
// and sends them to ch. Closes done when DA1 is seen or a read error occurs.
func readOSCResponses(r io.Reader, ch chan<- colorResult, done chan<- struct{}) {
	defer close(done)

	buf := make([]byte, 4096)
	var acc []byte

	for {
		n, err := r.Read(buf)
		if n > 0 {
			acc = append(acc, buf[:n]...)
			var da1 bool
			acc, da1 = processBuffer(acc, ch)
			if da1 {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// processBuffer extracts complete OSC responses and DA1 from the
// accumulated buffer. Returns the unprocessed remainder and whether
// DA1 was seen.
func processBuffer(buf []byte, ch chan<- colorResult) ([]byte, bool) {
	for {
		// Look for DA1 response: CSI ... c
		if i := findDA1(buf); i >= 0 {
			processOSCSequences(buf[:i], ch)
			return nil, true
		}

		// Look for a complete OSC response terminated by BEL or ST
		end := findOSCEnd(buf)
		if end < 0 {
			return buf, false
		}

		seq := buf[:end]
		buf = buf[end:]
		parseOSCColor(seq, ch)
	}
}

// findDA1 finds a DA1 response (ESC [ ... c) in the buffer.
func findDA1(buf []byte) int {
	for i := range len(buf) - 2 {
		if buf[i] == 0x1b && buf[i+1] == '[' {
			for j := i + 2; j < len(buf); j++ {
				if buf[j] == 'c' {
					return i
				}
				if buf[j] >= 0x40 && buf[j] <= 0x7e {
					break // different CSI final byte
				}
			}
		}
	}
	return -1
}

// findOSCEnd finds the end of the first OSC sequence in buf.
// Returns the index after the terminator, or -1 if incomplete.
func findOSCEnd(buf []byte) int {
	for i := range len(buf) {
		if buf[i] == 0x07 { // BEL
			return i + 1
		}
		if buf[i] == 0x1b && i+1 < len(buf) && buf[i+1] == '\\' { // ST
			return i + 2
		}
	}
	return -1
}

func processOSCSequences(buf []byte, ch chan<- colorResult) {
	for len(buf) > 0 {
		end := findOSCEnd(buf)
		if end < 0 {
			return
		}
		parseOSCColor(buf[:end], ch)
		buf = buf[end:]
	}
}

// parseOSCColor parses an OSC color response and sends the result.
// Formats:
//
//	ESC ] 10 ; rgb:RRRR/GGGG/BBBB BEL  (foreground)
//	ESC ] 11 ; rgb:RRRR/GGGG/BBBB BEL  (background)
//	ESC ] 4 ; N ; rgb:RRRR/GGGG/BBBB BEL  (ANSI color N)
func parseOSCColor(seq []byte, ch chan<- colorResult) {
	s := string(seq)

	if !strings.HasPrefix(s, "\x1b]") {
		return
	}
	s = s[2:]

	// Strip terminator
	s = strings.TrimRight(s, "\x07")
	s = strings.TrimSuffix(s, "\x1b\\")

	rgbIdx := strings.Index(s, "rgb:")
	if rgbIdx < 0 {
		return
	}
	prefix := s[:rgbIdx]
	rgbStr := s[rgbIdx:]

	clr, ok := parseRGB(rgbStr)
	if !ok {
		return
	}

	prefix = strings.TrimRight(prefix, ";")
	switch {
	case prefix == "10":
		ch <- colorResult{sampleForeground, clr}
	case prefix == "11":
		ch <- colorResult{sampleBackground, clr}
	case strings.HasPrefix(prefix, "4;"):
		numStr := strings.TrimPrefix(prefix, "4;")
		n, err := strconv.Atoi(numStr)
		if err == nil && n >= 0 && n < 16 {
			ch <- colorResult{n, clr}
		}
	}
}

// parseRGB parses "rgb:RRRR/GGGG/BBBB" into a colorful.Color.
// Handles both 4-digit (16-bit) and 2-digit (8-bit) hex components.
func parseRGB(s string) (colorful.Color, bool) {
	s = strings.TrimPrefix(s, "rgb:")
	parts := strings.Split(s, "/")
	if len(parts) != 3 {
		return colorful.Color{}, false
	}

	// XParseColor allows 1-4 hex digits per component: h, hh, hhh, hhhh.
	// Each is scaled to its own range: h/0xF, hh/0xFF, hhh/0xFFF, hhhh/0xFFFF.
	parse := func(hex string) (float64, bool) {
		if len(hex) < 1 || len(hex) > 4 {
			return 0, false
		}
		v, err := strconv.ParseUint(hex, 16, 16)
		if err != nil {
			return 0, false
		}
		maxVal := [5]float64{0, 0xF, 0xFF, 0xFFF, 0xFFFF}
		return float64(v) / maxVal[len(hex)], true
	}

	r, ok := parse(parts[0])
	if !ok {
		return colorful.Color{}, false
	}
	g, ok := parse(parts[1])
	if !ok {
		return colorful.Color{}, false
	}
	b, ok := parse(parts[2])
	if !ok {
		return colorful.Color{}, false
	}

	return colorful.Color{R: r, G: g, B: b}, true
}
