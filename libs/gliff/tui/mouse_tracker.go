package tui

import (
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"
)

type zone struct {
	name           string
	startX, startY int
	endX, endY     int
}

func (z zone) contains(x, y int) bool {
	return x >= z.startX && x <= z.endX && y >= z.startY && y <= z.endY
}

// MouseTracker tracks clickable regions in rendered content using zero-width
// ANSI markers. Mark wraps content with marker pairs during Render, Sweep
// strips them and builds a coordinate map, and Resolve matches mouse clicks
// to the innermost marked region.
type MouseTracker struct {
	nextID int
	names  map[int]string
	zones  []zone
}

var defaultMouseTracker = &MouseTracker{}

// WithTarget wraps content with mouse-tracking markers using the default tracker.
func WithTarget(name, content string) string {
	return defaultMouseTracker.Mark(name, content)
}

// Mark wraps content with a pair of zero-width CSI markers that will be
// resolved to screen coordinates during Sweep.
func (mt *MouseTracker) Mark(name, content string) string {
	id := mt.nextID
	mt.nextID++
	if mt.names == nil {
		mt.names = make(map[int]string)
	}
	mt.names[id] = name
	marker := fmt.Sprintf("\x1b[%dz", id)
	return marker + content + marker
}

// Sweep scans rendered content, strips mouse-tracking markers, and records
// the screen coordinates of each marked region. Returns cleaned content
// suitable for screen rendering.
func (mt *MouseTracker) Sweep(content string) string {
	mt.zones = mt.zones[:0]

	if len(mt.names) == 0 {
		mt.nextID = 0
		return content
	}

	type pending struct {
		name   string
		startX int
		startY int
	}

	open := make(map[int]pending)
	var zones []zone

	var out strings.Builder
	out.Grow(len(content))

	row, col := 0, 0

	const (
		sweepNormal = iota
		sweepEscape
		sweepCSI
	)

	state := sweepNormal
	var paramBuf strings.Builder
	var seqStart int // index in content where ESC began

	for i := 0; i < len(content); i++ {
		b := content[i]

		switch state {
		case sweepNormal:
			if b == '\x1b' {
				seqStart = i
				state = sweepEscape
				continue
			}
			if b == '\n' {
				out.WriteByte(b)
				row++
				col = 0
				continue
			}
			// Decode UTF-8 rune for width calculation
			r, size := decodeRune(content[i:])
			out.WriteString(content[i : i+size])
			col += runewidth.RuneWidth(r)
			i += size - 1

		case sweepEscape:
			if b == '[' {
				paramBuf.Reset()
				state = sweepCSI
				continue
			}
			// Not a CSI sequence, output ESC + this byte
			out.WriteString(content[seqStart : i+1])
			state = sweepNormal

		case sweepCSI:
			if b >= '0' && b <= '9' {
				paramBuf.WriteByte(b)
				continue
			}
			if b == ';' {
				paramBuf.WriteByte(b)
				continue
			}
			// Final byte
			if b == 'z' {
				id := parseCSIParam(paramBuf.String())
				if name, ok := mt.names[id]; ok {
					if p, opened := open[id]; opened {
						// Second marker: close the zone
						zones = append(zones, zone{
							name:   p.name,
							startX: p.startX, startY: p.startY,
							endX: col - 1, endY: row,
						})
						delete(open, id)
					} else {
						// First marker: record start position
						open[id] = pending{name: name, startX: col, startY: row}
					}
					state = sweepNormal
					continue
				}
			}
			// Not one of our markers — write the full CSI sequence to output
			out.WriteString(content[seqStart : i+1])
			state = sweepNormal
		}
	}

	mt.zones = zones
	mt.names = nil
	mt.nextID = 0

	return out.String()
}

// Resolve returns the name of the innermost zone containing (x, y), or ""
// if no zone matches. Inner zones appear before outer zones in the list
// (their end markers are encountered first during Sweep), so the first
// match is the most deeply nested.
func (mt *MouseTracker) Resolve(x, y int) string {
	for _, z := range mt.zones {
		if z.contains(x, y) {
			return z.name
		}
	}
	return ""
}

// Helpers

func parseCSIParam(s string) int {
	n := 0
	for _, b := range []byte(s) {
		if b >= '0' && b <= '9' {
			n = n*10 + int(b-'0')
		}
	}
	return n
}

func decodeRune(s string) (rune, int) {
	if len(s) == 0 {
		return 0, 0
	}
	b := s[0]
	if b < 0x80 {
		return rune(b), 1
	}
	// Determine byte count from leading bits
	var size int
	var r rune
	switch {
	case b&0xE0 == 0xC0:
		size = 2
		r = rune(b & 0x1F)
	case b&0xF0 == 0xE0:
		size = 3
		r = rune(b & 0x0F)
	case b&0xF8 == 0xF0:
		size = 4
		r = rune(b & 0x07)
	default:
		return rune(b), 1
	}
	if len(s) < size {
		return rune(b), 1
	}
	for i := 1; i < size; i++ {
		r = r<<6 | rune(s[i]&0x3F)
	}
	return r, size
}
