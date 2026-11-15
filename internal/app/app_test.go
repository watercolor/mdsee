package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func testModel(t *testing.T) *model {
	t.Helper()
	d, err := openDocument(filepath.Join(t.TempDir(), "note.md"))
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(d, splitMode, "dark", false)
	t.Cleanup(m.close)
	return m
}

func TestViewModesAndFocus(t *testing.T) {
	m := testModel(t)
	m.Update(tea.PasteMsg{Content: "draft"})
	m.Update(press('w', tea.ModCtrl))
	m.preview.SetContent(strings.Repeat("preview line\n", 100))
	m.Update(press(tea.KeyDown))
	if m.preview.YOffset() != 1 || m.editor.row != 0 {
		t.Fatal("预览焦点下方向键应滚动预览，不应移动编辑光标")
	}
	m.Update(press(tea.KeyUp))
	if m.preview.YOffset() != 0 {
		t.Fatal("预览应支持向上滚动")
	}
	m.Update(tea.PasteMsg{Content: "must not insert"})
	if m.editor.text() != "draft" {
		t.Fatal("预览焦点不应修改原文")
	}
	m.Update(press('w', tea.ModCtrl))
	if m.previewFocus || m.editor.text() != "draft" {
		t.Fatal("Ctrl+W 应返回编辑区并保留原文")
	}
	for _, want := range []viewMode{editMode, previewMode, splitMode} {
		m.Update(press('p', tea.ModCtrl))
		if m.mode != want {
			t.Fatal(m.mode)
		}
	}
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 18})
	if m.actualMode() != editMode {
		t.Fatal("窄屏默认显示编辑")
	}
	m.Update(press(tea.KeyF6))
	if m.actualMode() != previewMode {
		t.Fatal("窄屏应能切换预览")
	}
}

func TestQuitProtectionAndSaving(t *testing.T) {
	m := testModel(t)
	m.Update(tea.PasteMsg{Content: "unsaved"})
	_, cmd := m.Update(press('q', tea.ModCtrl))
	if cmd != nil || !m.confirmQuit {
		t.Fatal("未保存时应先提示")
	}
	m.Update(tea.PasteMsg{Content: "ignored"})
	if m.editor.text() != "unsaved" {
		t.Fatal("退出提示不应接受粘贴")
	}
	m.Update(press(tea.KeyEscape))
	m.Update(press('s', tea.ModCtrl))
	if m.dirty() {
		t.Fatal("保存后应清除修改标记")
	}
	got, err := os.ReadFile(m.doc.path)
	if err != nil || string(got) != "unsaved" {
		t.Fatalf("%q %v", got, err)
	}
	_, cmd = m.Update(press('q', tea.ModCtrl))
	if cmd == nil {
		t.Fatal("已保存应直接退出")
	}
}

func TestStalePreviewIsNotDisplayed(t *testing.T) {
	m := testModel(t)
	m.preview.SetContent("current")
	m.revision = 5
	m.busy = true
	_, cmd := m.Update(renderedMsg{revision: 4, content: "stale"})
	if strings.Contains(m.preview.GetContent(), "stale") || cmd == nil {
		t.Fatal("过期结果应丢弃并刷新")
	}
}

func TestLayoutFitsTerminal(t *testing.T) {
	for _, size := range [][2]int{{30, 10}, {50, 18}, {80, 24}, {121, 35}} {
		m := testModel(t)
		m.editor = newEditor(strings.Repeat("中文长行：\tcode ", 30) + "\nsecond line")
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, mode := range []viewMode{splitMode, editMode, previewMode} {
			m.mode = mode
			m.resize(size[0], size[1])
			panel := m.panel("原文", m.editor.view(m.editorWidth, m.bodyHeight, m.theme), m.editorWidth, true)
			if got := len(strings.Split(panel, "\n")); got != m.bodyHeight+3 {
				t.Fatalf("panel wrapped: %d rows, want %d", got, m.bodyHeight+3)
			}
			content := m.View().Content
			lines := strings.Split(content, "\n")
			if len(lines) > size[1] {
				t.Fatalf("too tall: %d > %d", len(lines), size[1])
			}
			for _, line := range lines {
				if ansi.StringWidth(line) > size[0] {
					t.Fatalf("too wide: %d > %d", ansi.StringWidth(line), size[0])
				}
			}
		}
	}
}

func TestPrintAndInvalidCLI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(path, []byte("# hello\n\n$x^2$"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Run([]string{"--print", path}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "x²") || strings.Contains(out.String(), "\x1b") {
		t.Fatal(out.String())
	}
	for _, args := range [][]string{{"--mode", "invalid"}, {"--theme", "invalid"}, {"--width", "0"}, {"--print", path + "missing"}, {"one.md", "two.md"}} {
		if err := Run(args, &out); err == nil {
			t.Fatalf("应拒绝参数 %v", args)
		}
	}
}

func TestHelpNavigationAndCursorVisibility(t *testing.T) {
	for _, width := range []int{30, 50, 100, 120} {
		m := testModel(t)
		m.vim.enabled = true
		m.Update(tea.WindowSizeMsg{Width: width, Height: 18})
		m.Update(press(tea.KeyF1))
		view := m.View()
		if view.Cursor != nil {
			t.Fatal("帮助页不应显示编辑光标")
		}
		if !strings.Contains(view.Content, "快捷键指南") {
			t.Fatal("缺少帮助标题")
		}
		if len(strings.Split(view.Content, "\n")) > 18 {
			t.Fatal("帮助页超出终端高度")
		}
		for _, line := range strings.Split(view.Content, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatal("帮助页超出终端宽度")
			}
		}
		m.Update(press(tea.KeyEnd))
		if !strings.Contains(m.helpViewport.View(), "内部寄存器") {
			t.Fatal("帮助底部必须可以滚动访问")
		}
		m.Update(tea.PasteMsg{Content: "ignored"})
		if m.editor.text() != "" {
			t.Fatal("帮助页不能接受编辑输入")
		}
		m.Update(press(tea.KeyEscape))
		if m.help || m.View().Cursor == nil {
			t.Fatal("关闭帮助后应恢复光标")
		}
		m.Update(press('w', tea.ModCtrl))
		if m.View().Cursor != nil {
			t.Fatal("预览区不应显示编辑光标")
		}
	}
}

func TestSpacePagesPreviewAndPreservesEditing(t *testing.T) {
	for _, vim := range []bool{true, false} {
		m := testModel(t)
		m.vim.enabled = vim
		m.previewFocus = true
		m.preview.SetContent(strings.Repeat("line\n", 100))
		m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
		if m.preview.YOffset() == 0 {
			t.Fatal("space must page down in preview")
		}
		m.previewFocus = false
		m.vim.insert = true
		m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
		if m.editor.text() != " " {
			t.Fatal("space must insert in editing mode")
		}
		m.openNavigation("搜索")
		m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
		if m.nav.query != " " {
			t.Fatal("space must remain part of the search query")
		}
	}
}

func TestPreviewQuitShortcutAndUnsavedBuffers(t *testing.T) {
	for _, mode := range []viewMode{previewMode, splitMode} {
		for _, vim := range []bool{false, true} {
			m := testModel(t)
			m.mode, m.previewFocus, m.vim.enabled = mode, true, vim
			_, cmd := m.Update(press('q'))
			if cmd == nil {
				t.Fatal("q did not quit the focused preview", mode, vim)
			}
			if _, quit := cmd().(tea.QuitMsg); !quit {
				t.Fatal("q returned an unexpected command")
			}
			m.editor = newEditor("unsaved")
			_, cmd = m.Update(press('q'))
			if cmd != nil || !m.confirmQuit {
				t.Fatal("q bypassed the unsaved-file prompt")
			}
			m.Update(press(tea.KeyEscape))
			if m.confirmQuit || m.editor.text() != "unsaved" {
				t.Fatal("canceling quit changed the document")
			}
			m.storeBuffer()
			doc, err := openDocument(filepath.Join(t.TempDir(), "second.md"))
			if err != nil {
				t.Fatal(err)
			}
			m.buffers = append(m.buffers, documentBuffer{doc: doc, editor: newEditor("")})
			m.switchBuffer(1)
			_, cmd = m.Update(press('q'))
			if cmd != nil || !m.confirmQuit {
				t.Fatal("q ignored an unsaved inactive buffer")
			}
		}
	}
}

func TestQStaysInEditingAndModalInput(t *testing.T) {
	key := tea.KeyPressMsg{Code: 'q', Text: "q"}
	for _, vim := range []bool{false, true} {
		m := testModel(t)
		m.vim.enabled, m.vim.insert = vim, true
		m.Update(key)
		if m.confirmQuit {
			t.Fatal("typing q opened quit confirmation")
		}
		if m.editor.text() != "q" {
			t.Fatal("editing q did not insert text")
		}
	}
	m := testModel(t)
	m.vim.enabled = true
	_, cmd := m.Update(key)
	if cmd != nil || m.confirmQuit || m.editor.text() != "" {
		t.Fatal("q quit from the source editor in Normal mode")
	}
	m.mode, m.previewFocus, m.vim.enabled = previewMode, true, true
	m.openNavigation("搜索")
	m.Update(key)
	if m.nav.query != "q" || m.confirmQuit {
		t.Fatal("q did not remain in the search input")
	}
	m.Update(press(tea.KeyEscape))
	m.help = true
	_, cmd = m.Update(key)
	if cmd != nil || !m.help || m.confirmQuit {
		t.Fatal("q quit from the help screen")
	}
	m.help = false
	m.vim.commandOpen = true
	m.Update(key)
	if m.vim.command != "q" || m.confirmQuit {
		t.Fatal("q did not remain in the Vim command input")
	}
}

func TestShortcutHintsStayVisibleWithMermaidStatus(t *testing.T) {
	m := testModel(t)
	m.mode, m.previewFocus = previewMode, true
	m.diagrams.binary = "mmdc"
	m.diagrams.entries = map[string]*mermaidImageEntry{
		"running": {loading: true, running: true},
		"cached":  {path: "ready.png", diskHit: true},
		"failed":  {problem: "mmdc 缺少浏览器，使用字符回退"},
	}
	for _, key := range []string{"running", "cached", "failed"} {
		m.diagramKeys = []string{key}
		for _, width := range []int{30, 50, 80, 120, 180} {
			m.width = width
			footer := m.statusLine("VIM NORMAL · 联动开 · 1:1")
			for _, want := range []string{"o 目录", "/ 搜索", "q 退出"} {
				if !strings.Contains(footer, want) {
					t.Fatalf("%d-column footer lost %q: %s", width, want, footer)
				}
			}
			for _, unwanted := range []string{"mmdc", "缓存", "字符回退", "F4", "F7", "Ctrl+W"} {
				if strings.Contains(footer, unwanted) {
					t.Fatal("unexpected footer hint", footer)
				}
			}
			if ansi.StringWidth(footer) > width || ansi.StringWidth(m.headerLine(" MDCLI note.md")) > width {
				t.Fatal("status exceeded terminal width", width)
			}
		}
	}
	m.width, m.previewFocus, m.vim.enabled = 180, false, true
	m.vim.insert = false
	if footer := m.statusLine("NORMAL"); !strings.Contains(footer, "/ 搜索") || strings.Contains(footer, "q 退出") {
		t.Fatal("incorrect Normal-mode shortcuts", footer)
	}
	m.vim.insert = true
	if footer := m.statusLine("INSERT"); strings.Contains(footer, "/ 搜索") || !strings.Contains(footer, "Ctrl+Q 退出") {
		t.Fatal("incorrect insert-mode shortcuts", footer)
	}
	m.previewFocus, m.help = true, true
	if footer := m.statusLine("NORMAL"); strings.Contains(footer, "q 退出") || !strings.Contains(footer, "Esc 返回") {
		t.Fatal("help advertised a preview-only quit key", footer)
	}
	m.help = false
	m.openNavigation("目录")
	if footer := m.statusLine("NORMAL"); strings.Contains(footer, "q 退出") || !strings.Contains(footer, "Enter 确认") {
		t.Fatal("navigation advertised a preview-only quit key", footer)
	}
}
