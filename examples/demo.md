# Markdown，留在终端里

欢迎使用 **mdcli**。左侧编辑原文，右侧查看实时预览。

按 `Ctrl+P` 或 `F5` 切换双列、编辑、预览；按 `Ctrl+W` 或 `F6` 切换焦点。预览区获得焦点后，用 `↑` / `↓` 滚动，`PgUp` / `PgDn` 翻页。

## 写代码

```go
package main

import "fmt"

func main() {
	fmt.Println("你好，Markdown！")
}
```

## 写公式

行内公式：$E = mc^2$，以及 $\alpha + \beta = \gamma$。

二次方程求根公式：

$$
x = \frac{-b \pm \sqrt{b^2 - 4ac}}{2a}
$$

求和：

$$
\sum_{i=1}^{n} i = \frac{n(n+1)}{2}
$$

矩阵：

$$
\begin{pmatrix} a & b \\ c & d \end{pmatrix}
$$

## 整理笔记

- [x] Markdown 编辑
- [x] 代码块高亮预览
- [x] LaTeX 文本公式预览
- [ ] 写下你的下一篇笔记

| 快捷键 | 功能 |
| --- | --- |
| Ctrl+S / F2 | 保存 |
| Ctrl+P / F5 | 切换布局 |
| Ctrl+W / F6 | 切换焦点 |
| Ctrl+Z / Ctrl+Y | 撤销 / 重做 |
| Ctrl+Q | 退出 |

> 文件只在保存时写入。退出前有未保存内容时，会提示保存或放弃。

支持标题、**粗体**、*斜体*、~~删除线~~、列表、引用、表格和[链接](https://github.com/charmbracelet/glamour)。

---

代码中的公式语法保持字面值：`$x^2$`。
