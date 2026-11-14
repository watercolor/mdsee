package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestMarkdownCodeAndMath(t *testing.T) {
	source := "# 标题\n\n**正文** $E=mc^2$\n\n$$\n\\frac{1}{2}\n$$\n\n```go\nfmt.Println(\"$x^2$\")\n```\n\n`$a^2$`\n\n| 项目 | 值 |\n| --- | --- |\n| A | B |\n"
	got, err := renderMarkdown(source, 60, "dark")
	if err != nil {
		t.Fatal(err)
	}
	plain := ansi.Strip(got)
	for _, want := range []string{"标题", "正文", "mc²", "─", "fmt.Println", "$x^2$", "$a^2$", "项目"} {
		if !strings.Contains(plain, want) {
			t.Errorf("missing %q in %s", want, plain)
		}
	}
	if strings.Contains(plain, `\frac`) {
		t.Fatal("分数没有渲染")
	}
	if !strings.Contains(got, "\x1b[") {
		t.Fatal("代码和 Markdown 应有 ANSI 样式")
	}
}

func TestIncompleteMathDoesNotBreakPreview(t *testing.T) {
	for _, source := range []string{"writing $", "$$\n\\frac{", "$$\\unknowncommand{a}$$", "```go\n$x^2$\n", "中文 **未完成"} {
		if _, err := renderMarkdown(source, 40, "dark"); err != nil {
			t.Fatalf("%q: %v", source, err)
		}
	}
}

func TestMathDoesNotModifyIndentedOrNestedCode(t *testing.T) {
	for _, source := range []string{"    $x^2$\n", "> ```tex\n> $x^2$\n> ```\n", "- code:\n\n      $x^2$\n"} {
		got := expandMath(source, 60)
		if got != source {
			t.Fatalf("code changed: %q => %q", source, got)
		}
	}
	got, err := renderMarkdown("```go\n\treturn 1\n```", 60, "dark")
	if err != nil || strings.Contains(got, "\t") {
		t.Fatalf("preview contains terminal tabs: %q, %v", got, err)
	}
}
