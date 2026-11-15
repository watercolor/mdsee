package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestHeadingsAndFuzzySearch(t *testing.T) {
	source := "# 开始\n\n```md\n# 假标题\n```\n\n中文标题\n--------\n\n## Final **section**\n"
	items := headings(source)
	if len(items) != 3 || items[1].row != 6 || items[2].level != 2 {
		t.Fatalf("headings: %+v", items)
	}
	if !fuzzyMatch("中文标题", "中题") || fuzzyMatch("标题", "题标") {
		t.Fatal("fuzzy order")
	}
	m := testModel(t)
	m.editor = newEditor(source)
	m.Update(tea.KeyPressMsg{Code: tea.KeyF8})
	if m.nav.kind != "目录" || len(m.nav.items) != 3 {
		t.Fatal("F8 must open headings")
	}
	m.navigationInput(tea.PasteMsg{Content: "Final"})
	m.navigationInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.editor.row != 9 || m.nav.kind != "" {
		t.Fatal("heading selection must navigate")
	}
}

func TestSearchUnicodeWrapAndDeferredRender(t *testing.T) {
	m := testModel(t)
	m.editor = newEditor("中文猫猫\n猫")
	m.searchQuery = "猫"
	m.linked = false
	if len(searchMatches(m.editor.lines, "猫")) != 3 {
		t.Fatal("Unicode matches")
	}
	m.findMatch(0)
	if m.editor.row != 0 || m.editor.col != 2 {
		t.Fatal("first match")
	}
	m.acceptRender(renderedMsg{revision: m.revision, content: "中文猫猫\n猫", source: m.editor.text(), mapping: []scrollAnchor{{0, 0}, {1, 1}}})
	if m.editor.col != 2 {
		t.Fatal("render lost match column")
	}
	m.findMatch(1)
	if m.editor.col != 3 {
		t.Fatal("next")
	}
	m.findMatch(1)
	if m.editor.row != 1 {
		t.Fatal("next row")
	}
	m.findMatch(1)
	if m.editor.row != 0 || m.editor.col != 2 {
		t.Fatal("wrap")
	}
	m.findMatch(-1)
	if m.editor.row != 1 {
		t.Fatal("reverse wrap")
	}
	highlighted := highlightMatches("\x1b[31m中文猫猫\x1b[0m", "猫")
	if ansi.Strip(highlighted) != "中文猫猫" || !strings.Contains(highlighted, "48;2;") {
		t.Fatalf("highlight: %q", highlighted)
	}
}

func TestBuffersPreserveEditsCursorAndQuit(t *testing.T) {
	m := testModel(t)
	m.editor.insert("edited")
	m.editor.col = 3
	m.storeBuffer()
	path := filepath.Join(t.TempDir(), "second.md")
	if err := os.WriteFile(path, []byte("second"), 0600); err != nil {
		t.Fatal(err)
	}
	m.openFile(path)
	m.editor.col = 2
	m.switchBuffer(0)
	m.acceptRender(renderedMsg{revision: m.revision, content: "edited", source: "edited", mapping: []scrollAnchor{{0, 0}}})
	if m.editor.text() != "edited" || m.editor.col != 3 {
		t.Fatal("buffer state lost")
	}
	m.switchBuffer(1)
	if !m.hasDirtyBuffers() || m.editor.col != 2 {
		t.Fatal("inactive dirty state lost")
	}
	m.Update(press('q', tea.ModCtrl))
	if !m.confirmQuit {
		t.Fatal("must protect all buffers")
	}
	m.confirmQuit = false
	if !m.saveAllBuffers() || m.hasDirtyBuffers() {
		t.Fatal("save all")
	}
}

func TestDiscoverMarkdown(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.md", "sub/中文.markdown", ".git/hidden.md", "node_modules/hidden.md", "a.txt"} {
		path := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(path), 0700)
		os.WriteFile(path, []byte("x"), 0600)
	}
	items, err := discoverMarkdown(root)
	if err != nil || len(items) != 2 {
		t.Fatalf("%+v %v", items, err)
	}
}

func TestVimNavigationBindings(t *testing.T) {
	m := testModel(t)
	m.vim.enabled = true
	m.previewFocus = true
	m.editor = newEditor("# A\ncat cat\n## B\ncat")
	m.Update(press('/', 0))
	if m.nav.kind != "搜索" {
		t.Fatal("/ must open search")
	}
	m.Update(tea.PasteMsg{Content: "cat"})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.editor.row != 1 || m.editor.col != 0 {
		t.Fatal("search first match")
	}
	m.Update(press('n', 0))
	if m.editor.col != 4 {
		t.Fatal("n must find next")
	}
	m.Update(tea.KeyPressMsg{Code: 'n', ShiftedCode: 'N', Text: "N", Mod: tea.ModShift})
	if m.editor.col != 0 {
		t.Fatal("N must find previous")
	}
	m.Update(press('/', 0))
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.editor.col != 4 {
		t.Fatal("empty search repeats previous pattern")
	}
	m.Update(press('o', 0))
	if m.nav.kind != "目录" {
		t.Fatal("preview o must open directory")
	}
	m.Update(press('j', 0))
	if m.nav.selected != 1 || m.nav.query != "" {
		t.Fatal("j must navigate")
	}
	m.Update(press('k', 0))
	if m.nav.selected != 0 {
		t.Fatal("k must navigate")
	}
	m.Update(press('/', 0))
	m.Update(tea.KeyPressMsg{Code: 'b', ShiftedCode: 'B', Text: "B", Mod: tea.ModShift})
	if len(m.navItems()) != 1 {
		t.Fatal("directory filter")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.nav.kind != "目录" || m.nav.filtering {
		t.Fatal("Esc must leave filter")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m.previewFocus = false
	m.editor = newEditor("one\ntwo")
	m.Update(press('o', 0))
	if !m.vim.insert || len(m.editor.lines) != 3 || m.nav.kind != "" {
		t.Fatal("Normal o must start a new line")
	}
	for _, r := range "/nNo" {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if !strings.Contains(m.editor.text(), "/nNo") {
		t.Fatal("Insert keys must insert text")
	}
}

func TestTabBindingsPreserveInsert(t *testing.T) {
	m := testModel(t)
	m.vim.enabled = true
	m.storeBuffer()
	m.openFile(filepath.Join(t.TempDir(), "two.md"))
	m.switchBuffer(0)
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.activeBuffer != 1 {
		t.Fatal("Normal Tab must switch")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.activeBuffer != 0 {
		t.Fatal("Shift+Tab must switch back")
	}
	m.vim.insert = true
	m.previewFocus = false
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.editor.text() != "\t" || m.activeBuffer != 0 {
		t.Fatal("Insert Tab must indent")
	}
}

func TestDirectoryOverlayDimensions(t *testing.T) {
	m := testModel(t)
	m.editor = newEditor("# 目录\n" + strings.Repeat("## 中文长标题\n", 40))
	m.openNavigation("目录")
	for _, size := range [][2]int{{100, 30}, {42, 16}, {30, 10}} {
		m.resize(size[0], size[1])
		m.nav.selected = 33
		overlay := m.directoryView()
		if ansi.StringWidth(strings.Split(overlay, "\n")[0]) > size[0] || len(strings.Split(overlay, "\n")) > size[1] {
			t.Fatalf("overlay exceeds terminal %v", size)
		}
		plain := ansi.Strip(overlay)
		if !strings.Contains(plain, "34/41") || !strings.Contains(plain, "H2") {
			t.Fatal(plain)
		}
	}
}

func TestDirectoryOverlayPreservesBackground(t *testing.T) {
	m := testModel(t)
	m.resize(100, 30)
	m.editor = newEditor("# 标题\n正文")
	m.preview.SetContent("BACKGROUND")
	m.previewFocus = true
	m.openNavigation("目录")
	view := ansi.Strip(m.View().Content)
	rows := strings.Split(view, "\n")
	if !strings.Contains(rows[0], "MDSEE") || !strings.Contains(view, "BACKGROUND") {
		t.Fatal("overlay must preserve the document background")
	}
	if strings.Index(rows[0], "目录 (") >= 0 {
		t.Fatal("overlay must be vertically centered")
	}
	for _, row := range rows {
		if strings.Index(row, "目录 (") > 3 {
			return
		}
	}
	t.Fatal("overlay must have a horizontal margin")
}
