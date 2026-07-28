package mouse

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMark(t *testing.T) {
	mt := &Tracker{}

	result := mt.Mark("btn", "Click")
	assert.Equal(t, "\x1b[0zClick\x1b[0z", result)

	result = mt.Mark("link", "Here")
	assert.Equal(t, "\x1b[1zHere\x1b[1z", result)
}

func TestSweep_StripsMarkers(t *testing.T) {
	mt := &Tracker{}
	content := mt.Mark("btn", "Click me")
	cleaned := mt.Sweep(content)
	assert.Equal(t, "Click me", cleaned)
}

func TestSweep_PreservesANSI(t *testing.T) {
	mt := &Tracker{}
	content := mt.Mark("btn", "\x1b[31mRed\x1b[0m")
	cleaned := mt.Sweep(content)
	assert.Equal(t, "\x1b[31mRed\x1b[0m", cleaned)
}

func TestSweep_NoMarkers(t *testing.T) {
	mt := &Tracker{}
	content := "plain text\nline two"
	cleaned := mt.Sweep(content)
	assert.Equal(t, content, cleaned)
}

func TestSweep_SingleTarget(t *testing.T) {
	mt := &Tracker{}
	content := "Hello " + mt.Mark("btn", "World")
	cleaned := mt.Sweep(content)
	assert.Equal(t, "Hello World", cleaned)

	require.Len(t, mt.zones, 1)
	z := mt.zones[0]
	assert.Equal(t, "btn", z.name)
	assert.Equal(t, 6, z.startX)
	assert.Equal(t, 0, z.startY)
	assert.Equal(t, 10, z.endX)
	assert.Equal(t, 0, z.endY)
}

func TestSweep_MultipleTargetsSameLine(t *testing.T) {
	mt := &Tracker{}
	content := mt.Mark("a", "AA") + " " + mt.Mark("b", "BB")
	cleaned := mt.Sweep(content)
	assert.Equal(t, "AA BB", cleaned)

	require.Len(t, mt.zones, 2)
	assert.Equal(t, "a", mt.zones[0].name)
	assert.Equal(t, 0, mt.zones[0].startX)
	assert.Equal(t, 1, mt.zones[0].endX)
	assert.Equal(t, "b", mt.zones[1].name)
	assert.Equal(t, 3, mt.zones[1].startX)
	assert.Equal(t, 4, mt.zones[1].endX)
}

func TestSweep_MultiLineTarget(t *testing.T) {
	mt := &Tracker{}
	content := mt.Mark("block", "line1\nline2\nline3")
	cleaned := mt.Sweep(content)
	assert.Equal(t, "line1\nline2\nline3", cleaned)

	require.Len(t, mt.zones, 1)
	z := mt.zones[0]
	assert.Equal(t, 0, z.startX)
	assert.Equal(t, 0, z.startY)
	assert.Equal(t, 4, z.endX)
	assert.Equal(t, 2, z.endY)
}

func TestSweep_WideCharacters(t *testing.T) {
	mt := &Tracker{}
	content := mt.Mark("wide", "中文")
	cleaned := mt.Sweep(content)
	assert.Equal(t, "中文", cleaned)

	require.Len(t, mt.zones, 1)
	z := mt.zones[0]
	assert.Equal(t, 0, z.startX)
	assert.Equal(t, 3, z.endX) // 2 wide chars = 4 cols, endX = 4-1 = 3
}

func TestSweep_NestedTargets(t *testing.T) {
	mt := &Tracker{}
	inner := mt.Mark("inner", "click")
	outer := mt.Mark("outer", "before "+inner+" after")
	cleaned := mt.Sweep(outer)
	assert.Equal(t, "before click after", cleaned)

	require.Len(t, mt.zones, 2)
	// Inner zone is recorded first (its end marker appears first)
	assert.Equal(t, "inner", mt.zones[0].name)
	assert.Equal(t, 7, mt.zones[0].startX)
	assert.Equal(t, 11, mt.zones[0].endX)

	assert.Equal(t, "outer", mt.zones[1].name)
	assert.Equal(t, 0, mt.zones[1].startX)
	assert.Equal(t, 17, mt.zones[1].endX)
}

func TestSweep_FrameReset(t *testing.T) {
	mt := &Tracker{}

	mt.Mark("a", "first")
	mt.Sweep(mt.Mark("a", "first"))

	// Second frame should start IDs from 0 again
	result := mt.Mark("b", "second")
	assert.Equal(t, "\x1b[0zsecond\x1b[0z", result)
}

func TestResolve_Hit(t *testing.T) {
	mt := &Tracker{}
	content := "Hello " + mt.Mark("btn", "World")
	mt.Sweep(content)

	assert.Equal(t, "btn", mt.Resolve(6, 0))
	assert.Equal(t, "btn", mt.Resolve(10, 0))
}

func TestResolve_Miss(t *testing.T) {
	mt := &Tracker{}
	content := "Hello " + mt.Mark("btn", "World")
	mt.Sweep(content)

	assert.Equal(t, "", mt.Resolve(0, 0))
	assert.Equal(t, "", mt.Resolve(11, 0))
	assert.Equal(t, "", mt.Resolve(6, 1))
}

func TestResolve_Nested(t *testing.T) {
	mt := &Tracker{}
	inner := mt.Mark("inner", "click")
	outer := mt.Mark("outer", "before "+inner+" after")
	mt.Sweep(outer)

	assert.Equal(t, "inner", mt.Resolve(7, 0))
	assert.Equal(t, "inner", mt.Resolve(11, 0))

	assert.Equal(t, "outer", mt.Resolve(0, 0))
	assert.Equal(t, "outer", mt.Resolve(17, 0))
}

func TestResolve_MultiLineHit(t *testing.T) {
	mt := &Tracker{}
	content := "pre " + mt.Mark("block", "line1\nline2\nline3") + " post"
	mt.Sweep(content)

	assert.Equal(t, "", mt.Resolve(3, 0))
	assert.Equal(t, "block", mt.Resolve(4, 0))
	assert.Equal(t, "block", mt.Resolve(4, 1))
	assert.Equal(t, "block", mt.Resolve(4, 2))
	assert.Equal(t, "", mt.Resolve(5, 2))
	assert.Equal(t, "", mt.Resolve(4, 3))
}

func TestZoneContains(t *testing.T) {
	singleLine := zone{name: "s", startX: 3, startY: 0, endX: 7, endY: 0}
	assert.True(t, singleLine.contains(3, 0))
	assert.True(t, singleLine.contains(5, 0))
	assert.True(t, singleLine.contains(7, 0))
	assert.False(t, singleLine.contains(2, 0))
	assert.False(t, singleLine.contains(8, 0))
	assert.False(t, singleLine.contains(5, 1))

	multiLine := zone{name: "m", startX: 5, startY: 0, endX: 15, endY: 2}
	assert.True(t, multiLine.contains(5, 0))
	assert.True(t, multiLine.contains(10, 1))
	assert.True(t, multiLine.contains(15, 2))
	assert.False(t, multiLine.contains(4, 1))
	assert.False(t, multiLine.contains(16, 1))
	assert.False(t, multiLine.contains(10, 3))
}

func TestZoneContains_EndAtNewLine(t *testing.T) {
	z := zone{name: "z", startX: 0, startY: 0, endX: -1, endY: 1}
	assert.False(t, z.contains(0, 0))
	assert.False(t, z.contains(0, 1))
}

func TestMark_DefaultTracker(t *testing.T) {
	saved := *defaultTracker
	defer func() { *defaultTracker = saved }()
	*defaultTracker = Tracker{}

	content := "Click " + Mark("link", "here") + " for more"
	cleaned := defaultTracker.Sweep(content)
	assert.Equal(t, "Click here for more", cleaned)
	assert.Equal(t, "link", defaultTracker.Resolve(6, 0))
	assert.Equal(t, "link", defaultTracker.Resolve(9, 0))
	assert.Equal(t, "", defaultTracker.Resolve(5, 0))
	assert.Equal(t, "", defaultTracker.Resolve(10, 0))
}

func TestMark_MarkerFormat(t *testing.T) {
	mt := &Tracker{}
	for i := range 5 {
		result := mt.Mark(fmt.Sprintf("zone%d", i), "x")
		expected := fmt.Sprintf("\x1b[%dzx\x1b[%dz", i, i)
		assert.Equal(t, expected, result)
	}
}

func TestSweep_MarkerAtLineStart(t *testing.T) {
	mt := &Tracker{}
	content := "first\n" + mt.Mark("second", "second line")
	cleaned := mt.Sweep(content)
	assert.Equal(t, "first\nsecond line", cleaned)

	require.Len(t, mt.zones, 1)
	z := mt.zones[0]
	assert.Equal(t, 0, z.startX)
	assert.Equal(t, 1, z.startY)
	assert.Equal(t, 10, z.endX)
	assert.Equal(t, 1, z.endY)
}

func TestSweep_PreservesNonMarkerCSI(t *testing.T) {
	mt := &Tracker{}
	content := mt.Mark("btn", "text") + "\x1b[2J"
	cleaned := mt.Sweep(content)
	assert.Equal(t, "text\x1b[2J", cleaned)
}

func TestSweep_OSCDoesNotAffectZoneCoordinates(t *testing.T) {
	mt := &Tracker{}
	// OSC hyperlink wrapping text before a marked zone
	osc := "\x1b]8;;https://example.com\x07Link\x1b]8;;\x07"
	content := osc + " " + mt.Mark("btn", "Click")
	cleaned := mt.Sweep(content)
	assert.Equal(t, osc+" Click", cleaned)

	require.Len(t, mt.zones, 1)
	z := mt.zones[0]
	// "Link" is 4 visible cols, plus 1 space = startX at 5
	assert.Equal(t, 5, z.startX)
	assert.Equal(t, 9, z.endX)
}

func TestResolve_SideBySideRectangular(t *testing.T) {
	mt := &Tracker{}

	left := mt.Mark("submit", "+---------+\n| Submit  |\n+---------+")
	right := mt.Mark("cancel", "+--------+\n| Cancel |\n+--------+")

	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")
	combined := make([]string, len(leftLines))
	for i := range leftLines {
		combined[i] = leftLines[i] + " " + rightLines[i]
	}
	content := strings.Join(combined, "\n")

	mt.Sweep(content)

	require.Len(t, mt.zones, 2)

	assert.Equal(t, "submit", mt.Resolve(5, 1))
	assert.Equal(t, "cancel", mt.Resolve(16, 1))
	assert.Equal(t, "", mt.Resolve(11, 1))
}
