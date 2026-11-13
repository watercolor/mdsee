package main

import (
	"fmt"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type snapshot struct {
	text     string
	row, col int
}

type editor struct {
	lines               []string
	row, col, top, left int
	undo, redo          []snapshot
}

func newEditor(text string) editor { return editor{lines: strings.Split(text, "\n")} }
func (e *editor) text() string     { return strings.Join(e.lines, "\n") }
func (e *editor) state() snapshot  { return snapshot{e.text(), e.row, e.col} }

func (e *editor) restore(s snapshot) {
	e.lines, e.row, e.col = strings.Split(s.text, "\n"), s.row, s.col
}

func pushHistory(stack []snapshot, s snapshot) []snapshot {
	stack = append(stack, s)
	size, start := 0, len(stack)
	for start > 0 && len(stack)-start < 100 {
		n := len(stack[start-1].text)
		if size+n > 8*1024*1024 {
			break
		}
		size += n
		start--
	}
	return append([]snapshot(nil), stack[start:]...)
}

func (e *editor) undoEdit(redo bool) {
	from, to := &e.undo, &e.redo
	if redo {
		from, to = to, from
	}
	if len(*from) == 0 {
		return
	}
	*to = pushHistory(*to, e.state())
	s := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	e.restore(s)
}

func (e *editor) insert(text string) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, text)
	line := []rune(e.lines[e.row])
	parts := strings.Split(text, "\n")
	before, after := string(line[:e.col]), string(line[e.col:])
	if len(parts) == 1 {
		e.lines[e.row] = before + text + after
		e.col += len([]rune(text))
		return
	}
	parts[0] = before + parts[0]
	lastCol := len([]rune(parts[len(parts)-1]))
	parts[len(parts)-1] += after
	lines := append([]string(nil), e.lines[:e.row]...)
	lines = append(lines, parts...)
	e.lines = append(lines, e.lines[e.row+1:]...)
	e.row += len(parts) - 1
	e.col = lastCol
}

func (e *editor) update(msg tea.Msg, height int) {
	before := e.state()
	switch msg := msg.(type) {
	case tea.PasteMsg:
		e.insert(msg.Content)
	case tea.KeyPressMsg:
		line := []rune(e.lines[e.row])
		switch msg.String() {
		case "ctrl+z":
			e.undoEdit(false)
			return
		case "ctrl+y":
			e.undoEdit(true)
			return
		case "left":
			if e.col > 0 {
				e.col--
			} else if e.row > 0 {
				e.row--
				e.col = len([]rune(e.lines[e.row]))
			}
		case "right":
			if e.col < len(line) {
				e.col++
			} else if e.row+1 < len(e.lines) {
				e.row++
				e.col = 0
			}
		case "up":
			e.row = max(0, e.row-1)
		case "down":
			e.row = min(len(e.lines)-1, e.row+1)
		case "pgup":
			e.row = max(0, e.row-max(1, height-1))
		case "pgdown":
			e.row = min(len(e.lines)-1, e.row+max(1, height-1))
		case "home", "ctrl+a":
			e.col = 0
		case "end", "ctrl+e":
			e.col = len(line)
		case "ctrl+home":
			e.row, e.col = 0, 0
		case "ctrl+end":
			e.row = len(e.lines) - 1
			e.col = len([]rune(e.lines[e.row]))
		case "enter":
			e.insert("\n")
		case "tab":
			e.insert("\t")
		case "backspace":
			if e.col > 0 {
				e.lines[e.row] = string(line[:e.col-1]) + string(line[e.col:])
				e.col--
			} else if e.row > 0 {
				e.col = len([]rune(e.lines[e.row-1]))
				e.lines[e.row-1] += e.lines[e.row]
				e.lines = append(e.lines[:e.row], e.lines[e.row+1:]...)
				e.row--
			}
		case "delete", "ctrl+d":
			if e.col < len(line) {
				e.lines[e.row] = string(line[:e.col]) + string(line[e.col+1:])
			} else if e.row+1 < len(e.lines) {
				e.lines[e.row] += e.lines[e.row+1]
				e.lines = append(e.lines[:e.row+1], e.lines[e.row+2:]...)
			}
		case "ctrl+k":
			e.lines[e.row] = string(line[:e.col])
		case "ctrl+u":
			e.lines[e.row] = string(line[e.col:])
			e.col = 0
		default:
			if msg.Text != "" && msg.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper) == 0 {
				e.insert(msg.Text)
			}
		}
	}
	e.col = min(e.col, len([]rune(e.lines[e.row])))
	if e.text() != before.text {
		if len(e.text()) > maxDocumentBytes {
			e.restore(before)
			return
		}
		e.undo = pushHistory(e.undo, before)
		e.redo = nil
	}
}

func expandTabs(s string) string {
	var b strings.Builder
	column := 0
	for _, r := range s {
		if r == '\t' {
			n := 4 - column%4
			b.WriteString(strings.Repeat(" ", n))
			column += n
		} else {
			b.WriteRune(r)
			column += ansi.StringWidth(string(r))
		}
	}
	return b.String()
}

func (e *editor) layout(width, height int) {
	e.top = min(e.top, e.row)
	if e.row >= e.top+height {
		e.top = e.row - height + 1
	}
	col := ansi.StringWidth(expandTabs(string([]rune(e.lines[e.row])[:e.col])))
	textWidth := max(1, width-e.gutter())
	e.left = min(e.left, col)
	if col >= e.left+textWidth {
		e.left = col - textWidth + 1
	}
	// Keep the horizontal origin on a cell boundary, including wide glyphs.
	line, offset := expandTabs(e.lines[e.row]), 0
	for len(line) > 0 && offset < e.left {
		cluster, w := ansi.FirstGraphemeCluster(line, ansi.GraphemeWidth)
		offset += w
		line = line[len(cluster):]
	}
	e.left = offset
}

func (e *editor) gutter() int { return max(4, len(fmt.Sprint(len(e.lines)))+2) }

func (e *editor) view(width, height int) string {
	rows := make([]string, height)
	gutter := e.gutter()
	for y := 0; y < height; y++ {
		i := e.top + y
		if i >= len(e.lines) {
			rows[y] = strings.Repeat(" ", width)
			continue
		}
		line := expandTabs(e.lines[i])
		prefix := lipgloss.NewStyle().Foreground(lipgloss.Color("#7b8496")).Render(fmt.Sprintf("%*d  ", gutter-2, i+1))
		visible := ansi.Truncate(ansi.Cut(line, e.left, e.left+max(1, width-gutter)), max(1, width-gutter), "")
		rows[y] = prefix + visible + strings.Repeat(" ", max(0, width-gutter-ansi.StringWidth(visible)))
	}
	return strings.Join(rows, "\n")
}
