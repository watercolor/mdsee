package app

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

var mermaidNode = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)(?:\[([^\[\]]*)\]|\(([^()]*)\)|\{([^{}]*)\})?`)
var mermaidArrow = regexp.MustCompile(`^(-->|---|-.->|==>)(?:\|([^|]*)\|)?`)

type diagramEdge struct{ from, to, label, arrow string }

func renderMermaid(source string, width int) (string, error) {
	statements := strings.FieldsFunc(source, func(r rune) bool { return r == '\n' || r == ';' })
	labels := map[string]string{}
	var order []string
	var edges []diagramEdge
	direction := ""
	node := func(s string) (string, string, bool) {
		parts := mermaidNode.FindStringSubmatch(strings.TrimSpace(s))
		if parts == nil {
			return "", s, false
		}
		id := parts[1]
		label := id
		for _, p := range parts[2:] {
			if p != "" {
				label = strings.Trim(p, `"`)
			}
		}
		if _, exists := labels[id]; !exists {
			order = append(order, id)
			labels[id] = label
		} else if label != id {
			labels[id] = label
		}
		return id, strings.TrimSpace(strings.TrimSpace(s)[len(parts[0]):]), true
	}
	for _, statement := range statements {
		statement = strings.TrimSpace(statement)
		if statement == "" || strings.HasPrefix(statement, "%%") {
			continue
		}
		if direction == "" {
			parts := strings.Fields(statement)
			if len(parts) != 2 || (parts[0] != "flowchart" && parts[0] != "graph") {
				return "", fmt.Errorf("仅支持 flowchart 或 graph")
			}
			switch parts[1] {
			case "TD", "TB", "BT", "LR", "RL":
				direction = parts[1]
			default:
				return "", fmt.Errorf("不支持的图方向")
			}
			continue
		}
		from, rest, ok := node(statement)
		if !ok {
			return "", fmt.Errorf("不支持的节点语法")
		}
		for rest != "" {
			arrow := mermaidArrow.FindStringSubmatch(rest)
			if arrow == nil {
				return "", fmt.Errorf("不支持的连线或样式语法")
			}
			to, next, ok := node(rest[len(arrow[0]):])
			if !ok {
				return "", fmt.Errorf("连线缺少目标节点")
			}
			edges = append(edges, diagramEdge{from, to, arrow[2], arrow[1]})
			from, rest = to, next
			if len(edges) > 64 {
				return "", fmt.Errorf("连线超过 64 条")
			}
		}
		if len(order) > 64 {
			return "", fmt.Errorf("节点超过 64 个")
		}
	}
	if direction == "" || len(order) == 0 {
		return "", fmt.Errorf("图中没有节点")
	}
	box := func(id string) []string {
		label := clipped(labels[id]+" ("+id+")", max(4, min(32, width-6)))
		w := ansi.StringWidth(label)
		return []string{"┌" + strings.Repeat("─", w+2) + "┐", "│ " + label + " │", "└" + strings.Repeat("─", w+2) + "┘"}
	}
	var rows []string
	rows = append(rows, "Mermaid · "+direction)
	used := map[string]bool{}
	for _, edge := range edges {
		from, to := box(edge.from), box(edge.to)
		used[edge.from], used[edge.to] = true, true
		label := edge.label
		if edge.arrow == "-.->" {
			label = "虚线 " + label
		}
		if edge.arrow == "==>" {
			label = "粗线 " + label
		}
		if edge.arrow == "---" {
			label = "无向 " + label
		}
		horizontal := direction == "LR" || direction == "RL"
		connector := " ──▶ "
		if direction == "RL" {
			connector = " ◀── "
			from, to = to, from
		}
		if edge.arrow == "---" {
			connector = " ─── "
		}
		if horizontal && ansi.StringWidth(from[0])+ansi.StringWidth(to[0])+5 <= width {
			if label != "" {
				rows = append(rows, label)
			}
			for i := range from {
				mid := strings.Repeat(" ", 5)
				if i == 1 {
					mid = connector
				}
				rows = append(rows, from[i]+mid+to[i])
			}
		} else {
			if direction == "BT" {
				from, to = to, from
			}
			rows = append(rows, from...)
			arrow := "  ▼"
			if direction == "BT" {
				arrow = "  ▲"
			}
			if edge.arrow == "---" {
				arrow = "  │"
			}
			rows = append(rows, "  │ "+label, arrow)
			rows = append(rows, to...)
		}
		rows = append(rows, "")
	}
	for _, id := range order {
		if !used[id] {
			rows = append(rows, box(id)...)
			rows = append(rows, "")
		}
	}
	return strings.Join(rows, "\n"), nil
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
