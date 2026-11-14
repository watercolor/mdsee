package app

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

type navItem struct {
	label, path        string
	row, level, buffer int
}
type navigation struct {
	kind, query string
	items       []navItem
	selected    int
	loading     bool
	filtering   bool
}
type filesFoundMsg struct {
	items []navItem
	err   error
}
type matchPosition struct{ row, col int }
type documentBuffer struct {
	doc    *document
	editor editor
	vim    vimState
	offset int
	query  string
}

func headings(source string) []navItem {
	raw := []byte(source)
	root := goldmark.New().Parser().Parse(text.NewReader(raw))
	var items []navItem
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		h, ok := n.(*ast.Heading)
		if entering && ok && h.Lines().Len() > 0 {
			label := string(h.Text(raw))
			row := strings.Count(source[:h.Lines().At(0).Start], "\n")
			items = append(items, navItem{label: label, row: row, level: h.Level})
		}
		return ast.WalkContinue, nil
	})
	return items
}

func fuzzyMatch(label, query string) bool {
	remaining := []rune(strings.ToLower(query))
	for _, r := range strings.ToLower(label) {
		if len(remaining) > 0 && r == remaining[0] {
			remaining = remaining[1:]
		}
	}
	return len(remaining) == 0
}

func (m *model) navItems() []navItem {
	var items []navItem
	for _, item := range m.nav.items {
		if fuzzyMatch(item.label, m.nav.query) {
			items = append(items, item)
		}
	}
	return items
}

func discoverMarkdown(root string) ([]navItem, error) {
	var items []navItem
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if entry.Type().IsRegular() && (ext == ".md" || ext == ".markdown") {
			rel, _ := filepath.Rel(root, path)
			items = append(items, navItem{label: rel, path: path})
		}
		return nil
	})
	return items, err
}

func (m *model) openNavigation(kind string) tea.Cmd {
	m.vim.clearNavigation()
	m.nav = navigation{kind: kind}
	switch kind {
	case "目录":
		m.nav.items = headings(m.editor.text())
		row := m.editor.row
		if m.previewFocus && m.mapSource == m.editor.text() {
			row = mappedLine(m.mapping, m.preview.YOffset(), true)
		}
		for i, item := range m.nav.items {
			if item.row <= row {
				m.nav.selected = i
			}
		}
	case "缓冲区":
		m.storeBuffer()
		for i, b := range m.buffers {
			label := b.doc.path
			if b.editor.text() != b.doc.text {
				label += " • 未保存"
			}
			m.nav.items = append(m.nav.items, navItem{label: label, buffer: i})
		}
	case "文件":
		m.nav.loading = true
		root, err := os.Getwd()
		if err != nil {
			m.nav.loading = false
			m.status = err.Error()
			return nil
		}
		for parent := root; ; parent = filepath.Dir(parent) {
			if _, err := os.Stat(filepath.Join(parent, ".git")); err == nil {
				root = parent
				break
			}
			if filepath.Dir(parent) == parent {
				break
			}
		}
		m.status = "文件发现：" + root
		return func() tea.Msg { items, err := discoverMarkdown(root); return filesFoundMsg{items, err} }
	case "搜索":
		m.nav.query = ""
	}
	return nil
}

func inputQuery(query string, msg tea.Msg) string {
	switch msg := msg.(type) {
	case tea.PasteMsg:
		return query + strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return -1
			}
			return r
		}, msg.Content)
	case tea.KeyPressMsg:
		if msg.String() == "backspace" {
			r := []rune(query)
			return string(r[:max(0, len(r)-1)])
		}
		if msg.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper) == 0 && msg.Text != "" {
			return query + msg.Text
		}
	}
	return query
}

func (m *model) navigationInput(msg tea.Msg) tea.Cmd {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		if m.nav.kind == "目录" && !m.nav.filtering {
			switch key.String() {
			case "j":
				m.nav.selected = min(max(0, len(m.navItems())-1), m.nav.selected+1)
				return nil
			case "k":
				m.nav.selected = max(0, m.nav.selected-1)
				return nil
			case "/":
				m.nav.filtering = true
				return nil
			case "o":
				m.nav = navigation{}
				return nil
			}
		}
		switch key.String() {
		case "esc":
			if m.nav.kind == "目录" && m.nav.filtering {
				m.nav.filtering = false
				return nil
			}
			m.nav = navigation{}
			return nil
		case "up":
			m.nav.selected = max(0, m.nav.selected-1)
			return nil
		case "down":
			m.nav.selected = min(max(0, len(m.navItems())-1), m.nav.selected+1)
			return nil
		case "enter":
			if m.nav.kind == "搜索" {
				if m.nav.query != "" {
					m.searchQuery = m.nav.query
				}
				m.nav = navigation{}
				return m.findMatch(1)
			}
			items := m.navItems()
			if len(items) == 0 {
				return nil
			}
			item := items[min(m.nav.selected, len(items)-1)]
			kind := m.nav.kind
			m.nav = navigation{}
			switch kind {
			case "目录":
				return m.navigateTo(item.row, 0)
			case "缓冲区":
				return m.switchBuffer(item.buffer)
			case "文件":
				return m.openFile(item.path)
			}
		}
	}
	before := m.nav.query
	m.nav.query = inputQuery(m.nav.query, msg)
	if m.nav.kind == "目录" && m.nav.query != before {
		m.nav.filtering = true
	}
	m.nav.selected = 0
	return nil
}

func (m *model) navigationView() string {
	lines := []string{clipped(m.nav.kind+" > "+m.nav.query, m.width-4), "Enter 确认 · Esc 返回 · ↑↓ 选择"}
	if m.nav.kind == "搜索" {
		lines = append(lines, fmt.Sprintf("%d 处字面匹配", len(searchMatches(m.editor.lines, m.nav.query))))
	} else {
		items := m.navItems()
		start := max(0, m.nav.selected-max(1, m.bodyHeight-5)/2)
		for i := start; i < len(items) && len(lines) < m.bodyHeight; i++ {
			prefix := "  "
			if i == m.nav.selected {
				prefix = "› "
			}
			lines = append(lines, clipped(prefix+strings.Repeat("  ", max(0, items[i].level-1))+items[i].label, m.width-4))
		}
		if m.nav.loading {
			lines = append(lines, "扫描 Markdown 文件中…")
		} else if len(items) == 0 {
			lines = append(lines, "没有匹配结果")
		}
	}
	return m.panel(m.nav.kind, strings.Join(lines, "\n"), m.width-4, true)
}

func searchMatches(lines []string, query string) []matchPosition {
	if query == "" {
		return nil
	}
	var found []matchPosition
	for row, line := range lines {
		for offset := 0; offset <= len(line); {
			pos := strings.Index(line[offset:], query)
			if pos < 0 {
				break
			}
			pos += offset
			found = append(found, matchPosition{row, len([]rune(line[:pos]))})
			offset = pos + len(query)
		}
	}
	return found
}

func (m *model) findMatch(direction int) tea.Cmd {
	matches := searchMatches(m.editor.lines, m.searchQuery)
	if len(matches) == 0 {
		m.status = "搜索无匹配"
		return nil
	}
	index := 0
	if direction < 0 {
		index = len(matches) - 1
	}
	for i, p := range matches {
		after := p.row > m.editor.row || (p.row == m.editor.row && p.col > m.editor.col)
		same := p.row == m.editor.row && p.col == m.editor.col
		before := p.row < m.editor.row || (p.row == m.editor.row && p.col < m.editor.col)
		if direction >= 0 && (after || (direction == 0 && same)) {
			index = i
			break
		}
		if direction < 0 && before {
			index = i
		}
	}
	count, _ := m.vim.motionCount()
	if direction > 0 {
		index = (index + (count-1)%len(matches)) % len(matches)
	}
	if direction < 0 {
		index = (index - (count-1)%len(matches) + len(matches)) % len(matches)
	}
	p := matches[index]
	cmd := m.navigateTo(p.row, p.col)
	m.status = fmt.Sprintf("搜索 %q · %d/%d", m.searchQuery, index+1, len(matches))
	return cmd
}

func highlightMatches(content, query string) string {
	if query == "" {
		return content
	}
	rows := strings.Split(content, "\n")
	style := lipgloss.NewStyle().Background(lipgloss.Color("#facc15")).Foreground(lipgloss.Color("#111827"))
	for i, row := range rows {
		plain := ansi.Strip(row)
		offset, last := 0, 0
		var out strings.Builder
		for offset < len(plain) {
			pos := strings.Index(plain[offset:], query)
			if pos < 0 {
				break
			}
			pos += offset
			start := ansi.StringWidth(plain[:pos])
			end := ansi.StringWidth(plain[:pos+len(query)])
			out.WriteString(ansi.Cut(row, last, start))
			out.WriteString(style.Render(plain[pos : pos+len(query)]))
			last = end
			offset = pos + len(query)
		}
		if offset > 0 {
			out.WriteString(ansi.Cut(row, last, ansi.StringWidth(row)))
			rows[i] = out.String()
		}
	}
	return strings.Join(rows, "\n")
}

func (m *model) jumpHeading(direction int) tea.Cmd {
	items := headings(m.editor.text())
	row := m.editor.row
	if m.previewFocus && m.mapSource == m.editor.text() {
		row = mappedLine(m.mapping, m.preview.YOffset(), true)
	}
	if direction > 0 {
		for _, h := range items {
			if h.row > row {
				return m.navigateTo(h.row, 0)
			}
		}
	} else {
		for i := len(items) - 1; i >= 0; i-- {
			if items[i].row < row {
				return m.navigateTo(items[i].row, 0)
			}
		}
	}
	m.status = "已到章节边界"
	return nil
}

func (m *model) storeBuffer() {
	b := documentBuffer{m.doc, m.editor, m.vim, m.preview.YOffset(), m.searchQuery}
	if len(m.buffers) == 0 {
		m.buffers = append(m.buffers, b)
		m.activeBuffer = 0
	} else {
		m.buffers[m.activeBuffer] = b
	}
}

func (m *model) switchBuffer(index int) tea.Cmd {
	m.storeBuffer()
	if index < 0 || index >= len(m.buffers) {
		return nil
	}
	m.activeBuffer = index
	b := m.buffers[index]
	m.doc, m.editor, m.vim, m.searchQuery = b.doc, b.editor, b.vim, b.query
	m.mapping = nil
	m.diagramKeys = nil
	m.mapSource = ""
	m.pendingLineJump = 0
	m.pendingNavigation = nil
	offset := b.offset
	m.restorePreviewOffset = &offset
	m.preview.SetContent("")
	m.preview.SetYOffset(b.offset)
	m.editor.layout(m.editorWidth, m.bodyHeight)
	return m.scheduleRender()
}

func (m *model) openFile(path string) tea.Cmd {
	d, err := openDocument(path)
	if err != nil {
		m.status = err.Error()
		return nil
	}
	m.storeBuffer()
	for i, b := range m.buffers {
		if b.doc.path == d.path {
			return m.switchBuffer(i)
		}
	}
	m.buffers = append(m.buffers, documentBuffer{doc: d, editor: newEditor(d.text), vim: vimState{enabled: m.vim.enabled}})
	return m.switchBuffer(len(m.buffers) - 1)
}

func (m *model) hasDirtyBuffers() bool {
	if m.dirty() {
		return true
	}
	for i, b := range m.buffers {
		if i != m.activeBuffer && b.editor.text() != b.doc.text {
			return true
		}
	}
	return false
}

func (m *model) saveAllBuffers() bool {
	m.storeBuffer()
	for _, b := range m.buffers {
		if b.editor.text() != b.doc.text {
			if err := b.doc.save(b.editor.text()); err != nil {
				m.status = err.Error()
				return false
			}
		}
	}
	m.status = "已保存全部文件"
	return true
}

func (m *model) navigateTo(row, col int) tea.Cmd {
	m.vim.clearNavigation()
	m.restorePreviewOffset = nil
	m.pendingLineJump = 0
	m.editor.row = max(0, min(row, len(m.editor.lines)-1))
	m.editor.col = max(0, min(col, len([]rune(m.editor.lines[m.editor.row]))))
	m.editor.layout(m.editorWidth, m.bodyHeight)
	if m.mapSource != m.editor.text() || len(m.mapping) == 0 {
		m.pendingNavigation = &matchPosition{m.editor.row, m.editor.col}
		return m.scheduleRender()
	}
	m.preview.SetYOffset(mappedLine(m.mapping, m.editor.row, false))
	return nil
}

func (m *model) directoryView() string {
	width := min(110, max(24, m.width-8))
	inner := width - 4
	capacity := max(1, min(28, m.height-8))
	items := m.navItems()
	selected := min(m.nav.selected, max(0, len(items)-1))
	start := max(0, min(selected-capacity/2, len(items)-capacity))
	end := min(len(items), start+capacity)
	bg, fg, border, selection := "#222638", "#c4cbe3", "#65708e", "#454d68"
	if m.theme == "light" {
		bg, fg, border, selection = "#f1f5f9", "#253047", "#64748b", "#cbd5e1"
	}
	base := lipgloss.NewStyle().Background(lipgloss.Color(bg)).Foreground(lipgloss.Color(fg)).Width(inner)
	title := fmt.Sprintf("目录 (%d/%d)", min(selected+1, len(items)), len(items))
	lines := []string{base.Bold(true).Render(clipped(title, inner))}
	if m.nav.filtering || m.nav.query != "" {
		lines = append(lines, base.Render(clipped("/"+m.nav.query, inner)))
	}
	for i := start; i < end; i++ {
		item := items[i]
		marker := "  "
		if i == selected {
			marker = "› "
		}
		label := marker + strings.Repeat("  ", max(0, item.level-1)) + item.label
		label = clipped(label, inner-4)
		row := label + strings.Repeat(" ", max(0, inner-ansi.StringWidth(label)-2)) + fmt.Sprintf("H%d", item.level)
		style := base
		if i == selected {
			style = style.Background(lipgloss.Color(selection)).Bold(true)
		}
		lines = append(lines, style.Render(row))
	}
	if len(items) == 0 {
		lines = append(lines, base.Render("没有匹配标题"))
	}
	scroll := ""
	if start > 0 {
		scroll += " ↑"
	}
	if end < len(items) {
		scroll += " ↓"
	}
	hint := "j/k ↑↓ 选择 · Enter 跳转 · / 过滤 · Esc 关闭" + scroll
	if m.nav.filtering {
		hint = "输入标题过滤 · Enter 跳转 · Esc 返回选择"
	}
	lines = append(lines, base.Render(clipped(hint, inner)))
	return lipgloss.NewStyle().Background(lipgloss.Color(bg)).Foreground(lipgloss.Color(fg)).
		Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(border)).Padding(0, 1).Width(width).
		Render(strings.Join(lines, "\n"))
}
