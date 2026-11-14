package app

import (
	"strconv"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

type vimState struct {
	enabled, insert  bool
	pending, command string
	count            string
	commandOpen      bool
	register         string
	linewise         bool
}

func (v *vimState) label() string {
	if !v.enabled {
		return "普通"
	}
	if v.insert {
		return "VIM INSERT"
	}
	return "VIM NORMAL"
}

func (v *vimState) reset() {
	v.insert, v.commandOpen, v.pending, v.command, v.count = false, false, "", "", ""
}

func (e *editor) normalCursor() {
	e.row = max(0, min(e.row, len(e.lines)-1))
	e.col = max(0, min(e.col, max(0, len([]rune(e.lines[e.row]))-1)))
}

func (e *editor) recordChange(before snapshot) {
	if e.text() == before.text {
		return
	}
	if len(e.text()) > maxDocumentBytes {
		e.restore(before)
		return
	}
	e.undo = pushHistory(e.undo, before)
	e.redo = nil
}

func wordClass(r rune) int {
	if unicode.IsSpace(r) {
		return 0
	}
	if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
		return 1
	}
	return 2
}

func (m *model) finishEditorInput(before string) tea.Cmd {
	m.pendingLineJump = 0
	if m.vim.enabled && !m.vim.insert {
		m.editor.normalCursor()
	}
	m.editor.layout(m.editorWidth, m.bodyHeight)
	if before != m.editor.text() {
		m.mapping = nil
		return m.scheduleRender()
	}
	m.syncFromEditor()
	return nil
}

func (m *model) vimInput(msg tea.Msg) tea.Cmd {
	v, e := &m.vim, &m.editor
	before := e.text()
	if v.insert && !m.previewFocus {
		if key, ok := msg.(tea.KeyPressMsg); ok && (key.String() == "esc" || key.String() == "ctrl+c") {
			v.insert = false
			e.col = max(0, e.col-1)
		} else {
			e.update(msg, m.bodyHeight)
		}
		return m.finishEditorInput(before)
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	k := key.String()
	if k == "esc" || k == "ctrl+c" {
		v.clearNavigation()
		if v.insert {
			v.insert = false
			e.normalCursor()
		}
		return nil
	}
	if handled, cmd := m.vimMotion(k); handled {
		return cmd
	}
	if k == ":" {
		v.commandOpen = true
		v.command = ""
		v.clearNavigation()
		return nil
	}
	if m.previewFocus {
		v.pending = ""
		return nil
	}
	if v.pending != "" {
		pending := v.pending
		v.pending = ""
		switch pending + k {
		case "dd":
			s := e.state()
			v.register = e.lines[e.row]
			v.linewise = true
			if len(e.lines) == 1 {
				e.lines[0] = ""
			} else {
				e.lines = append(e.lines[:e.row], e.lines[e.row+1:]...)
			}
			e.normalCursor()
			e.recordChange(s)
		case "yy":
			v.register = e.lines[e.row]
			v.linewise = true
			m.status = "已复制一行（内部寄存器）"
		default:
			m.status = "未支持的 Vim 命令：" + pending + k
		}
		return m.finishEditorInput(before)
	}
	s := e.state()
	switch k {
	case "d", "y":
		v.pending = k
	case "i":
		v.insert = true
	case "a":
		e.col = min(e.col+1, len([]rune(e.lines[e.row])))
		v.insert = true
	case "I":
		e.col = 0
		v.insert = true
	case "A":
		e.col = len([]rune(e.lines[e.row]))
		v.insert = true
	case "o", "O":
		at := e.row
		if k == "o" {
			at++
		}
		lines := append([]string(nil), e.lines[:at]...)
		lines = append(lines, "")
		e.lines = append(lines, e.lines[at:]...)
		e.row, e.col = at, 0
		v.insert = true
		e.recordChange(s)
	case "x", "delete":
		line := []rune(e.lines[e.row])
		if len(line) > 0 {
			v.register = string(line[e.col])
			v.linewise = false
			e.lines[e.row] = string(line[:e.col]) + string(line[e.col+1:])
			e.recordChange(s)
		}
	case "p", "P":
		if v.linewise {
			at := e.row
			if k == "p" {
				at++
			}
			lines := append([]string(nil), e.lines[:at]...)
			lines = append(lines, v.register)
			e.lines = append(lines, e.lines[at:]...)
			e.row, e.col = at, 0
		} else if v.register != "" {
			if k == "p" {
				e.col = min(e.col+1, len([]rune(e.lines[e.row])))
			}
			e.insert(v.register)
			e.col = max(0, e.col-1)
		}
		e.recordChange(s)
	case "u":
		e.undoEdit(false)
	case "ctrl+r":
		e.undoEdit(true)
	default:
		m.status = "Vim Normal：i 插入 · F1 帮助"
	}
	return m.finishEditorInput(before)
}

func (m *model) vimScroll(k string, count int, explicit bool) bool {
	delta := 1
	switch k {
	case "ctrl+d", "ctrl+u":
		if !explicit {
			delta = max(1, m.bodyHeight/2)
		}
	case "ctrl+f", "ctrl+b", "pgdown", "pgup":
		delta = max(1, m.bodyHeight-1)
	}
	limit := len(m.editor.lines)
	if m.previewFocus {
		limit = max(m.preview.TotalLineCount(), m.preview.Width())
	}
	distance := min(count, limit/delta+1) * delta
	if m.previewFocus {
		switch k {
		case "j", "down", "ctrl+d", "ctrl+f", "pgdown":
			m.preview.ScrollDown(distance)
		case "k", "up", "ctrl+u", "ctrl+b", "pgup":
			m.preview.ScrollUp(distance)
		case "h", "left":
			m.preview.ScrollLeft(distance)
		case "l", "right":
			m.preview.ScrollRight(distance)
		default:
			return false
		}
		m.syncFromPreview()
		return true
	}
	switch k {
	case "ctrl+u", "ctrl+b", "pgup":
		distance = -distance
	case "ctrl+d", "ctrl+f", "pgdown":
	default:
		return false
	}
	m.editor.row = max(0, min(len(m.editor.lines)-1, m.editor.row+distance))
	m.editor.top = max(0, min(max(0, len(m.editor.lines)-m.bodyHeight), m.editor.top+distance))
	m.editor.normalCursor()
	m.editor.layout(m.editorWidth, m.bodyHeight)
	m.syncFromEditor()
	return true
}

func (m *model) commandInput(msg tea.Msg) tea.Cmd {
	v := &m.vim
	if paste, ok := msg.(tea.PasteMsg); ok {
		for _, r := range paste.Content {
			if !unicode.IsControl(r) && len(v.command) < 128 {
				v.command += string(r)
			}
		}
		return nil
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil
	}
	switch key.String() {
	case "esc", "ctrl+c":
		v.commandOpen = false
		v.command = ""
	case "backspace":
		r := []rune(v.command)
		if len(r) > 0 {
			v.command = string(r[:len(r)-1])
		} else {
			v.commandOpen = false
		}
	case "enter":
		cmd := strings.TrimSpace(v.command)
		v.commandOpen = false
		v.command = ""
		if line, err := strconv.Atoi(cmd); err == nil {
			if line < 1 {
				m.status = "行号从 1 开始"
				return nil
			}
			return m.jumpToSourceLine(line - 1)
		}
		switch cmd {
		case "w":
			m.save()
		case "wq", "x":
			if m.saveAllBuffers() {
				return tea.Quit
			}
		case "q":
			if !m.hasDirtyBuffers() {
				return tea.Quit
			}
			m.status = "未保存；使用 :w 保存或 :q! 放弃修改"
		case "q!":
			return tea.Quit
		default:
			m.status = "未支持的命令：:" + cmd
		}
	default:
		if key.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper) == 0 && len(v.command)+len(key.Text) <= 128 {
			v.command += key.Text
		}
	}
	return nil
}
