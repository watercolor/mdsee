package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestBlockMappingAcrossWrappedTextCodeAndMath(t *testing.T) {
	source := "# 开始\n\n" + strings.Repeat("这是很长的段落，", 40) + "\n\n## 代码\n\n```go\nline1\nline2\n```\n\n## 公式\n\n$$\n\\frac{1}{2}\n$$\n\n## 结尾\n\nend\n"
	content, mapping, err := renderMappedMarkdown(source, 40, "dark")
	if err != nil {
		t.Fatal(err)
	}
	rows := strings.Split(content, "\n")
	lines := strings.Split(source, "\n")
	for _, heading := range []string{"## 代码", "## 公式", "## 结尾"} {
		row := 0
		for lines[row] != heading {
			row++
		}
		mapped := mappedLine(mapping, row, false)
		if normalizedLine(rows[mapped]) != strings.TrimPrefix(heading, "## ") {
			t.Fatalf("%q 映射到 %d: %q; anchors %v", heading, mapped, rows[mapped], mapping)
		}
		if mappedLine(mapping, mapped, true) != row {
			t.Fatal("反向块映射不一致")
		}
	}
	for i := 1; i < len(mapping); i++ {
		if mapping[i].source <= mapping[i-1].source || mapping[i].preview <= mapping[i-1].preview {
			t.Fatal("映射不单调", mapping)
		}
	}
}

func TestLinkedPreviewScrollPreservesOrClampsCursor(t *testing.T) {
	m := testModel(t)
	var b strings.Builder
	for i := 0; i < 30; i++ {
		b.WriteString("## 章节\n\nparagraph content\n\n")
	}
	m.editor = newEditor(b.String())
	m.resize(100, 18)
	content, mapping, err := renderMappedMarkdown(m.editor.text(), m.previewWidth, "dark")
	if err != nil {
		t.Fatal(err)
	}
	m.Update(renderedMsg{revision: m.revision, source: m.editor.text(), content: content, mapping: mapping})
	m.editor.row = 5
	m.preview.ScrollDown(1)
	m.syncFromPreview()
	if m.editor.row != 5 {
		t.Fatal("可见光标应保留", m.editor.row)
	}
	m.preview.SetYOffset(50)
	m.syncFromPreview()
	if m.editor.top == 0 || m.editor.row < m.editor.top || m.editor.row >= m.editor.top+m.bodyHeight {
		t.Fatal("光标应进入新的可见范围")
	}
	before := m.editor.top
	m.Update(press('w', tea.ModCtrl))
	m.Update(press('w', tea.ModCtrl))
	if m.editor.top != before {
		t.Fatal("切焦点不应跳回旧位置")
	}
	m.Update(press(tea.KeyF7))
	m.preview.SetYOffset(0)
	m.syncFromPreview()
	if m.editor.top != before {
		t.Fatal("关闭联动后不应移动原文")
	}
	m.Update(press(tea.KeyF7))
	m.previewFocus = false
	m.editor.row = 0
	m.editor.top = 0
	m.syncFromEditor()
	if m.preview.YOffset() != 0 {
		t.Fatal("原文回顶部应带动预览")
	}
}

func TestStaleMappingDoesNotMoveCursor(t *testing.T) {
	m := testModel(t)
	m.editor = newEditor("new\ncontent")
	m.mapping = []scrollAnchor{{0, 0}, {20, 100}}
	m.mapSource = "old"
	m.preview.SetContent(strings.Repeat("line\n", 200))
	m.preview.SetYOffset(100)
	m.syncFromPreview()
	if m.editor.top != 0 || m.editor.row != 0 {
		t.Fatal("过期映射不能改变光标")
	}
}
