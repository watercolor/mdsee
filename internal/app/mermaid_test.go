package app

import (
	"github.com/charmbracelet/x/ansi"
	"strings"
	"testing"
)

func TestMermaidDiagramsAndFallback(t *testing.T) {
	for _, direction := range []string{"LR", "RL", "TD", "BT"} {
		source := "```mermaid\nflowchart " + direction + "\nA[开始] -->|成功| B{检查}\nB -.-> A\nC(完成)\n```\n\n后文"
		output, err := renderMarkdown(source, 80, "dark")
		if err != nil {
			t.Fatal(err)
		}
		plain := ansi.Strip(output)
		for _, want := range []string{"┌", "开始", "成功", "完成", "后文"} {
			if !strings.Contains(plain, want) {
				t.Fatalf("missing %s: %s", want, plain)
			}
		}
	}
	source := "```mermaid\npie\n\"A\" : 1\n```"
	output, err := renderMarkdown(source, 80, "dark")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "Mermaid 字符回退") || !strings.Contains(ansi.Strip(output), "pie") {
		t.Fatal(output)
	}
	code := "````md\n```mermaid\ngraph TD\nA --> B\n```\n````"
	if expandMermaid(code, 80) != code {
		t.Fatal("rewrote literal code example")
	}
}
