package app

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

type helpBinding struct{ key, action string }
type helpSection struct {
	title    string
	bindings []helpBinding
}

func (m *model) helpSections() ([]helpSection, []helpSection) {
	left := []helpSection{
		{"编辑文字", []helpBinding{
			{"方向键", "移动光标"}, {"Home / End", "行首 / 行尾"}, {"Ctrl+Home/End", "文档首 / 尾"},
			{"Tab", "插入制表符"}, {"Ctrl+K / U", "删除至行尾 / 行首"}, {"Ctrl+Z / Y", "撤销 / 重做"},
			{"终端粘贴键", "粘贴中文和多行文字"},
		}},
	}
	if m.vim.enabled {
		left = []helpSection{
			{"进入与退出 Insert", []helpBinding{
				{"i / a", "光标前 / 后插入"}, {"I / A", "行首 / 尾插入"}, {"o / O", "下方 / 上方新建行"}, {"Esc / Ctrl+C", "返回 Normal"},
			}},
			{"Normal · 移动与编辑", []helpBinding{
				{"h j k l", "左 / 下 / 上 / 右"}, {"5j / 3w", "下移5行 / 前进3个词"}, {"w / b / e", "后一个词 / 前一个词 / 词尾"},
				{"x / dd / yy", "删除字符 / 删除行 / 复制行"}, {"p / P", "在后 / 前粘贴"}, {"u / Ctrl+R", "撤销 / 重做"},
				{"Ctrl+D / U", "半页下 / 上"}, {"Ctrl+F / B", "整页下 / 上"},
			}},
			{"定位 · 行号与屏幕", []helpBinding{
				{"20G / 20gg", "跳至源码第20行"}, {":20 + Enter", "按行号定位"}, {"gg / G", "文档首 / 尾"},
				{"H / M / L", "屏幕顶部 / 中间 / 底部"}, {"3H / 3L", "屏幕顶部 / 底部第3行"},
				{"0 / ^ / $", "行首 / 首个非空白 / 行尾"}, {"10| / 50%", "第10显示列 / 文档50%"},
				{"{ / }", "上一个 / 下一个段落边界"}, {"zz / zt / zb", "当前行放到中间 / 顶部 / 底部"},
			}},
		}
	}
	save := []helpBinding{{"Ctrl+S / F2", "保存文件"}, {"q / Ctrl+Q", "预览焦点下退出 / 全局退出"}, {"S / D / Esc", "退出提示中保存 / 放弃 / 返回"}}
	if m.vim.enabled {
		save = append(save, helpBinding{":w / :wq", "保存 / 保存并退出"}, helpBinding{":q / :q!", "安全退出 / 放弃修改退出"})
	} else {
		save = append(save, helpBinding{"Ctrl+C", "退出，有修改时提示"})
	}
	right := []helpSection{
		{"搜索与文件", []helpBinding{
			{"/ · n / N", "搜索 · 下一处 / 上一处"}, {"F3 / Shift+F3", "下一处 / 上一处匹配"}, {"o / F8", "预览目录 / 编辑区目录"}, {"Alt+[ / ]", "上一章 / 下一章"}, {"F9", "发现当前目录及子目录文件"}, {"F10", "缓冲区列表"}, {"Tab / Shift+Tab", "预览或 Normal 中切换文件"},
		}},
		{"窗口与模式", []helpBinding{
			{"Ctrl+P / F5", "双列 → 编辑 → 预览"}, {"Ctrl+W / F6", "切换左右焦点"}, {"F4", "普通编辑 ↔ Vim 模式"}, {"F7", "开关双向关联滚动"},
		}},
		{"浏览与滚动", []helpBinding{
			{"↑ ↓ / j k", "预览上下滚动"}, {"Space / PgDn", "预览向下翻页"}, {"PgUp", "当前面板向上翻页"}, {"Alt+↑ / ↓", "编辑时滚动预览"}, {"Alt+PgUp/PgDn", "编辑时翻预览页"},
		}},
		{"保存与退出", save},
	}
	return left, right
}

func renderHelpSections(sections []helpSection, width int, theme string) string {
	p := paletteFor(theme)
	accent := lipgloss.Color(p.headings[1])
	title := lipgloss.NewStyle().Foreground(accent).Bold(true)
	keyWidth := min(18, width/2)
	keys := lipgloss.NewStyle().Bold(true).Width(keyWidth)
	description := lipgloss.NewStyle().Width(width - keyWidth - 1)
	var groups []string
	for _, section := range sections {
		lines := []string{title.Render(section.title), lipgloss.NewStyle().Foreground(lipgloss.Color(p.border)).Render(strings.Repeat("─", width))}
		for _, binding := range section.bindings {
			lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, keys.Render(binding.key), " ", description.Render(binding.action)))
		}
		groups = append(groups, strings.Join(lines, "\n"))
	}
	return strings.Join(groups, "\n\n")
}

func (m *model) resizeHelp() {
	width := max(1, min(104, m.width)-4)
	m.helpViewport.SetWidth(width)
	m.helpViewport.SetHeight(max(1, m.bodyHeight-1))
	if !m.help {
		return
	}
	left, right := m.helpSections()
	var content string
	if width >= 88 {
		column := (width - 3) / 2
		content = lipgloss.JoinHorizontal(lipgloss.Top, renderHelpSections(left, column, m.theme), "   ", renderHelpSections(right, width-column-3, m.theme))
	} else {
		content = renderHelpSections(append(left, right...), width, m.theme)
	}
	note := "粘贴使用终端快捷键。LaTeX：$...$ 行内，$$...$$ 块公式。"
	if m.vim.enabled {
		note = "Normal 使用块状光标，Insert 使用竖线光标。复制内容保存在内部寄存器。"
	}
	content += "\n\n" + lipgloss.Wrap(note, width, "")
	m.helpViewport.SetContent(content)
}

func (m *model) helpPanel() string {
	width := min(104, m.width)
	inner := max(1, width-4)
	p := paletteFor(m.theme)
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(p.headings[1])).Render(clipped("快捷键指南 · "+m.vim.label(), inner))
	nav := fmt.Sprintf("F1/Esc 关闭 · ↑↓ 滚动 · %.0f%%", 100*m.helpViewport.ScrollPercent())
	panel := lipgloss.NewStyle().Width(width).Height(m.bodyHeight+3).Padding(0, 1).Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color(p.border)).Render(title + "\n" + m.helpViewport.View() + "\n" + clipped(nav, inner))
	return lipgloss.PlaceHorizontal(m.width, lipgloss.Center, panel)
}
