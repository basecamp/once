package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMark(t *testing.T) {
	mt := &MouseTracker{}

	result := mt.Mark("btn", "Click")
	assert.Equal(t, "\x1b[0zClick\x1b[0z", result)

	result = mt.Mark("link", "Here")
	assert.Equal(t, "\x1b[1zHere\x1b[1z", result)
}

func TestSweep_StripsMarkers(t *testing.T) {
	mt := &MouseTracker{}
	content := mt.Mark("btn", "Click me")
	cleaned := mt.Sweep(content)
	assert.Equal(t, "Click me", cleaned)
}

func TestSweep_PreservesANSI(t *testing.T) {
	mt := &MouseTracker{}
	content := mt.Mark("btn", "\x1b[31mRed\x1b[0m")
	cleaned := mt.Sweep(content)
	assert.Equal(t, "\x1b[31mRed\x1b[0m", cleaned)
}

func TestSweep_NoMarkers(t *testing.T) {
	mt := &MouseTracker{}
	content := "plain text\nline two"
	cleaned := mt.Sweep(content)
	assert.Equal(t, content, cleaned)
}

func TestSweep_SingleTarget(t *testing.T) {
	mt := &MouseTracker{}
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
	mt := &MouseTracker{}
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
	mt := &MouseTracker{}
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
	mt := &MouseTracker{}
	content := mt.Mark("wide", "中文")
	cleaned := mt.Sweep(content)
	assert.Equal(t, "中文", cleaned)

	require.Len(t, mt.zones, 1)
	z := mt.zones[0]
	assert.Equal(t, 0, z.startX)
	assert.Equal(t, 3, z.endX) // 2 wide chars = 4 cols, endX = 4-1 = 3
}

func TestSweep_NestedTargets(t *testing.T) {
	mt := &MouseTracker{}
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
	mt := &MouseTracker{}

	mt.Mark("a", "first")
	mt.Sweep(mt.Mark("a", "first"))

	// Second frame should start IDs from 0 again
	result := mt.Mark("b", "second")
	assert.Equal(t, "\x1b[0zsecond\x1b[0z", result)
}

func TestResolve_Hit(t *testing.T) {
	mt := &MouseTracker{}
	content := "Hello " + mt.Mark("btn", "World")
	mt.Sweep(content)

	assert.Equal(t, "btn", mt.Resolve(6, 0))
	assert.Equal(t, "btn", mt.Resolve(10, 0))
}

func TestResolve_Miss(t *testing.T) {
	mt := &MouseTracker{}
	content := "Hello " + mt.Mark("btn", "World")
	mt.Sweep(content)

	assert.Equal(t, "", mt.Resolve(0, 0))
	assert.Equal(t, "", mt.Resolve(11, 0))
	assert.Equal(t, "", mt.Resolve(6, 1))
}

func TestResolve_Nested(t *testing.T) {
	mt := &MouseTracker{}
	inner := mt.Mark("inner", "click")
	outer := mt.Mark("outer", "before "+inner+" after")
	mt.Sweep(outer)

	// Hit on inner region returns innermost
	assert.Equal(t, "inner", mt.Resolve(7, 0))
	assert.Equal(t, "inner", mt.Resolve(11, 0))

	// Hit on outer but not inner returns outer
	assert.Equal(t, "outer", mt.Resolve(0, 0))
	assert.Equal(t, "outer", mt.Resolve(17, 0))
}

func TestResolve_MultiLineHit(t *testing.T) {
	mt := &MouseTracker{}
	content := "pre " + mt.Mark("block", "line1\nline2\nline3") + " post"
	mt.Sweep(content)

	// Zone is a bounding box from (4,0) to (4,2)
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
	assert.False(t, multiLine.contains(4, 1))  // left of zone
	assert.False(t, multiLine.contains(16, 1)) // right of zone
	assert.False(t, multiLine.contains(10, 3)) // below zone
}

func TestZoneContains_EndAtNewLine(t *testing.T) {
	// When end marker is at col 0 of a new line, endX = -1, so no point
	// can satisfy x >= 0 && x <= -1 — the zone is effectively empty.
	z := zone{name: "z", startX: 0, startY: 0, endX: -1, endY: 1}
	assert.False(t, z.contains(0, 0))
	assert.False(t, z.contains(0, 1))
}

func TestTarget_EndToEnd(t *testing.T) {
	// Use the package-level Target function with the default tracker
	saved := *defaultMouseTracker
	defer func() { *defaultMouseTracker = saved }()
	*defaultMouseTracker = MouseTracker{}

	content := "Click " + WithTarget("link", "here") + " for more"
	cleaned := defaultMouseTracker.Sweep(content)
	assert.Equal(t, "Click here for more", cleaned)
	assert.Equal(t, "link", defaultMouseTracker.Resolve(6, 0))
	assert.Equal(t, "link", defaultMouseTracker.Resolve(9, 0))
	assert.Equal(t, "", defaultMouseTracker.Resolve(5, 0))
	assert.Equal(t, "", defaultMouseTracker.Resolve(10, 0))
}

func TestApplication_MouseTarget(t *testing.T) {
	tracker := &MouseTracker{}
	var lastTarget string

	comp := &testComponent{
		renderFn: func() string {
			return tracker.Mark("btn", "Click")
		},
		updateFn: func(msg Msg) Cmd {
			if m, ok := msg.(MouseMsg); ok {
				lastTarget = m.Target
			}
			return nil
		},
	}

	app := &Application{
		component:    comp,
		mouseTracker: tracker,
	}

	msgs := make(chan Msg, 10)

	// Initialize sets up zones from first render
	scr := &fakeScreen{}
	app.initialize(scr, msgs, 80, 24)

	// Send a mouse click at position (0, 0) — inside the "btn" zone
	msgs <- MouseMsg{Button: MouseLeft, Type: MousePress, X: 0, Y: 0}
	msgs <- QuitMsg{}

	app.eventLoop(scr, msgs)

	assert.Equal(t, "btn", lastTarget)
	// Screen should receive cleaned content (no markers)
	assert.Equal(t, "Click", scr.lastContent)
}

// Helpers

type testComponent struct {
	renderFn func() string
	updateFn func(Msg) Cmd
}

func (c *testComponent) Init() Cmd          { return nil }
func (c *testComponent) Render() string     { return c.renderFn() }
func (c *testComponent) Update(msg Msg) Cmd { return c.updateFn(msg) }

type fakeScreen struct {
	lastContent string
}

func (s *fakeScreen) Render(content string) int {
	s.lastContent = content
	return 0
}

func (s *fakeScreen) Resize(w, h int) {}

func TestSweep_MarkerAtLineStart(t *testing.T) {
	mt := &MouseTracker{}
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
	mt := &MouseTracker{}
	// CSI sequence with a different final byte should be preserved
	content := mt.Mark("btn", "text") + "\x1b[2J"
	cleaned := mt.Sweep(content)
	assert.Equal(t, "text\x1b[2J", cleaned)
}

func TestMark_MarkerFormat(t *testing.T) {
	mt := &MouseTracker{}
	for i := range 5 {
		result := mt.Mark(fmt.Sprintf("zone%d", i), "x")
		expected := fmt.Sprintf("\x1b[%dzx\x1b[%dz", i, i)
		assert.Equal(t, expected, result)
	}
}

func TestResolve_SideBySideRectangular(t *testing.T) {
	mt := &MouseTracker{}

	// Simulate two bordered buttons placed side by side. Target wraps each
	// full multi-line button, then JoinHorizontal splits by \n and combines
	// corresponding lines — so markers end up on the first and last rows only.
	left := mt.Mark("submit", "+---------+\n| Submit  |\n+---------+")
	right := mt.Mark("cancel", "+--------+\n| Cancel |\n+--------+")

	// Simulate JoinHorizontal: combine corresponding lines
	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")
	combined := make([]string, len(leftLines))
	for i := range leftLines {
		combined[i] = leftLines[i] + " " + rightLines[i]
	}
	content := strings.Join(combined, "\n")

	mt.Sweep(content)

	require.Len(t, mt.zones, 2)

	// Submit zone is rectangular: (0,0)-(10,2)
	// Cancel zone is rectangular: (12,0)-(21,2)

	// Click on submit button content (row 1, col 5)
	assert.Equal(t, "submit", mt.Resolve(5, 1))

	// Click on cancel button content (row 1, col 16)
	assert.Equal(t, "cancel", mt.Resolve(16, 1))

	// Click between buttons (the space at col 11) — should miss both
	assert.Equal(t, "", mt.Resolve(11, 1))
}
