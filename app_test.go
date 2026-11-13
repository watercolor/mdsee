package main

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
	return newModel(d, splitMode, "dark")
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
			panel := m.panel("原文", m.editor.view(m.editorWidth, m.bodyHeight), m.editorWidth, true)
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
	if err := run([]string{"--print", path}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "x²") || strings.Contains(out.String(), "\x1b") {
		t.Fatal(out.String())
	}
	for _, args := range [][]string{{"--mode", "invalid"}, {"--theme", "invalid"}, {"--width", "0"}, {"--print", path + "missing"}, {"one.md", "two.md"}} {
		if err := run(args, &out); err == nil {
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
