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
	if theme != "ascii" {
		imageState := newImageRenderer("off", "", theme, width, 0, .5)
		if len(images) > 0 && images[0] != nil {
			imageState = images[0]
		}
		return newImageMarkdownRenderer(width, style, imageState), nil
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
	if r, imageMode := renderer.(*imageMarkdownRenderer); !imageMode || r.images.mode == "off" {
		source = expandMermaid(source, width)
	}
	expanded := expandMath(source, max(10, width-4))
	result, err := renderer.Render(expanded)
	return strings.Trim(result, "\n"), err
}

func markdownStyles(theme string) (ansi.StyleConfig, error) {
	style := styles.DarkStyleConfig
	switch theme {
	case "dark":
	case "light":
		style = styles.LightStyleConfig
	case "ascii":
		return styles.ASCIIStyleConfig, nil
	default:
		return ansi.StyleConfig{}, fmt.Errorf("未知预览主题：%s", theme)
	}
	p := paletteFor(theme)
	style.Document.Color = &p.foreground
	style.Document.BackgroundColor = &p.background
	bold := true
	headings := []*ansi.StyleBlock{&style.H1, &style.H2, &style.H3, &style.H4, &style.H5, &style.H6}
	for i, heading := range headings {
		*heading = ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{
			Bold:  &bold,
			Color: &p.headings[i],
		}}
	}
	style.Code.Color, style.Code.BackgroundColor = &p.inlineForeground, &p.inlineBackground
	style.Link.Color, style.LinkText.Color = &p.headings[1], &p.headings[1]
	style.Image.Color, style.ImageText.Color = &p.headings[3], &p.muted
	style.HorizontalRule.Color = &p.border
	style.Table.Color = &p.foreground
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
