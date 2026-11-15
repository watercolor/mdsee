package app

import (
	"bytes"
	"fmt"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

type codeBlock struct{ token, language, source string }

type codeBlockRenderer struct {
	palette themePalette
	style   *chroma.Style
}

func newCodeBlockRenderer(theme string) *codeBlockRenderer {
	p := paletteFor(theme)
	style := chroma.MustNewStyle("mdcli-"+theme, chroma.StyleEntries{
		chroma.Text: p.codeText, chroma.Background: "bg:" + p.codeBackground,
		chroma.Comment: p.codeComment, chroma.Keyword: p.codeKeyword,
		chroma.LiteralString: p.codeString, chroma.LiteralNumber: p.codeNumber,
		chroma.NameTag: p.codeString, chroma.NameAttribute: p.codeString,
		chroma.NameFunction: p.codeFunction, chroma.NameClass: p.codeFunction,
		chroma.Operator: p.codeKeyword, chroma.Punctuation: p.codeText,
	})
	return &codeBlockRenderer{palette: p, style: style}
}

// Placeholder paragraphs keep Glamour's surrounding list/quote layout while
// code is framed separately at the actual available width.
func replaceCodeNodes(raw []byte, root ast.Node) ([]byte, []codeBlock) {
	prefix := "MC"
	for bytes.Contains(raw, []byte(prefix)) {
		prefix += "C"
	}
	var nodes []ast.Node
	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && (node.Kind() == ast.KindFencedCodeBlock || node.Kind() == ast.KindCodeBlock) {
			nodes = append(nodes, node)
		}
		return ast.WalkContinue, nil
	})
	var blocks []codeBlock
	for _, node := range nodes {
		var source strings.Builder
		for i := 0; i < node.Lines().Len(); i++ {
			segment := node.Lines().At(i)
			source.Write(segment.Value(raw))
		}
		language := "text"
		if code, ok := node.(*ast.FencedCodeBlock); ok && len(code.Language(raw)) > 0 {
			language = string(code.Language(raw))
		}
		token := fmt.Sprintf("%s%04dZ", prefix, len(blocks))
		blocks = append(blocks, codeBlock{token, language, source.String()})
		start := len(raw)
		raw = append(raw, token...)
		paragraph := ast.NewParagraph()
		paragraph.AppendChild(paragraph, ast.NewTextSegment(text.NewSegment(start, len(raw))))
		parent := node.Parent()
		parent.ReplaceChild(parent, node, paragraph)
	}
	return raw, blocks
}

func (r *codeBlockRenderer) replaceLine(line string, blocks []codeBlock, width int) ([]string, bool) {
	for _, block := range blocks {
		index := strings.Index(line, block.token)
		if index < 0 {
			continue
		}
		prefix := line[:index] + "\x1b[0m"
		continuation := strings.Map(func(c rune) rune {
			if c == '│' || c == '┃' || c == '>' {
				return c
			}
			return ' '
		}, ansi.Strip(prefix))
		rows := r.render(block, max(5, width-ansi.StringWidth(prefix)-2))
		for i := range rows {
			indent := continuation
			if i == 0 {
				indent = prefix
			}
			rows[i] = indent + rows[i] + "\x1b[0m"
		}
		return rows, true
	}
	return nil, false
}

func (r *codeBlockRenderer) render(block codeBlock, available int) []string {
	source := strings.TrimSuffix(strings.ReplaceAll(block.source, "\r\n", "\n"), "\n")
	source = ansi.Strip(source)
	language := strings.Map(func(c rune) rune {
		if unicode.IsControl(c) {
			return -1
		}
		return c
	}, block.language)
	contentWidth := ansi.StringWidth(language) + 1
	for _, line := range strings.Split(source, "\n") {
		contentWidth = max(contentWidth, ansi.StringWidth(line))
	}
	contentWidth = max(1, min(contentWidth, available-4))
	lexer := lexers.Get(language)
	if lexer == nil {
		lexer = lexers.Fallback
	}
	highlighted := source
	if tokens, err := lexer.Tokenise(nil, source); err == nil {
		var output bytes.Buffer
		if err := formatters.TTY16m.Format(&output, r.style, tokens); err == nil {
			highlighted = output.String()
		}
	}
	p := r.palette
	codeStyle := ansi.Style{}.ForegroundColor(lipgloss.Color(p.codeText)).BackgroundColor(lipgloss.Color(p.codeBackground)).String()
	highlighted = strings.ReplaceAll(highlighted, "\x1b[0m", "\x1b[0m"+codeStyle)
	var wrapped bytes.Buffer
	writer := lipgloss.NewWrapWriter(&wrapped)
	_, _ = writer.Write([]byte(ansi.Hardwrap(highlighted, contentWidth, true)))
	_ = writer.Close()
	border := lipgloss.NewStyle().Foreground(lipgloss.Color(p.border)).Background(lipgloss.Color(p.codeBackground))
	body := lipgloss.NewStyle().Foreground(lipgloss.Color(p.codeText)).Background(lipgloss.Color(p.codeBackground))
	label := ansi.Truncate(language, max(0, contentWidth-1), "…")
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(p.codeLabel)).Background(lipgloss.Color(p.codeBackground))
	rows := []string{border.Render("╭─ ") + labelStyle.Render(label) + border.Render(" "+strings.Repeat("─", max(0, contentWidth-1-ansi.StringWidth(label)))+"╮")}
	for _, line := range strings.Split(wrapped.String(), "\n") {
		padding := strings.Repeat(" ", max(0, contentWidth-ansi.StringWidth(line)))
		rows = append(rows, border.Render("│")+body.Render(" "+line+codeStyle+padding+" ")+border.Render("│"))
	}
	rows = append(rows, border.Render("╰"+strings.Repeat("─", contentWidth+2)+"╯"))
	return rows
}
