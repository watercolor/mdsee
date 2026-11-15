package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestCodeFramesAndThemeColors(t *testing.T) {
	source := "## 配置\n\n正文与 `theme`。\n\n```json\n{\n  \"theme\": \"dark\",\n  \"cache_mib\": 256\n}\n```\n\n## 后文\n"
	for _, theme := range []string{"dark", "light", "dark"} {
		content, mapping, err := renderMappedMarkdown(source, 50, theme)
		if err != nil {
			t.Fatal(err)
		}
		plain := ansi.Strip(content)
		for _, want := range []string{"╭─ json", "╰", "│", `"cache_mib"`, "后文"} {
			if !strings.Contains(plain, want) {
				t.Fatalf("%s missing %q: %s", theme, want, plain)
			}
		}
		color := "38;2;163;190;140"
		if theme == "light" {
			color = "38;2;24;54;145"
		}
		if !strings.Contains(content, color) {
			t.Fatalf("%s JSON string color missing", theme)
		}
		rows := strings.Split(content, "\n")
		for _, row := range rows {
			if ansi.StringWidth(row) > 50 {
				t.Fatalf("frame exceeds width: %q", row)
			}
		}
		if got := normalizedLine(rows[mappedLine(mapping, 11, false)]); got != "后文" {
			t.Fatal("code frame broke following heading mapping", got, mapping)
		}
	}
}

func TestCodeFramesWrapAndPreserveNestedCode(t *testing.T) {
	for _, source := range []string{
		"```text\n" + strings.Repeat("ABCDEFG", 12) + "\n```",
		"> ```go\n> fmt.Println(\"中文\")\n> ```",
		"- item\n\n  ```go\n  fmt.Println(42)\n  ```",
		"    indented code\n",
		"```unknown-language\nMC0000Z\n```",
		"```\n```",
	} {
		for _, theme := range []string{"dark", "light"} {
			content, err := renderMarkdown(source, 30, theme)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(ansi.Strip(content), "╭") || !strings.Contains(ansi.Strip(content), "╯") {
				t.Fatal("missing code frame", content)
			}
			for _, row := range strings.Split(content, "\n") {
				if ansi.StringWidth(row) > 30 {
					t.Fatalf("wrapped code exceeds width: %q", row)
				}
			}
		}
	}
	content, err := renderMarkdown("```go\nreturn 1\n```", 30, "ascii")
	if err != nil || strings.Contains(content, "╭") || strings.Contains(content, "\x1b[") {
		t.Fatal("ASCII output lost its plain format", content, err)
	}
}

func TestThemeAppliesToWholeView(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	for _, theme := range []string{"dark", "light"} {
		m := testModel(t)
		m.theme = theme
		m.resize(80, 24)
		view := m.View()
		content := view.Content
		background := "48;2;30;30;46"
		if theme == "light" {
			background = "48;2;239;241;245"
		}
		if !strings.Contains(content, background) {
			t.Fatal("full view background does not follow theme", theme)
		}
		if view.BackgroundColor == nil || view.ForegroundColor == nil {
			t.Fatal("unstyled cells do not have themed terminal defaults", theme)
		}
	}
}

func TestPlainModeKeepsTerminalDefaults(t *testing.T) {
	for _, theme := range []string{"dark", "ascii"} {
		t.Setenv("NO_COLOR", "")
		m := testModel(t)
		m.theme = theme
		m.resize(80, 24)
		if theme == "dark" {
			t.Setenv("NO_COLOR", "1")
		}
		view := m.View()
		if view.BackgroundColor != nil || view.ForegroundColor != nil {
			t.Fatal("plain mode changed terminal defaults", theme)
		}
	}
}

func TestWrappedCodeStringKeepsHighlight(t *testing.T) {
	r := newCodeBlockRenderer("dark")
	rows := r.render(codeBlock{language: "json", source: `{"key":"` + strings.Repeat("abcdefgh", 8) + `"}`}, 24)
	for _, row := range rows {
		if strings.Contains(ansi.Strip(row), "cdef") && !strings.Contains(row, "38;2;163;190;140") {
			t.Fatal("wrapping lost string highlighting", row)
		}
	}
}
