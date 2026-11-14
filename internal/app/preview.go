package app

import (
	"fmt"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"github.com/doug/termtex"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func renderMarkdown(source string, width int, theme string) (string, error) {
	renderer, err := newMarkdownRenderer(width, theme)
	if err != nil {
		return "", err
	}
	return renderWithRenderer(renderer, source, width)
}

type markdownRenderer interface {
	Render(string) (string, error)
}

func newMarkdownRenderer(width int, theme string, images ...*imageRenderer) (markdownRenderer, error) {
	style, err := markdownStyles(theme)
	if err != nil {
		return nil, err
	}
	if len(images) > 0 && images[0] != nil && images[0].mode != "off" {
		return newImageMarkdownRenderer(width, style, images[0]), nil
	}
	return glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(max(10, width)),
	)
}

func renderWithRenderer(renderer markdownRenderer, source string, width int) (string, error) {
	// Expand tabs only in the preview; terminal tab stops depend on pane position.
	lines := strings.Split(source, "\n")
	for i := range lines {
		lines[i] = expandTabs(lines[i])
	}
	source = strings.Join(lines, "\n")
	if _, imageMode := renderer.(*imageMarkdownRenderer); !imageMode {
		source = expandMermaid(source, width)
	}
	expanded := expandMath(source, max(10, width-4))
	result, err := renderer.Render(expanded)
	return strings.Trim(result, "\n"), err
}

func markdownStyles(theme string) (ansi.StyleConfig, error) {
	style := styles.DarkStyleConfig
	colors := [6]string{"#f0fdfa", "#5eead4", "#93c5fd", "#c4b5fd", "#a5b4fc", "#cbd5e1"}
	background := "#115e59"
	switch theme {
	case "dark":
	case "light":
		style = styles.LightStyleConfig
		colors = [6]string{"#ffffff", "#0f766e", "#1d4ed8", "#6d28d9", "#4338ca", "#475569"}
		background = "#0f766e"
	case "ascii":
		return styles.ASCIIStyleConfig, nil
	default:
		return ansi.StyleConfig{}, fmt.Errorf("未知预览主题：%s", theme)
	}
	bold, underline := true, true
	headings := []*ansi.StyleBlock{&style.H1, &style.H2, &style.H3, &style.H4, &style.H5, &style.H6}
	for i, heading := range headings {
		*heading = ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{
			Bold:  &bold,
			Color: &colors[i],
		}}
	}
	style.H1.Prefix, style.H1.Suffix = " ", " "
	style.H1.BackgroundColor = &background
	style.H2.Underline = &underline
	return style, nil
}

func expandMath(source string, width int) string {
	// Protect code identified by CommonMark, including indented and nested blocks.
	marker := "MDCLILITERALDOLLAR"
	for strings.Contains(source, marker) {
		marker += "X"
	}
	raw := []byte(source)
	root := goldmark.DefaultParser().Parse(text.NewReader(raw))
	type span struct{ start, end int }
	var spans []span
	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if node.Kind() == ast.KindCodeBlock || node.Kind() == ast.KindFencedCodeBlock {
			for i := 0; i < node.Lines().Len(); i++ {
				line := node.Lines().At(i)
				spans = append(spans, span{line.Start, line.Stop})
			}
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	var protected strings.Builder
	pos := 0
	for _, s := range spans {
		protected.WriteString(source[pos:s.start])
		protected.WriteString(strings.ReplaceAll(source[s.start:s.end], "$", marker))
		pos = s.end
	}
	protected.WriteString(source[pos:])
	return strings.ReplaceAll(termtex.Expand(protected.String(), termtex.Style{Width: width}), marker, "$")
}
