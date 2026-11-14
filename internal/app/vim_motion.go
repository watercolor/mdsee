package app

import (
	"strconv"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

const maxVimCount = 1_000_000_000

func (e *editor) moveWords(motion string, count int) {
	all := []rune(e.text())
	if len(all) == 0 {
		return
	}
	pos := e.col
	for _, line := range e.lines[:e.row] {
		pos += len([]rune(line)) + 1
	}
	pos = min(pos, len(all)-1)
	for n := 0; n < count; n++ {
		old := pos
		switch motion {
		case "b":
			pos = max(0, pos-1)
			for pos > 0 && wordClass(all[pos]) == 0 {
				pos--
			}
			class := wordClass(all[pos])
			for pos > 0 && wordClass(all[pos-1]) == class {
				pos--
			}
		case "e":
			pos = min(len(all)-1, pos+1)
			for pos < len(all)-1 && wordClass(all[pos]) == 0 {
				pos++
			}
			class := wordClass(all[pos])
			for pos < len(all)-1 && wordClass(all[pos+1]) == class {
				pos++
			}
		case "w":
			class := wordClass(all[pos])
			for pos < len(all) && wordClass(all[pos]) == class {
				pos++
			}
			for pos < len(all) && wordClass(all[pos]) == 0 {
				pos++
			}
			pos = min(pos, len(all)-1)
		}
		if pos == old {
			break
		}
	}
	e.row = 0
	for e.row < len(e.lines)-1 && pos > len([]rune(e.lines[e.row])) {
		pos -= len([]rune(e.lines[e.row])) + 1
		e.row++
	}
	e.col = pos
	e.normalCursor()
}

func (v *vimState) clearNavigation() { v.pending, v.count = "", "" }

func (v *vimState) motionCount() (int, bool) {
	if v.count == "" {
		return 1, false
	}
	n, _ := strconv.Atoi(v.count)
	return max(1, n), true
}

func (e *editor) firstNonblank() {
	e.col = 0
	for _, r := range []rune(e.lines[e.row]) {
		if !unicode.IsSpace(r) {
			break
		}
		e.col++
	}
	e.normalCursor()
}

func (e *editor) moveToScreenColumn(column int) {
	line := []rune(e.lines[e.row])
	e.col = 0
	width := 0
	for i, r := range line {
		step := ansi.StringWidth(string(r))
		if r == '\t' {
			step = 4 - width%4
		}
		width += step
		if width > column {
			e.col = i
			return
		}
	}
	e.col = max(0, len(line)-1)
}

func (e *editor) paragraphMotion(back bool, count int) {
	for n := 0; n < count; n++ {
		old := e.row
		delta := 1
		if back {
			delta = -1
		}
		startedBlank := strings.TrimSpace(e.lines[e.row]) == ""
		e.row = max(0, min(len(e.lines)-1, e.row+delta))
		for startedBlank && strings.TrimSpace(e.lines[e.row]) == "" {
			next := e.row + delta
			if next < 0 || next >= len(e.lines) {
				break
			}
			e.row = next
		}
		for strings.TrimSpace(e.lines[e.row]) != "" {
			next := e.row + delta
			if next < 0 || next >= len(e.lines) {
				break
			}
			e.row = next
		}
		if e.row == old {
			break
		}
	}
	e.col = 0
}

func (m *model) jumpToSourceLine(row int) tea.Cmd {
	e := &m.editor
	m.pendingLineJump = 0
	e.row = max(0, min(row, len(e.lines)-1))
	e.firstNonblank()
	if e.row < e.top || e.row >= e.top+m.bodyHeight {
		e.top = max(0, min(e.row-m.bodyHeight/2, max(0, len(e.lines)-m.bodyHeight)))
	}
	e.layout(m.editorWidth, m.bodyHeight)
	m.status = "跳至第 " + strconv.Itoa(e.row+1) + " 行"
	if m.mapSource != e.text() || len(m.mapping) == 0 {
		if m.linked || m.previewFocus {
			m.pendingLineJump = e.row + 1
			return m.scheduleRender()
		}
		return nil
	}
	if m.previewFocus {
		m.alignPreviewToEditor()
	} else {
		m.syncFromEditor()
	}
	return nil
}

func (m *model) vimMotion(k string) (bool, tea.Cmd) {
	v, e := &m.vim, &m.editor
	if v.pending == "d" || v.pending == "y" {
		return false, nil
	}
	if len(k) == 1 && k[0] >= '0' && k[0] <= '9' && (k != "0" || v.count != "") && v.pending == "" {
		n, _ := strconv.Atoi(v.count)
		digit := int(k[0] - '0')
		if n > (maxVimCount-digit)/10 {
			n = maxVimCount
		} else {
			n = n*10 + digit
		}
		v.count = strconv.Itoa(n)
		return true, nil
	}
	n, explicit := v.motionCount()
	if v.pending == "g" || v.pending == "z" {
		k = v.pending + k
		v.pending = ""
	} else if k == "g" || k == "z" {
		v.pending = k
		return true, nil
	}
	switch k {
	case "G", "gg":
		if m.previewFocus && !explicit {
			v.clearNavigation()
			m.pendingLineJump = 0
			if k == "G" {
				m.preview.GotoBottom()
			} else {
				m.preview.GotoTop()
			}
			m.syncFromPreview()
			return true, nil
		}
		row := 0
		if explicit {
			row = n - 1
		} else if k == "G" {
			row = len(e.lines) - 1
		}
		v.clearNavigation()
		return true, m.jumpToSourceLine(row)
	case "%":
		v.clearNavigation()
		if !explicit || n > 100 {
			m.status = "百分比定位请使用 1% 到 100%"
			return true, nil
		}
		return true, m.jumpToSourceLine(max(0, (len(e.lines)*n+99)/100-1))
	case "H", "M", "L":
		m.pendingLineJump = 0
		top, bottom := e.top, min(len(e.lines)-1, e.top+m.bodyHeight-1)
		if m.previewFocus {
			if m.mapSource != e.text() || len(m.mapping) == 0 {
				v.clearNavigation()
				m.status = "预览映射尚未就绪，请稍后定位"
				return true, nil
			}
			top = m.preview.YOffset()
			bottom = min(m.preview.TotalLineCount()-1, top+m.bodyHeight-1)
		}
		target := (top + bottom) / 2
		if k == "H" {
			target = top + min(n-1, bottom-top)
		}
		if k == "L" {
			target = bottom - min(n-1, bottom-top)
		}
		if m.previewFocus {
			target = mappedLine(m.mapping, target, true)
		}
		e.row = max(0, min(len(e.lines)-1, target))
		e.firstNonblank()
		v.clearNavigation()
		if m.previewFocus {
			e.layout(m.editorWidth, m.bodyHeight)
			return true, nil
		}
		return true, m.finishEditorInput(e.text())
	case "h", "left", "j", "down", "k", "up", "l", "right", "ctrl+d", "ctrl+u", "ctrl+f", "ctrl+b", "pgup", "pgdown":
		v.clearNavigation()
		m.pendingLineJump = 0
		if m.previewFocus || strings.HasPrefix(k, "ctrl+") || k == "pgup" || k == "pgdown" {
			return m.vimScroll(k, n, explicit), nil
		}
		switch k {
		case "h", "left":
			e.col = max(0, e.col-n)
		case "l", "right":
			e.col = min(len([]rune(e.lines[e.row])), e.col+n)
		case "j", "down":
			e.row = min(len(e.lines)-1, e.row+n)
		case "k", "up":
			e.row = max(0, e.row-n)
		}
	case "w", "b", "e", "0", "home", "^", "$", "end", "|", "{", "}", "zz", "zt", "zb":
		v.clearNavigation()
		m.pendingLineJump = 0
		if m.previewFocus {
			m.status = "此定位命令用于原文编辑区；Ctrl+W 切换焦点"
			return true, nil
		}
		switch k {
		case "w", "b", "e":
			e.moveWords(k, n)
		case "0", "home":
			e.col = 0
		case "^":
			e.firstNonblank()
		case "$", "end":
			e.row = min(len(e.lines)-1, e.row+n-1)
			e.col = max(0, len([]rune(e.lines[e.row]))-1)
		case "|":
			e.moveToScreenColumn(n - 1)
		case "{", "}":
			e.paragraphMotion(k == "{", n)
		case "zz", "zt", "zb":
			if explicit {
				e.row = min(len(e.lines)-1, n-1)
			}
			switch k {
			case "zz":
				e.top = max(0, e.row-m.bodyHeight/2)
			case "zt":
				e.top = e.row
			case "zb":
				e.top = max(0, e.row-m.bodyHeight+1)
			}
		}
	default:
		if explicit || len(k) > 1 && (k[0] == 'g' || k[0] == 'z') {
			v.clearNavigation()
			m.status = "未支持的带次数命令或定位命令：" + k
			return true, nil
		}
		return false, nil
	}
	return true, m.finishEditorInput(e.text())
}
