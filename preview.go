package main

import (
	"strings"

	"charm.land/glamour/v2"
	"github.com/doug/termtex"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func renderMarkdown(source string, width int, theme string) (string, error) {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(theme),
		glamour.WithWordWrap(max(10, width)),
	)
	if err != nil {
		return "", err
	}
	// Expand tabs only in the preview; terminal tab stops depend on pane position.
	lines := strings.Split(source, "\n")
	for i := range lines {
		lines[i] = expandTabs(lines[i])
	}
	source = strings.Join(lines, "\n")
	expanded := expandMath(source, max(10, width-4))
	result, err := renderer.Render(expanded)
	return strings.Trim(result, "\n"), err
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
