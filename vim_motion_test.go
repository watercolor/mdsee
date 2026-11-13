package main

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func motionModel(t *testing.T) *model {
	t.Helper()
	m := testModel(t)
	m.vim.enabled = true
	m.linked = false
	var lines []string
	for i := 1; i <= 100; i++ {
		lines = append(lines, fmt.Sprintf("  line %d alpha beta", i))
	}
	m.editor = newEditor(strings.Join(lines, "\n"))
	m.resize(100, 15)
	return m
}

func TestVimLineNumberAndCountedMotions(t *testing.T) {
	m := motionModel(t)
	before := m.editor.text()
	for _, test := range []struct {
		keys string
		row  int
	}{{"20G", 19}, {"50gg", 49}, {"5j", 54}, {"10k", 44}, {"G", 99}, {"gg", 0}, {"999999999999999999999999G", 99}, {"20G", 19}, {"50%", 49}, {"100%", 99}} {
		vimKeys(m, test.keys)
		if m.editor.row != test.row {
			t.Fatalf("%s: row %d, want %d", test.keys, m.editor.row, test.row)
		}
	}
	vimKeys(m, ":20")
	m.Update(press(tea.KeyEnter))
	if m.editor.row != 19 || m.editor.col != 2 {
		t.Fatal(":20 应进入第20行首个非空白字符", m.editor.state())
	}
	vimKeys(m, ":0")
	m.Update(press(tea.KeyEnter))
	if m.editor.row != 19 {
		t.Fatal("非法行号不应移动光标")
	}
	if m.editor.text() != before {
		t.Fatal("定位不应修改文件内容")
	}
	vimKeys(m, "2")
	m.Update(press('d', tea.ModCtrl))
	if m.editor.row != 21 {
		t.Fatal("2Ctrl+D 应下移两行", m.editor.row)
	}
	vimKeys(m, "2")
	m.Update(press('u', tea.ModCtrl))
	if m.editor.row != 19 {
		t.Fatal("2Ctrl+U 应上移两行", m.editor.row)
	}
}

func TestVimScreenMotionsAndViewportPositioning(t *testing.T) {
	m := motionModel(t)
	m.editor.top = 40
	m.editor.row = 45
	for _, test := range []struct {
		keys string
		row  int
	}{{"H", 40}, {"M", 44}, {"L", 49}, {"3H", 42}, {"3L", 47}, {"999H", 49}, {"999L", 40}} {
		vimKeys(m, test.keys)
		if m.editor.row != test.row || m.editor.top != 40 || m.editor.col != 2 {
			t.Fatalf("%s: row/top/col %d/%d/%d", test.keys, m.editor.row, m.editor.top, m.editor.col)
		}
	}
	vimKeys(m, "50Gzt")
	if m.editor.top != 49 || m.editor.row != 49 {
		t.Fatal("zt 未将当前行放在顶部")
	}
	vimKeys(m, "zz")
	if m.editor.top != 44 || m.editor.row != 49 {
		t.Fatal("zz 未将当前行置中")
	}
	vimKeys(m, "zb")
	if m.editor.top != 40 || m.editor.row != 49 {
		t.Fatal("zb 未将当前行放在底部")
	}
	vimKeys(m, "GztM")
	if m.editor.row != 99 {
		t.Fatal("屏幕定位不能进入文件尾部空白行")
	}
}

func TestVimWordColumnAndParagraphMotions(t *testing.T) {
	m := motionModel(t)
	m.editor = newEditor("\t你好 alpha beta\n\nsecond paragraph\nline\n\nlast")
	vimKeys(m, "^2w")
	if m.editor.col != 10 {
		t.Fatal("2w 未到 beta 词首", m.editor.col)
	}
	vimKeys(m, "b")
	if m.editor.col != 4 {
		t.Fatal("b 未到 alpha 词首", m.editor.col)
	}
	vimKeys(m, "e")
	if m.editor.col != 8 {
		t.Fatal("e 未到 alpha 词尾", m.editor.col)
	}
	vimKeys(m, "10|")
	if m.editor.col != 4 {
		t.Fatal("10| 应按制表符和中文显示宽度定位", m.editor.col)
	}
	vimKeys(m, "0")
	if m.editor.col != 0 {
		t.Fatal("不带次数的 0 应到行首")
	}
	vimKeys(m, "}")
	if m.editor.row != 1 {
		t.Fatalf("} 未到段落边界，row=%d col=%d status=%q", m.editor.row, m.editor.col, m.status)
	}
	vimKeys(m, "}")
	if m.editor.row != 4 {
		t.Fatal("} 未到下一段落边界")
	}
	vimKeys(m, "2{")
	if m.editor.row != 0 {
		t.Fatal("2{ 未回到文件首")
	}
}

func TestVimCountCancellationAndInsertDigits(t *testing.T) {
	m := motionModel(t)
	before := m.editor.text()
	vimKeys(m, "20")
	m.Update(press(tea.KeyEscape))
	vimKeys(m, "j")
	if m.editor.row != 1 || m.vim.count != "" {
		t.Fatal("Esc 应取消待输入次数")
	}
	vimKeys(m, "9")
	m.Update(press('w', tea.ModCtrl))
	m.Update(press('w', tea.ModCtrl))
	vimKeys(m, "j")
	if m.editor.row != 2 {
		t.Fatal("焦点切换应取消次数")
	}
	vimKeys(m, "20x")
	if m.editor.text() != before || m.vim.count != "" {
		t.Fatal("不支持的带次数编辑命令不应误删文字")
	}
	vimKeys(m, "i20G")
	m.Update(press(tea.KeyEscape))
	if !strings.Contains(m.editor.lines[2], "20G") {
		t.Fatal("Insert 中数字应作为文字输入")
	}
}

func TestVimPreviewSourceLineJumpAndDeferredMapping(t *testing.T) {
	m := motionModel(t)
	m.previewFocus = true
	m.linked = true
	vimKeys(m, "20G")
	if m.pendingLineJump != 20 || m.editor.row != 19 {
		t.Fatal("映射未就绪时应保留行号定位请求")
	}
	content, mapping, err := renderMappedMarkdown(m.editor.text(), m.previewWidth, "dark")
	if err != nil {
		t.Fatal(err)
	}
	m.Update(renderedMsg{revision: m.revision, source: m.editor.text(), content: content, mapping: mapping})
	if m.pendingLineJump != 0 || m.editor.row != 19 {
		t.Fatal("映射完成后应保留目标源码行")
	}
	row := mappedLine(mapping, 19, false)
	if row < m.preview.YOffset() || row >= m.preview.YOffset()+m.bodyHeight {
		t.Fatal("目标源码行应在预览可见范围内")
	}
	before := m.editor.text()
	vimKeys(m, "3j")
	if m.editor.text() != before {
		t.Fatal("预览定位不能修改原文")
	}
}
