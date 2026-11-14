package main

import (
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

type scrollAnchor struct{ source, preview int }

// Positions between block anchors are interpolated locally, not across the document.
func mappedLine(anchors []scrollAnchor, line int, reverse bool) int {
	if len(anchors) == 0 {
		return 0
	}
	x, y := func(a scrollAnchor) int { return a.source }, func(a scrollAnchor) int { return a.preview }
	if reverse {
		x, y = y, x
	}
	i := sort.Search(len(anchors), func(i int) bool { return x(anchors[i]) > line })
	if i == 0 {
		return y(anchors[0])
	}
	a := anchors[i-1]
	if i == len(anchors) {
		return y(a)
	}
	b := anchors[i]
	if x(b) <= x(a) {
		return y(a)
	}
	return y(a) + (line-x(a))*(y(b)-y(a))/(x(b)-x(a))
}

func sourceBlocks(source string) []int {
	starts := []int{0}
	for i, b := range []byte(source) {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	lineAt := func(offset int) int { return max(0, sort.SearchInts(starts, offset+1)-1) }
	md := goldmark.New(goldmark.WithExtensions(extension.GFM, extension.DefinitionList))
	raw := []byte(source)
	root := md.Parser().Parse(text.NewReader(raw))
	var blocks []int
	cursor := 0
	for node := root.FirstChild(); node != nil; node = node.NextSibling() {
		first := len(raw)
		_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if entering && n.Type() != ast.TypeInline {
				for i := 0; i < n.Lines().Len(); i++ {
					first = min(first, n.Lines().At(i).Start)
				}
			}
			return ast.WalkContinue, nil
		})
		if code, ok := node.(*ast.FencedCodeBlock); ok {
			if code.Info != nil {
				first = min(first, code.Info.Segment.Start)
			} else if first < len(raw) {
				first = starts[max(0, lineAt(first)-1)]
			}
		}
		line := cursor
		if first < len(raw) {
			line = lineAt(first)
		} else {
			for line < len(starts)-1 && strings.TrimSpace(source[starts[line]:starts[line+1]]) == "" {
				line++
			}
		}
		line = max(cursor, line)
		if len(blocks) == 0 || line > blocks[len(blocks)-1] {
			blocks = append(blocks, line)
		}
		cursor = min(len(starts)-1, line+1)
	}
	return blocks
}

func normalizedLine(line string) string { return strings.Join(strings.Fields(ansi.Strip(line)), " ") }

func renderMappedMarkdown(source string, width int, theme string, images ...*imageRenderer) (string, []scrollAnchor, error) {
	renderer, err := newMarkdownRenderer(width, theme, images...)
	if err != nil {
		return "", nil, err
	}
	content, err := renderWithRenderer(renderer, source, width)
	if err != nil {
		return "", nil, err
	}
	sourceLines := strings.Split(source, "\n")
	previewLines := strings.Split(content, "\n")
	plain := make([]string, len(previewLines))
	positions := make(map[string][]int)
	for i, line := range previewLines {
		plain[i] = normalizedLine(line)
		positions[plain[i]] = append(positions[plain[i]], i)
	}
	blocks := sourceBlocks(source)
	anchors := []scrollAnchor{{0, 0}}
	next := 0
	for i, start := range blocks {
		end := len(sourceLines)
		if i+1 < len(blocks) {
			end = blocks[i+1]
		}
		probe, err := renderWithRenderer(renderer, strings.Join(sourceLines[start:end], "\n"), width)
		if err != nil {
			continue
		}
		var wanted []string
		for _, line := range strings.Split(probe, "\n") {
			if s := normalizedLine(line); s != "" {
				wanted = append(wanted, s)
			}
			if len(wanted) == 2 {
				break
			}
		}
		if len(wanted) == 0 {
			continue
		}
		candidates := positions[wanted[0]]
		for _, row := range candidates[sort.SearchInts(candidates, next):] {
			// A second content row disambiguates repeated headings and list items.
			if len(wanted) > 1 {
				j := row + 1
				for j < len(plain) && plain[j] == "" {
					j++
				}
				if j == len(plain) || plain[j] != wanted[1] {
					continue
				}
			}
			if start > anchors[len(anchors)-1].source && row > anchors[len(anchors)-1].preview {
				anchors = append(anchors, scrollAnchor{start, row})
			}
			next = row + 1
			break
		}
	}
	last := scrollAnchor{len(sourceLines) - 1, len(previewLines) - 1}
	if last.source > anchors[len(anchors)-1].source && last.preview > anchors[len(anchors)-1].preview {
		anchors = append(anchors, last)
	}
	return content, anchors, nil
}

func (m *model) syncFromEditor() {
	if !m.linked {
		return
	}
	m.alignPreviewToEditor()
}

func (m *model) alignPreviewToEditor() {
	if len(m.mapping) == 0 || m.mapSource != m.editor.text() {
		return
	}
	m.preview.SetYOffset(mappedLine(m.mapping, m.editor.top, false))
	// Keep the active block visible even when the source cursor has not scrolled yet.
	row := mappedLine(m.mapping, m.editor.row, false)
	if row < m.preview.YOffset() {
		m.preview.SetYOffset(row)
	}
	if row >= m.preview.YOffset()+m.bodyHeight {
		m.preview.SetYOffset(row - m.bodyHeight + 1)
	}
}

func (m *model) syncFromPreview() {
	if !m.linked || len(m.mapping) == 0 || m.mapSource != m.editor.text() {
		return
	}
	e := &m.editor
	e.top = min(max(0, len(e.lines)-m.bodyHeight), mappedLine(m.mapping, m.preview.YOffset(), true))
	e.row = max(e.top, min(e.row, min(len(e.lines)-1, e.top+m.bodyHeight-1)))
	e.col = min(e.col, len([]rune(e.lines[e.row])))
	if m.vim.enabled && !m.vim.insert {
		e.normalCursor()
	}
	e.layout(m.editorWidth, m.bodyHeight)
}
