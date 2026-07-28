package mouse

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

// Tracker tracks clickable regions in rendered content using zero-width
// ANSI markers. Mark wraps content with marker pairs during View, Sweep
// strips them and builds a coordinate map, and Resolve matches mouse clicks
// to the innermost marked region.
type Tracker struct {
	nextID int
	names  map[int]string
	zones  []zone
}

var defaultTracker = &Tracker{}

// Mark wraps content with mouse-tracking markers using the default tracker.
func Mark(name, content string) string {
	return defaultTracker.Mark(name, content)
}

// Sweep strips markers and records screen coordinates using the default tracker.
func Sweep(content string) string {
	return defaultTracker.Sweep(content)
}

// Resolve returns the innermost zone at (x, y) using the default tracker.
func Resolve(x, y int) string {
	return defaultTracker.Resolve(x, y)
}

// Mark wraps content with a pair of zero-width CSI markers that will be
// resolved to screen coordinates during Sweep.
func (mt *Tracker) Mark(name, content string) string {
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
func (mt *Tracker) Sweep(content string) string {
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

	lexer := NewLexer(content)

	for {
		tok := lexer.Next()
		if tok.Type == EOFToken {
			break
		}

		switch tok.Type {
		case TextToken:
			for _, r := range tok.Text {
				if r == '\n' {
					out.WriteRune(r)
					row++
					col = 0
				} else {
					out.WriteRune(r)
					col += runewidth.RuneWidth(r)
				}
			}

		case CSIToken:
			if tok.Final == 'z' {
				id := ParseCSIParam(tok.Params)
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
					continue
				}
			}
			// Not one of our markers — write the full CSI sequence to output
			out.WriteString(tok.Text)

		case OSCToken, ESCToken:
			out.WriteString(tok.Text)
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
func (mt *Tracker) Resolve(x, y int) string {
	for _, z := range mt.zones {
		if z.contains(x, y) {
			return z.name
		}
	}
	return ""
}
