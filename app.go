package main

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
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
	mapping  []scrollAnchor
	source   string
	graphics string
	imageIDs []int
}

type imagesReadyMsg struct{ renderedMsg }
type remoteImagesChangedMsg struct{}

type model struct {
	doc                                                  *document
	editor                                               editor
	preview                                              viewport.Model
	helpViewport                                         viewport.Model
	mode                                                 viewMode
	theme                                                string
	width, height, editorWidth, previewWidth, bodyHeight int
	revision                                             int
	busy, previewFocus, confirmQuit, help                bool
	status                                               string
	vim                                                  vimState
	linked                                               bool
	mapping                                              []scrollAnchor
	mapSource                                            string
	pendingLineJump                                      int
	imageMode, imageRequested                            string
	imageIDBase                                          int
	imageIDs                                             []int
	cellRatio                                            float64
	kittySupported, imageTrueColor                       bool
	remoteImages                                         *remoteImageCache
}

func newModel(d *document, mode viewMode, theme string, vim ...bool) *model {
	m := &model{doc: d, editor: newEditor(d.text), mode: mode, theme: theme, preview: viewport.New(), helpViewport: viewport.New()}
	m.previewFocus = mode == previewMode
	m.linked = true
	m.imageIDBase = int(rand.Uint32N(0xffff00)) + 1
	m.cellRatio = 0.5
	m.remoteImages = newRemoteImageCache()
	m.configureImages("auto")
	m.vim.enabled = len(vim) == 0 || vim[0]
	m.resize(100, 30)
	return m
}

func (m *model) Init() tea.Cmd {
	if m.imageMode != "off" && os.Getenv("TMUX") == "" {
		query := "\x1b[16t"
		if m.imageRequested == "auto" {
			query += fmt.Sprintf("\x1b_Ga=q,i=%d,e=T,U=1\x1b\\", imageProbeID)
			query += ansi.RequestNameVersion
		}
		return tea.Batch(m.scheduleRender(), tea.Raw(query), m.waitForRemoteImages())
	}
	return tea.Batch(m.scheduleRender(), m.waitForRemoteImages())
}

func (m *model) waitForRemoteImages() tea.Cmd {
	cache := m.remoteImages
	return func() tea.Msg {
		select {
		case <-cache.updates:
			return remoteImagesChangedMsg{}
		case <-cache.ctx.Done():
			return nil
		}
	}
}

func (m *model) configureImages(mode string) {
	m.imageRequested, m.imageMode = mode, mode
	if mode == "auto" {
		m.imageMode = "blocks"
	}
	if mode == "kitty" && os.Getenv("TMUX") != "" {
		m.imageMode = "blocks"
	}
	if os.Getenv("NO_COLOR") != "" || m.theme == "ascii" {
		m.imageMode = "off"
	}
}

func (m *model) acceptKittySupport() tea.Cmd {
	if m.imageRequested != "auto" || m.imageMode == "off" || os.Getenv("TMUX") != "" {
		return nil
	}
	m.kittySupported = true
	if m.imageTrueColor && m.imageMode != "kitty" {
		m.imageMode = "kitty"
		return m.scheduleRender()
	}
	return nil
}

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
	m.bodyHeight = max(1, h-5)
	m.editorWidth, m.previewWidth = max(1, w-2), max(1, w-2)
	if m.actualMode() == splitMode {
		m.editorWidth, m.previewWidth = w/2-2, w-w/2-2
	}
	m.preview.SetWidth(m.previewWidth)
	m.preview.SetHeight(m.bodyHeight)
	m.editor.layout(m.editorWidth, m.bodyHeight)
	m.resizeHelp()
}

func (m *model) scheduleRender() tea.Cmd {
	m.revision++
	revision := m.revision
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return refreshMsg(revision) })
}

func (m *model) startRender() tea.Cmd {
	m.busy = true
	revision, source, width, theme := m.revision, m.editor.text(), m.previewWidth, m.theme
	images := newImageRenderer(m.imageMode, m.doc.path, theme, width, m.imageIDBase, m.cellRatio)
	images.remote = m.remoteImages
	return func() tea.Msg {
		content, mapping, err := renderMappedMarkdown(source, width, theme, images)
		return renderedMsg{revision: revision, content: content, err: err, mapping: mapping, source: source,
			graphics: images.graphics.String(), imageIDs: images.ids}
	}
}

func (m *model) acceptRender(msg renderedMsg) (tea.Model, tea.Cmd) {
	m.busy = false
	if msg.revision != m.revision {
		return m, m.startRender()
	}
	if msg.err != nil {
		m.status = "预览失败：" + msg.err.Error()
	} else {
		m.preview.SetContent(msg.content)
		m.mapping, m.mapSource = msg.mapping, msg.source
		if m.pendingLineJump > 0 {
			m.jumpToSourceLine(m.pendingLineJump - 1)
		} else if m.previewFocus {
			m.syncFromPreview()
		} else {
			m.syncFromEditor()
		}
	}
	return m, nil
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
	case remoteImagesChangedMsg:
		return m, tea.Batch(m.scheduleRender(), m.waitForRemoteImages())
	case uv.KittyGraphicsEvent:
		if msg.Options.ID == imageProbeID &&
			msg.Options.VirtualPlacement && string(msg.Payload) == "OK" {
			return m, m.acceptKittySupport()
		}
		return m, nil
	case tea.TerminalVersionMsg:
		if itermSupportsImagePlaceholders(msg.Name) {
			return m, m.acceptKittySupport()
		}
		return m, nil
	case uv.CellSizeEvent:
		if msg.Width > 0 && msg.Height > 0 {
			m.cellRatio = float64(msg.Width) / float64(msg.Height)
			return m, m.scheduleRender()
		}
		return m, nil
	case tea.ColorProfileMsg:
		// Placeholder foreground colors encode image IDs and must survive unchanged.
		before := m.imageMode
		m.imageTrueColor = msg.Profile == colorprofile.TrueColor
		m.configureImages(m.imageRequested)
		if msg.Profile == colorprofile.NoTTY || msg.Profile == colorprofile.ASCII {
			m.imageMode = "off"
		} else if !m.imageTrueColor && m.imageMode == "kitty" {
			m.imageMode = "blocks"
		} else if m.imageTrueColor && m.imageMode == "blocks" && m.imageRequested == "auto" && m.kittySupported && os.Getenv("TMUX") == "" {
			m.imageMode = "kitty"
		}
		if before != m.imageMode {
			return m, m.scheduleRender()
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, m.scheduleRender()
	case refreshMsg:
		if int(msg) == m.revision && !m.busy {
			return m, m.startRender()
		}
		return m, nil
	case renderedMsg:
		if msg.revision != m.revision {
			return m, m.startRender()
		}
		if msg.err == nil && (msg.graphics != "" || len(m.imageIDs) > 0) {
			payload := deleteImageResources(m.imageIDs) + msg.graphics
			m.imageIDs = msg.imageIDs
			return m, tea.Sequence(tea.Raw(payload), func() tea.Msg { return imagesReadyMsg{msg} })
		}
		return m.acceptRender(msg)
	case imagesReadyMsg:
		return m.acceptRender(msg.renderedMsg)
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
		if key == "ctrl+q" || (key == "ctrl+c" && !m.vim.enabled) {
			if m.dirty() {
				m.confirmQuit = true
				return m, nil
			}
			return m, tea.Quit
		}
		if key == "f1" {
			m.vim.clearNavigation()
			m.help = !m.help
			if m.help {
				m.resizeHelp()
				m.helpViewport.GotoTop()
			}
			return m, nil
		}
		if m.help {
			if key == "esc" {
				m.help = false
			} else {
				switch key {
				case "home", "g":
					m.helpViewport.GotoTop()
				case "end", "G":
					m.helpViewport.GotoBottom()
				default:
					m.helpViewport, _ = m.helpViewport.Update(msg)
				}
			}
			return m, nil
		}
		if key == "f4" {
			m.vim.enabled = !m.vim.enabled
			m.vim.reset()
			if m.vim.enabled {
				m.editor.normalCursor()
			}
			m.status = "编辑方式：" + m.vim.label()
			return m, nil
		}
		if m.vim.commandOpen {
			return m, m.commandInput(msg)
		}
		switch key {
		case "f7":
			m.vim.clearNavigation()
			m.linked = !m.linked
			if m.linked {
				if m.previewFocus {
					m.syncFromPreview()
				} else {
					m.syncFromEditor()
				}
			}
			return m, nil
		case "ctrl+s", "f2":
			m.vim.clearNavigation()
			m.save()
			return m, nil
		case "ctrl+p", "f5":
			m.vim.clearNavigation()
			m.mode = (m.mode + 1) % 3
			m.previewFocus = m.mode == previewMode
			m.resize(m.width, m.height)
			return m, m.scheduleRender()
		case "ctrl+w", "f6":
			m.vim.clearNavigation()
			if m.mode == splitMode {
				m.previewFocus = !m.previewFocus
				m.resize(m.width, m.height)
				return m, nil
			}
			return m, nil
		case "alt+up":
			m.pendingLineJump = 0
			m.vim.clearNavigation()
			m.preview.ScrollUp(3)
			m.syncFromPreview()
			return m, nil
		case "alt+down":
			m.pendingLineJump = 0
			m.vim.clearNavigation()
			m.preview.ScrollDown(3)
			m.syncFromPreview()
			return m, nil
		case "alt+pgup":
			m.pendingLineJump = 0
			m.vim.clearNavigation()
			m.preview.PageUp()
			m.syncFromPreview()
			return m, nil
		case "alt+pgdown":
			m.pendingLineJump = 0
			m.vim.clearNavigation()
			m.preview.PageDown()
			m.syncFromPreview()
			return m, nil
		}
	}
	// Modal screens also block pasted text and other input messages.
	if m.confirmQuit || m.help {
		return m, nil
	}
	if m.vim.commandOpen {
		return m, m.commandInput(msg)
	}
	if m.vim.enabled {
		return m, m.vimInput(msg)
	}
	if m.previewFocus {
		m.pendingLineJump = 0
		var cmd tea.Cmd
		m.preview, cmd = m.preview.Update(msg)
		m.syncFromPreview()
		return m, cmd
	}
	before := m.editor.text()
	m.editor.update(msg, m.bodyHeight)
	return m, m.finishEditorInput(before)
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
		body = m.helpPanel()
	} else {
		switch m.actualMode() {
		case editMode:
			body = m.panel("原文", m.editor.view(m.editorWidth, m.bodyHeight), m.editorWidth, !m.previewFocus)
		case previewMode:
			body = m.panel("预览 · ↑↓ / PgUp PgDn", m.preview.View(), m.previewWidth, true)
		default:
			left := m.panel("原文", m.editor.view(m.editorWidth, m.bodyHeight), m.editorWidth, !m.previewFocus)
			right := m.panel("预览", m.preview.View(), m.previewWidth, m.previewFocus)
			body = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
		}
	}
	link := "联动关"
	if m.linked {
		link = "联动开"
	}
	status := fmt.Sprintf("%s · %s · %d:%d", m.vim.label(), link, m.editor.row+1, m.editor.col+1)
	if m.vim.pending != "" || m.vim.count != "" {
		status += " · " + m.vim.count + m.vim.pending
	}
	if m.mode == splitMode && m.width < 70 {
		status += " · 窄屏"
	}
	footer := m.statusLine(status)
	if m.vim.commandOpen {
		status = ":" + m.vim.command
		status = ansi.TruncateLeft(status, max(0, ansi.StringWidth(status)-m.width+1), "")
		footer = lipgloss.NewStyle().Width(m.width).Render(status)
	}
	if m.confirmQuit {
		footer = clipped(" 未保存：S 保存并退出 · D 放弃修改 · Esc 返回", m.width)
	}
	content := strings.Join([]string{header, body, footer}, "\n")
	v := tea.NewView(lipgloss.NewStyle().MaxWidth(m.width).MaxHeight(m.height).Render(content))
	v.AltScreen = true
	if !m.help && !m.confirmQuit {
		if m.vim.commandOpen {
			v.Cursor = tea.NewCursor(ansi.StringWidth(status), m.bodyHeight+4)
			v.Cursor.Shape = tea.CursorBar
		} else if !m.previewFocus {
			x := 1 + m.editor.gutter() + ansi.StringWidth(expandTabs(string([]rune(m.editor.lines[m.editor.row])[:m.editor.col]))) - m.editor.left
			v.Cursor = tea.NewCursor(x, 3+m.editor.row-m.editor.top)
			v.Cursor.Shape = tea.CursorBar
			if m.vim.enabled && !m.vim.insert {
				v.Cursor.Shape = tea.CursorBlock
				v.Cursor.Blink = false
			}
		}
	}
	return v
}

func (m *model) statusLine(right string) string {
	right = ansi.Truncate(right, max(0, m.width-1), "…")
	available := max(0, m.width-ansi.StringWidth(right)-1)
	left := ""
	if m.status != "" {
		left = " " + m.status
	} else {
		for _, hint := range []string{
			" F1 帮助  F4 Vim  F7 联动  Ctrl+W 焦点  Ctrl+P 布局  Ctrl+S 保存  Ctrl+Q 退出",
			" F1 帮助  Ctrl+P 布局  Ctrl+Q 退出",
			" F1 帮助",
		} {
			if ansi.StringWidth(hint) <= available {
				left = hint
				break
			}
		}
	}
	left = ansi.Truncate(left, available, "…")
	gap := max(0, m.width-ansi.StringWidth(left)-ansi.StringWidth(right)-1)
	return left + strings.Repeat(" ", gap) + right + " "
}
