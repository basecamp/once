package ui

import (
	"math/rand/v2"
	"strings"
	"time"

	"github.com/basecamp/gliff/tui"
)

const (
	starCount = 100
	starSpeed = 0.03
	starMinZ  = 0.1
	starMaxZ  = 3.0
	starTick  = 33 * time.Millisecond
)

var (
	starBright = "\x1b[38;2;255;255;255m" // white
	starDim    = "\x1b[38;2;136;136;136m" // grey
	starReset  = "\x1b[0m"
)

type starfieldTickMsg struct{}

type star struct {
	x, y, z float64
}

type starCell struct {
	ch     rune
	bright bool
}

type Starfield struct {
	width, height int
	stars         []star
	rng           *rand.Rand
	grid          [][]starCell
}

func NewStarfield() *Starfield {
	return &Starfield{
		rng: rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())),
	}
}

func (s *Starfield) Init() tui.Cmd {
	return s.scheduleTick()
}

func (s *Starfield) Update(msg tui.Msg) tui.Cmd {
	switch msg := msg.(type) {
	case tui.WindowSizeMsg:
		s.resize(msg.Width, msg.Height)
	case starfieldTickMsg:
		s.step()
		return s.scheduleTick()
	}
	return nil
}

// RenderRow renders starfield cells for columns [fromCol, toCol) on the given row.
func (s *Starfield) RenderRow(row, fromCol, toCol int) string {
	if row < 0 || row >= s.height {
		return strings.Repeat(" ", max(toCol-fromCol, 0))
	}

	var sb strings.Builder
	for col := fromCol; col < toCol; col++ {
		if col < 0 || col >= s.width {
			sb.WriteByte(' ')
			continue
		}
		cell := &s.grid[row][col]
		if cell.ch == 0 {
			sb.WriteByte(' ')
		} else {
			if cell.bright {
				sb.WriteString(starBright)
			} else {
				sb.WriteString(starDim)
			}
			sb.WriteRune(cell.ch)
			sb.WriteString(starReset)
		}
	}
	return sb.String()
}

// RenderFullRow renders a full-width starfield row.
func (s *Starfield) RenderFullRow(row int) string {
	return s.RenderRow(row, 0, s.width)
}

// ComputeGrid projects all stars onto the cell grid.
// Must be called before RenderRow.
func (s *Starfield) ComputeGrid() {
	if s.width <= 0 || s.height <= 0 {
		return
	}

	// Reset grid
	for row := range s.height {
		for col := range s.width {
			s.grid[row][col] = starCell{}
		}
	}

	subW := s.width * 2
	subH := s.height * 4
	centerX := float64(subW) / 2
	centerY := float64(subH) / 2

	for i := range s.stars {
		st := &s.stars[i]
		if st.z <= 0 {
			continue
		}

		sx := centerX + st.x/st.z
		sy := centerY + st.y/st.z

		sxi := int(sx)
		syi := int(sy)
		if sxi < 0 || sxi >= subW || syi < 0 || syi >= subH {
			continue
		}

		col := sxi / 2
		row := syi / 4
		dotCol := sxi % 2
		dotRow := syi % 4
		dotIndex := 3 - dotRow

		cell := &s.grid[row][col]
		if cell.ch == 0 {
			cell.ch = 0x2800
		}
		if dotCol == 0 {
			cell.ch |= leftDots[dotIndex]
		} else {
			cell.ch |= rightDots[dotIndex]
		}

		if st.z < starMaxZ/2 {
			cell.bright = true
		}
	}
}

// Private

func (s *Starfield) resize(width, height int) {
	s.width = width
	s.height = height
	s.stars = make([]star, starCount)
	for i := range s.stars {
		s.stars[i] = s.randomStar()
	}
	s.grid = make([][]starCell, height)
	for row := range height {
		s.grid[row] = make([]starCell, width)
	}
}

func (s *Starfield) step() {
	subW := s.width * 2
	subH := s.height * 4
	centerX := float64(subW) / 2
	centerY := float64(subH) / 2

	for i := range s.stars {
		st := &s.stars[i]
		st.z -= starSpeed

		if st.z <= starMinZ {
			s.stars[i] = s.randomStar()
			continue
		}

		sx := centerX + st.x/st.z
		sy := centerY + st.y/st.z
		if sx < 0 || sx >= float64(subW) || sy < 0 || sy >= float64(subH) {
			s.stars[i] = s.randomStar()
		}
	}
}

func (s *Starfield) randomStar() star {
	spread := float64(max(s.width, s.height))
	return star{
		x: (s.rng.Float64() - 0.5) * spread,
		y: (s.rng.Float64() - 0.5) * spread,
		z: starMinZ + s.rng.Float64()*(starMaxZ-starMinZ),
	}
}

func (s *Starfield) scheduleTick() tui.Cmd {
	return tui.After(starTick, func() tui.Msg {
		return starfieldTickMsg{}
	})
}
