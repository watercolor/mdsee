package main

import (
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func vimKeys(m *model, keys string) {
	for _, r := range keys {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func TestVimToggleInsertAndNormalEditing(t *testing.T) {
	m := testModel(t)
	m.Update(tea.PasteMsg{Content: "one two\n中文行\nlast"})
	m.editor.row, m.editor.col = 0, 0
	m.Update(press(tea.KeyF4))
	if !m.vim.enabled || m.vim.insert {
		t.Fatal("F4 应进入 Vim Normal")
	}
	if c := m.View().Cursor; c == nil || c.Shape != tea.CursorBlock || c.Blink {
		t.Fatal("Normal 应显示稳定块状光标")
	}
	vimKeys(m, "w")
	if m.editor.col != 4 {
		t.Fatal("w 未移动到下一个词", m.editor.col)
	}
	vimKeys(m, "b")
	if m.editor.col != 0 {
		t.Fatal("b 未返回词首")
	}
	vimKeys(m, "i")
	if c := m.View().Cursor; c == nil || c.Shape != tea.CursorBar || !c.Blink {
		t.Fatal("Insert 应显示闪烁竖线光标")
	}
	m.Update(tea.PasteMsg{Content: "你好 "})
	m.Update(press('c', tea.ModCtrl))
	if m.vim.insert || m.confirmQuit || m.editor.text() != "你好 one two\n中文行\nlast" {
		t.Fatal("Ctrl+C 应返回 Normal 且保留输入")
	}
	vimKeys(m, "0x")
	if !strings.HasPrefix(m.editor.text(), "好 one") {
		t.Fatal(m.editor.text())
	}
	vimKeys(m, "u")
	if !strings.HasPrefix(m.editor.text(), "你好 one") {
		t.Fatal("u 未撤销删除")
	}
	m.Update(press('r', tea.ModCtrl))
	if !strings.HasPrefix(m.editor.text(), "好 one") {
		t.Fatal("Ctrl+R 未重做")
	}
	m.Update(press(tea.KeyF4))
	vimKeys(m, "Z")
	if m.vim.enabled || !strings.HasPrefix(m.editor.text(), "Z好 one") {
		t.Fatal("切回普通模式后应直接输入")
	}
}

func TestVimLinesRegisterAndEmptyDocument(t *testing.T) {
	m := testModel(t)
	m.vim.enabled = true
	m.editor = newEditor("alpha\n中文\nomega")
	vimKeys(m, "yyp")
	if m.editor.text() != "alpha\nalpha\n中文\nomega" {
		t.Fatal(m.editor.text())
	}
	vimKeys(m, "dd")
	if m.editor.text() != "alpha\n中文\nomega" {
		t.Fatal(m.editor.text())
	}
	vimKeys(m, "G0x")
	if m.editor.text() != "alpha\n中文\nmega" {
		t.Fatal(m.editor.text())
	}
	vimKeys(m, "ggO")
	m.Update(tea.PasteMsg{Content: "new"})
	m.Update(press(tea.KeyEscape))
	if m.editor.text() != "new\nalpha\n中文\nmega" {
		t.Fatal(m.editor.text())
	}
	m.editor = newEditor("")
	m.vim.reset()
	vimKeys(m, "ddxGGggbw")
	if len(m.editor.lines) < 1 || m.editor.row != 0 || m.editor.col != 0 {
		t.Fatal("空文档光标越界")
	}
	vimKeys(m, "p")
	if len(m.editor.lines) != 2 || m.editor.row != 1 {
		t.Fatal("空行寄存器应粘贴一个空行")
	}
}

func TestVimPreviewNeverEditsEvenFromInsert(t *testing.T) {
	m := testModel(t)
	m.vim.enabled = true
	m.editor = newEditor("source")
	m.preview.SetContent(strings.Repeat("preview\n", 100))
	vimKeys(m, "i")
	m.Update(press('w', tea.ModCtrl))
	vimKeys(m, "jj")
	m.Update(tea.PasteMsg{Content: "do not insert"})
	if m.editor.text() != "source" || m.preview.YOffset() != 2 {
		t.Fatal("预览焦点下不应修改原文")
	}
	vimKeys(m, "Ggg")
	if m.preview.YOffset() != 0 {
		t.Fatal("预览 gg 应回顶部")
	}
}

func TestVimCommandsProtectUnsavedChanges(t *testing.T) {
	m := testModel(t)
	m.vim.enabled = true
	m.editor = newEditor("draft")
	vimKeys(m, ":q")
	_, cmd := m.Update(press(tea.KeyEnter))
	if cmd != nil || !m.dirty() || !strings.Contains(m.status, "未保存") {
		t.Fatal(":q 不应丢失修改")
	}
	vimKeys(m, ":w")
	m.Update(press(tea.KeyEnter))
	raw, err := os.ReadFile(m.doc.path)
	if err != nil || string(raw) != "draft" || m.dirty() {
		t.Fatal(":w 保存失败", err)
	}
	vimKeys(m, ":q")
	_, cmd = m.Update(press(tea.KeyEnter))
	if cmd == nil {
		t.Fatal("已保存应能 :q 退出")
	}
	vimKeys(m, ":q!")
	_, cmd = m.Update(press(tea.KeyEnter))
	if cmd == nil {
		t.Fatal(":q! 应退出")
	}
}
