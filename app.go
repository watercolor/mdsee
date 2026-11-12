package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type viewMode int

const (
	splitMode viewMode = iota
	editMode
	previewMode
)

func (v viewMode) String() string { return [...]string{"双列", "编辑", "预览"}[v] }

type refreshMsg int
type renderedMsg struct {
	revision int
	content  string
	err      error
}

type model struct {
	doc                                                  *document
	editor                                               editor
	preview                                              viewport.Model
	mode                                                 viewMode
	theme                                                string
	width, height, editorWidth, previewWidth, bodyHeight int
	revision                                             int
	busy, previewFocus, confirmQuit, help                bool
	status                                               string
}

func newModel(d *document, mode viewMode, theme string) *model {
	m := &model{doc: d, editor: newEditor(d.text), mode: mode, theme: theme, preview: viewport.New(), status: "F1 帮助"}
	m.previewFocus = mode == previewMode
	m.resize(100, 30)
	return m
}

func (m *model) Init() tea.Cmd { return m.scheduleRender() }

func (m *model) actualMode() viewMode {
	if m.mode == splitMode && m.width < 70 {
		if m.previewFocus {
			return previewMode
		}
		return editMode
	}
	return m.mode
}

func (m *model) resize(w, h int) {
	m.width, m.height = max(1, w), max(1, h)
	m.bodyHeight = max(1, h-7)
	m.editorWidth, m.previewWidth = max(1, w-2), max(1, w-2)
	if m.actualMode() == splitMode {
		m.editorWidth, m.previewWidth = w/2-2, w-w/2-2
	}
	m.preview.SetWidth(m.previewWidth)
	m.preview.SetHeight(m.bodyHeight)
	m.editor.layout(m.editorWidth, m.bodyHeight)
}

func (m *model) scheduleRender() tea.Cmd {
	m.revision++
	revision := m.revision
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return refreshMsg(revision) })
}

func (m *model) startRender() tea.Cmd {
	m.busy = true
	revision, source, width, theme := m.revision, m.editor.text(), m.previewWidth, m.theme
	return func() tea.Msg {
		content, err := renderMarkdown(source, width, theme)
		return renderedMsg{revision, content, err}
	}
}

func (m *model) dirty() bool { return m.editor.text() != m.doc.text }

func (m *model) save() bool {
	if err := m.doc.save(m.editor.text()); err != nil {
		m.status = "保存失败：" + err.Error()
		return false
	}
	m.status = "已保存 " + filepath.Base(m.doc.path)
	return true
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, m.scheduleRender()
	case refreshMsg:
		if int(msg) == m.revision && !m.busy {
			return m, m.startRender()
		}
		return m, nil
	case renderedMsg:
		m.busy = false
		if msg.revision != m.revision {
			return m, m.startRender()
		}
		if msg.err != nil {
			m.status = "预览失败：" + msg.err.Error()
		} else {
			m.preview.SetContent(msg.content)
		}
		return m, nil
	case tea.KeyPressMsg:
		key := msg.String()
		if m.confirmQuit {
			switch key {
			case "s", "ctrl+s":
				if m.save() {
					return m, tea.Quit
				}
			case "d":
				return m, tea.Quit
			case "esc":
				m.confirmQuit = false
				m.status = "已取消退出"
			}
			return m, nil
		}
		if key == "ctrl+q" || key == "ctrl+c" {
			if m.dirty() {
				m.confirmQuit = true
				return m, nil
			}
			return m, tea.Quit
		}
		if key == "f1" {
			m.help = !m.help
			return m, nil
		}
		if m.help {
			if key == "esc" {
				m.help = false
			}
			return m, nil
		}
		switch key {
		case "ctrl+s", "f2":
			m.save()
			return m, nil
		case "ctrl+p", "f5":
			m.mode = (m.mode + 1) % 3
			m.previewFocus = m.mode == previewMode
			m.resize(m.width, m.height)
			return m, m.scheduleRender()
		case "f6":
			if m.mode == splitMode {
				m.previewFocus = !m.previewFocus
				m.resize(m.width, m.height)
				return m, m.scheduleRender()
			}
			return m, nil
		case "alt+up":
			m.preview.ScrollUp(3)
			return m, nil
		case "alt+down":
			m.preview.ScrollDown(3)
			return m, nil
		case "alt+pgup":
			m.preview.PageUp()
			return m, nil
		case "alt+pgdown":
			m.preview.PageDown()
			return m, nil
		}
	}
	// Modal screens also block pasted text and other input messages.
	if m.confirmQuit || m.help {
		return m, nil
	}
	if m.previewFocus {
		var cmd tea.Cmd
		m.preview, cmd = m.preview.Update(msg)
		return m, cmd
	}
	before := m.editor.text()
	m.editor.update(msg, m.bodyHeight)
	m.editor.layout(m.editorWidth, m.bodyHeight)
	if before != m.editor.text() {
		return m, m.scheduleRender()
	}
	return m, nil
}

func clipped(s string, width int) string { return ansi.Truncate(s, max(1, width), "…") }

func (m *model) panel(title, content string, width int, focused bool) string {
	color := "#515b70"
	if focused {
		color = "#67c8b3"
	}
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(color)).Width(width + 2).Height(m.bodyHeight + 3)
	label := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Bold(true).Render(clipped(" "+title, width))
	return style.Render(label + "\n" + content)
}

func (m *model) View() tea.View {
	if m.width < 30 || m.height < 10 {
		v := tea.NewView(clipped("终端过小，请扩大至 30×10。Ctrl+Q 退出", m.width))
		v.AltScreen = true
		return v
	}
	mark := ""
	if m.dirty() {
		mark = " • 未保存"
	}
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#67c8b3")).Render(clipped(" MDCLI  "+filepath.Base(m.doc.path)+mark+"  / "+m.mode.String(), m.width))
	var body string
	if m.help {
		body = lipgloss.NewStyle().Width(m.width).Height(m.bodyHeight + 3).Render(helpText)
	} else {
		switch m.actualMode() {
		case editMode:
			body = m.panel("原文", m.editor.view(m.editorWidth, m.bodyHeight, !m.previewFocus), m.editorWidth, !m.previewFocus)
		case previewMode:
			body = m.panel("预览 · ↑↓ / PgUp PgDn", m.preview.View(), m.previewWidth, true)
		default:
			left := m.panel("原文", m.editor.view(m.editorWidth, m.bodyHeight, !m.previewFocus), m.editorWidth, !m.previewFocus)
			right := m.panel("预览", m.preview.View(), m.previewWidth, m.previewFocus)
			body = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
		}
	}
	status := fmt.Sprintf(" %d:%d  %s", m.editor.row+1, m.editor.col+1, m.status)
	if m.mode == splitMode && m.width < 70 {
		status += " · 窄屏单栏，F6 切换"
	}
	if m.confirmQuit {
		status = " 未保存：S 保存并退出 · D 放弃修改 · Esc 返回"
	}
	footer := " Ctrl+S 保存  Ctrl+P/F5 切换布局  F6 焦点  Ctrl+Q 退出  F1 帮助"
	content := strings.Join([]string{header, body, clipped(status, m.width), clipped(footer, m.width)}, "\n")
	v := tea.NewView(lipgloss.NewStyle().MaxWidth(m.width).MaxHeight(m.height).Render(content))
	v.AltScreen = true
	return v
}

const helpText = `
 MDCLI · 快捷键

 Ctrl+P / F5       双列 → 编辑 → 预览
 F6                双列模式下切换焦点（窄屏切换可见面板）
 Ctrl+S / F2       保存
 Ctrl+Q / Ctrl+C   退出；未保存时提示
 Ctrl+Z / Ctrl+Y   撤销 / 重做
 ↑ ↓ ← →           移动光标；预览焦点下滚动
 Home / End        行首 / 行尾
 Ctrl+Home / End   文档首 / 尾
 PgUp / PgDn       翻页
 Alt+↑ / ↓         编辑时独立滚动预览
 Tab               插入制表符（显示宽度 4）
 Ctrl+K / Ctrl+U   删除至行尾 / 行首
 终端粘贴快捷键     粘贴文字（支持多行与中文）

 LaTeX：$...$ 行内公式，$$...$$ 块公式。
 公式使用 Unicode 排版；源码保持原样。
 F1 / Esc 关闭帮助。`
