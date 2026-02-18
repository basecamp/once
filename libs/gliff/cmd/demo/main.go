package main

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"github.com/basecamp/gliff/components"
	"github.com/basecamp/gliff/tui"
)

// TickMsg signals a timer tick.
type TickMsg struct{}

// ClickExpiredMsg signals that a click highlight should expire.
type ClickExpiredMsg struct {
	id int // component identifier
}

// ANSI codes for click highlight
const (
	orangeBG = "\x1b[48;5;208m"
	resetBG  = "\x1b[49m"
)

// Header displays a title bar.
type Header struct {
	width   int
	clicked bool
}

func (h *Header) Init() tui.Cmd { return nil }

func (h *Header) Update(msg tui.Msg) tui.Cmd {
	switch msg := msg.(type) {
	case tui.ComponentSizeMsg:
		h.width = msg.Width
	case tui.MouseMsg:
		if msg.Type == tui.MousePress && msg.Button == tui.MouseLeft {
			h.clicked = true
			return tui.After(time.Second, func(time.Time) tui.Msg {
				return ClickExpiredMsg{id: 1}
			})
		}
	case ClickExpiredMsg:
		if msg.id == 1 {
			h.clicked = false
		}
	}
	return nil
}

func (h *Header) Render() string {
	if h.width < 1 {
		return ""
	}
	title := " Stack Layout Demo "
	var content string
	if h.width < len(title) {
		content = strings.Repeat("═", h.width)
	} else {
		padding := h.width - len(title)
		left := padding / 2
		right := padding - left
		content = strings.Repeat("═", left) + title + strings.Repeat("═", right)
	}
	if h.clicked {
		return orangeBG + content + resetBG
	}
	return content
}

// Footer displays a status bar.
type Footer struct {
	width   int
	clicked bool
}

func (f *Footer) Init() tui.Cmd { return nil }

func (f *Footer) Update(msg tui.Msg) tui.Cmd {
	switch msg := msg.(type) {
	case tui.ComponentSizeMsg:
		f.width = msg.Width
	case tui.MouseMsg:
		if msg.Type == tui.MousePress && msg.Button == tui.MouseLeft {
			f.clicked = true
			return tui.After(time.Second, func(time.Time) tui.Msg {
				return ClickExpiredMsg{id: 2}
			})
		}
	case ClickExpiredMsg:
		if msg.id == 2 {
			f.clicked = false
		}
	}
	return nil
}

func (f *Footer) Render() string {
	if f.width < 1 {
		return ""
	}
	hint := " Press Ctrl+C to quit "
	var content string
	if f.width < len(hint) {
		content = strings.Repeat("─", f.width)
	} else {
		padding := f.width - len(hint)
		left := padding / 2
		right := padding - left
		content = strings.Repeat("─", left) + hint + strings.Repeat("─", right)
	}
	if f.clicked {
		return orangeBG + content + resetBG
	}
	return content
}

// WindowInfo displays the current window dimensions.
type WindowInfo struct {
	width       int
	height      int
	clicked     bool
	clickX      int
	clickY      int
	progressBar *components.ProgressBar
}

func (w *WindowInfo) Init() tui.Cmd {
	w.progressBar = &components.ProgressBar{
		Width:   20,
		Total:   100,
		Current: 0,
		Color:   color.RGBA{R: 0, G: 0xff, B: 0, A: 0xff},
	}
	return tui.Every(50*time.Millisecond, func(time.Time) tui.Msg {
		return progressUpdateMsg{}
	})
}

type progressUpdateMsg struct{}

func (w *WindowInfo) Update(msg tui.Msg) tui.Cmd {
	switch msg := msg.(type) {
	case tui.ComponentSizeMsg:
		w.width = msg.Width
		w.height = msg.Height
		if w.progressBar != nil {
			w.progressBar.Width = max(0, msg.Width-4)
		}
	case progressUpdateMsg:
		if w.progressBar != nil {
			w.progressBar.Current += 1
			if w.progressBar.Current > w.progressBar.Total {
				w.progressBar.Current = 0
			}
		}
		return tui.Every(50*time.Millisecond, func(time.Time) tui.Msg {
			return progressUpdateMsg{}
		})
	case tui.MouseMsg:
		if msg.Type == tui.MousePress && msg.Button == tui.MouseLeft {
			w.clicked = true
			w.clickX = msg.RelX
			w.clickY = msg.RelY
			return tui.After(time.Second, func(time.Time) tui.Msg {
				return ClickExpiredMsg{id: 3}
			})
		}
	case ClickExpiredMsg:
		if msg.id == 3 {
			w.clicked = false
		}
	}
	return nil
}

func (w *WindowInfo) Render() string {
	if w.width < 4 || w.height < 2 {
		return ""
	}

	var lines []string
	inner := w.width - 2

	lines = append(lines, "┌"+strings.Repeat("─", inner)+"┐")

	title := " Window Info "
	if inner >= len(title) {
		pad := inner - len(title)
		lines = append(lines, "│"+title+strings.Repeat(" ", pad)+"│")
	} else {
		lines = append(lines, "│"+strings.Repeat(" ", inner)+"│")
	}

	lines = append(lines, "├"+strings.Repeat("─", inner)+"┤")

	widthLine := fmt.Sprintf(" Width:  %d ", w.width)
	if inner >= len(widthLine) {
		lines = append(lines, "│"+widthLine+strings.Repeat(" ", inner-len(widthLine))+"│")
	}

	heightLine := fmt.Sprintf(" Height: %d ", w.height)
	if inner >= len(heightLine) {
		lines = append(lines, "│"+heightLine+strings.Repeat(" ", inner-len(heightLine))+"│")
	}

	// Add progress bar line
	if w.progressBar != nil && inner >= 4 {
		progressLine := " " + w.progressBar.Render() + " "
		lines = append(lines, "│"+progressLine+strings.Repeat(" ", max(0, inner-len([]rune(progressLine))))+"│")
	}

	for len(lines) < w.height-1 {
		lines = append(lines, "│"+strings.Repeat(" ", inner)+"│")
	}

	lines = append(lines, "└"+strings.Repeat("─", inner)+"┘")

	if len(lines) > w.height {
		lines = lines[:w.height]
	}

	if w.clicked {
		// Place block at click position
		if w.clickY >= 0 && w.clickY < len(lines) {
			line := []rune(lines[w.clickY])
			if w.clickX >= 0 && w.clickX < len(line) {
				line[w.clickX] = '█'
				lines[w.clickY] = string(line)
			}
		}
		// Wrap each line with orange background
		for i, line := range lines {
			lines[i] = orangeBG + line + resetBG
		}
	}

	return strings.Join(lines, "\n")
}

// Timer displays the current time.
type Timer struct {
	width        int
	height       int
	clicked      bool
	clickX       int
	clickY       int
	progressBusy *components.ProgressBusy
}

func (t *Timer) Init() tui.Cmd {
	t.progressBusy = components.NewProgressBusy(20, color.RGBA{R: 0xff, G: 0, B: 0xff, A: 0xff})
	return tui.Batch(
		t.progressBusy.Init(),
		tui.Every(time.Second/60, func(time.Time) tui.Msg {
			return TickMsg{}
		}),
	)
}

func (t *Timer) Update(msg tui.Msg) tui.Cmd {
	var cmds []tui.Cmd

	// Forward messages to progressBusy
	if t.progressBusy != nil {
		if cmd := t.progressBusy.Update(msg); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	switch msg := msg.(type) {
	case tui.KeyMsg:
		if msg.Type == tui.KeyCtrlC {
			return func() tui.Msg { return tui.QuitMsg{} }
		}
	case TickMsg:
		cmds = append(cmds, tui.Every(time.Second/60, func(time.Time) tui.Msg {
			return TickMsg{}
		}))
	case tui.ComponentSizeMsg:
		t.width = msg.Width
		t.height = msg.Height
		if t.progressBusy != nil {
			t.progressBusy.Width = max(0, msg.Width-4)
		}
	case tui.MouseMsg:
		if msg.Type == tui.MousePress && msg.Button == tui.MouseLeft {
			t.clicked = true
			t.clickX = msg.RelX
			t.clickY = msg.RelY
			cmds = append(cmds, tui.After(time.Second, func(time.Time) tui.Msg {
				return ClickExpiredMsg{id: 4}
			}))
		}
	case ClickExpiredMsg:
		if msg.id == 4 {
			t.clicked = false
		}
	}

	return tui.Batch(cmds...)
}

func (t *Timer) Render() string {
	if t.width < 4 || t.height < 2 {
		return ""
	}

	var lines []string
	inner := t.width - 2

	lines = append(lines, "┌"+strings.Repeat("─", inner)+"┐")

	title := " Timer "
	if inner >= len(title) {
		pad := inner - len(title)
		lines = append(lines, "│"+title+strings.Repeat(" ", pad)+"│")
	} else {
		lines = append(lines, "│"+strings.Repeat(" ", inner)+"│")
	}

	lines = append(lines, "├"+strings.Repeat("─", inner)+"┤")

	timeStr := " " + time.Now().Format("15:04:05.000") + " "
	if inner >= len(timeStr) {
		lines = append(lines, "│"+timeStr+strings.Repeat(" ", inner-len(timeStr))+"│")
	}

	// Add progress busy line
	if t.progressBusy != nil && inner >= 4 {
		busyLine := " " + t.progressBusy.Render() + " "
		lines = append(lines, "│"+busyLine+strings.Repeat(" ", max(0, inner-len([]rune(busyLine))))+"│")
	}

	for len(lines) < t.height-1 {
		lines = append(lines, "│"+strings.Repeat(" ", inner)+"│")
	}

	lines = append(lines, "└"+strings.Repeat("─", inner)+"┘")

	if len(lines) > t.height {
		lines = lines[:t.height]
	}

	if t.clicked {
		// Place block at click position
		if t.clickY >= 0 && t.clickY < len(lines) {
			line := []rune(lines[t.clickY])
			if t.clickX >= 0 && t.clickX < len(line) {
				line[t.clickX] = '█'
				lines[t.clickY] = string(line)
			}
		}
		// Wrap each line with orange background
		for i, line := range lines {
			lines[i] = orangeBG + line + resetBG
		}
	}

	return strings.Join(lines, "\n")
}

func main() {
	// Create individual components
	header := &Header{}
	footer := &Footer{}
	windowInfo := &WindowInfo{}
	timer := &Timer{}

	// Create horizontal layout for the middle section
	middle := components.NewStackLayout(components.Horizontal,
		components.Child{Component: windowInfo, Size: components.Percent(50)},
		components.Child{Component: timer, Size: components.Fill{}},
	)

	// Create the main vertical layout: header, middle, footer
	root := components.NewStackLayout(components.Vertical,
		components.Child{Component: header, Size: components.Fixed(1)},
		components.Child{Component: middle, Size: components.Fill{}},
		components.Child{Component: footer, Size: components.Fixed(1)},
	)

	app := tui.NewApplication(root)
	if err := app.Run(); err != nil {
		panic(err)
	}
}
