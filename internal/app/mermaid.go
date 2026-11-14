package app

import (
	"fmt"
	"strings"

	"github.com/spacedock-dev/mermaidtext"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func renderMermaid(source string, width int) (string, error) {
	if len(source) > 64*1024 {
		return "", fmt.Errorf("图表超过 64 KiB")
	}
	art, ok := mermaidtext.Render(source, mermaidtext.Options{MaxWidth: max(10, width)})
	if !ok {
		return "", fmt.Errorf("图表为空")
	}
	output := strings.Join(art.PlainLines, "\n")
	if art.Fallback {
		output = "Mermaid 字符回退（语法暂不支持或布局超宽）\n" + output
	}
	return output, nil
}

// Replace fenced bodies using parser offsets so code examples and source buffers stay intact.
func expandMermaid(source string, width int) string {
	raw := []byte(source)
	root := goldmark.New().Parser().Parse(text.NewReader(raw))
	type replacement struct {
		start, end int
		value      string
	}
	var changes []replacement
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		code, ok := n.(*ast.FencedCodeBlock)
		if !entering || !ok || string(code.Language(raw)) != "mermaid" || code.Lines().Len() == 0 {
			return ast.WalkContinue, nil
		}
		var body strings.Builder
		for i := 0; i < code.Lines().Len(); i++ {
			segment := code.Lines().At(i)
			body.Write(segment.Value(raw))
		}
		diagram, err := renderMermaid(body.String(), max(10, width-8))
		if err != nil {
			diagram = "Mermaid 未渲染（" + err.Error() + "）\n" + body.String()
		}
		start := code.Lines().At(0).Start
		end := code.Lines().At(code.Lines().Len() - 1).Stop
		lineStart := strings.LastIndex(source[:start], "\n") + 1
		prefix := source[lineStart:start]
		value := strings.ReplaceAll(strings.TrimRight(diagram, "\n"), "\n", "\n"+prefix) + "\n"
		changes = append(changes, replacement{start, end, value})
		return ast.WalkContinue, nil
	})
	for i := len(changes) - 1; i >= 0; i-- {
		c := changes[i]
		source = source[:c.start] + c.value + source[c.end:]
	}
	return source
}
