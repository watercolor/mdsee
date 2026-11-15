package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func press(code rune, mods ...tea.KeyMod) tea.KeyPressMsg {
	var mod tea.KeyMod
	if len(mods) > 0 {
		mod = mods[0]
	}
	return tea.KeyPressMsg{Code: code, Mod: mod}
}

func TestEditingUnicodeTabsAndUndo(t *testing.T) {
	e := newEditor("你好\nworld\n")
	e.update(press(tea.KeyEnd), 10)
	e.update(tea.PasteMsg{Content: "，中文\n\tGo"}, 10)
	if got := e.text(); got != "你好，中文\n\tGo\nworld\n" {
		t.Fatalf("got %q", got)
	}
	e.update(press('z', tea.ModCtrl), 10)
	if got := e.text(); got != "你好\nworld\n" {
		t.Fatalf("undo: %q", got)
	}
	e.update(press('y', tea.ModCtrl), 10)
	e.update(press(tea.KeyBackspace), 10)
	if got := e.text(); got != "你好，中文\n\tG\nworld\n" {
		t.Fatalf("redo/delete: %q", got)
	}
}

func TestJoinLinesAndNewEditClearsRedo(t *testing.T) {
	e := newEditor("first\nsecond")
	e.update(press(tea.KeyDown), 10)
	e.update(press(tea.KeyBackspace), 10)
	if e.text() != "firstsecond" || e.col != 5 {
		t.Fatal(e.state())
	}
	e.update(press('z', tea.ModCtrl), 10)
	e.update(tea.PasteMsg{Content: "new"}, 10)
	e.update(press('y', tea.ModCtrl), 10)
	if e.text() != "first\nnewsecond" {
		t.Fatal(e.text())
	}
}

func TestEditorLongDocumentsAndViewBounds(t *testing.T) {
	e := newEditor(strings.Repeat("中文\tlong content", 100) + strings.Repeat("\nline", 300))
	e.update(press(tea.KeyEnd), 10)
	e.layout(25, 10)
	if e.left == 0 {
		t.Fatal("长行应横向滚动")
	}
	for _, line := range strings.Split(e.view(25, 10, "dark"), "\n") {
		if ansi.StringWidth(line) != 25 {
			t.Fatalf("width %d: %q", ansi.StringWidth(line), line)
		}
	}
	e.update(press(tea.KeyEnd, tea.ModCtrl), 10)
	e.update(press(tea.KeyEnter), 10)
	if len(e.lines) != 302 {
		t.Fatal("长文档不应被截断")
	}
	e.layout(25, 10)
	if e.row >= e.top+10 {
		t.Fatal("光标应保持可见")
	}
}
