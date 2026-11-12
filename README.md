# mdcli

Go 编写的终端 Markdown 编辑器。默认左侧原文、右侧实时预览，支持代码块高亮与 LaTeX 文本公式。无需浏览器、Node.js 或本地 TeX 安装。

## 运行

构建需要 **Go 1.25.8 或更新版本**。

```sh
go build -o bin/mdcli .
./bin/mdcli examples/demo.md
./bin/mdcli notes.md
```

文件不存在时打开空白缓冲区，第一次保存才创建文件；父目录必须已存在。不指定路径时打开当前目录的 `untitled.md`。

Windows：

```powershell
go build -o bin/mdcli.exe .
.\bin\mdcli.exe examples\demo.md
```

## 功能

- UTF-8 Markdown 编辑：中文、多行粘贴、制表符、行号、长文档滚动、撤销和重做。
- 实时预览：标题、列表、任务列表、引用、表格、链接、行内代码与代码块语法高亮。
- 双列 / 仅编辑 / 仅预览；宽度小于 70 列时双列布局自动显示单个面板，通过 F6 切换。
- 编辑和预览独立滚动，编辑时可用 Alt+↑/↓ 滚动预览。
- `$...$` 行内公式和 `$$...$$` 块公式，以 Unicode 字符排版。
- 保留制表符、末尾换行、UTF-8 BOM、统一的 LF 或 CRLF 格式。
- 保存采用同目录临时文件再重命名；保存前检查外部修改，避免覆盖已检测到的更改。
- 退出时提示未保存内容。平时不自动写入文件。

源码编辑区以原文显示；语法高亮位于预览中的代码块。长源码行横向滚动，预览正文按宽度折行。

## 快捷键

| 快捷键 | 功能 |
| --- | --- |
| Ctrl+P / F5 | 双列 → 编辑 → 预览 |
| F6 | 双列模式下切换焦点；窄屏切换可见面板 |
| Ctrl+S / F2 | 保存 |
| Ctrl+Q / Ctrl+C | 退出；未保存时提示 |
| S / D / Esc | 退出提示中：保存并退出 / 放弃修改 / 返回 |
| Ctrl+Z / Ctrl+Y | 撤销 / 重做 |
| 方向键、Home / End | 移动光标；预览焦点下滚动 |
| Ctrl+Home / Ctrl+End | 移动到文档首 / 尾 |
| PgUp / PgDn | 当前面板翻页 |
| Alt+↑ / Alt+↓ | 独立滚动预览 |
| Alt+PgUp / Alt+PgDn | 预览翻页 |
| Tab | 插入制表符，显示为 4 列制表位 |
| Ctrl+K / Ctrl+U | 删除至行尾 / 行首 |
| F1 | 帮助 |

粘贴使用终端自身的快捷键，例如 macOS 的 Cmd+V，或终端提供的 Ctrl+Shift+V。若 Ctrl+S 被终端流控拦截，可使用 F2。暂不提供鼠标编辑、文本选择、查找替换及 Vim 模式。

## 命令行参数

选项放在文件名前：

```sh
mdcli --mode preview notes.md
mdcli --mode edit notes.md
mdcli --theme light notes.md
mdcli --print --width 100 notes.md
mdcli --help
```

`--theme` 可选 `dark`、`light`、`ascii`，控制 Markdown 预览样式。`--print` 输出到文件或管道时移除 ANSI 颜色。参数省略时使用双列布局、dark 主题。

## 公式支持范围

使用 [doug/termtex](https://github.com/doug/termtex) 渲染数学表达式，常用分数、上下标、希腊字母、求和、积分、矩阵等采用字符网格排版。公式始终以原始 LaTeX 保存到 Markdown 文件。

这不是完整 TeX 引擎：不执行宏包、自定义宏或任意 LaTeX 文档，不保证与 KaTeX / MathJax 的排版一致。未完成、解析失败的公式不会阻止编辑。复杂公式的准确显示取决于 termtex 支持范围和终端字体；本版不使用 Kitty / Sixel 图片协议。

示例：

```markdown
行内：$E = mc^2$

$$
x = \frac{-b \pm \sqrt{b^2 - 4ac}}{2a}
$$
```

## 当前边界

- 单文件编辑，UTF-8 文本最大 2 MiB。混用 LF / CRLF、无效 UTF-8 或不支持的控制字符会在打开时报错，不会默默转换。
- 撤销历史最多 100 次变更、约 8 MiB 文本快照；多行粘贴算一次变更。
- 两侧独立滚动，尚无精确的源码与预览行映射。
- 终端预览不执行 HTML、JavaScript，不加载远程图片；图片无法像浏览器一样显示。
- 外部修改检测不是跨进程文件锁。检测到冲突时保存失败并保留内存中的编辑内容。
- 支持普通中文字符；组合字符和复杂 Emoji 的光标移动仍按 Unicode 码点处理。
- 终端至少 30 列 × 10 行，推荐 100 列 × 30 行；字体应支持中文和数学 Unicode 字符。

## 测试与跨平台构建

```sh
go test ./...
go test -race ./...
go vet ./...

CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o dist/mdcli-darwin-arm64 .
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/mdcli-linux-amd64 .
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o dist/mdcli-windows-amd64.exe .
```

跨平台编译成功不代表已在对应操作系统和每种终端中完成交互验证。

2026-11-12 本地验证：`go test ./...`、`go test -race ./...`、`go vet ./...` 通过；上述三平台构建通过。macOS arm64 的真实 tmux 终端会话已验证布局切换、预览翻页、中文输入、多行粘贴、撤销/重做、保存、窄屏焦点切换和未保存退出提示。Linux、Windows 尚未进行原生交互实测；仓库中的 GitHub Actions 配置尚未运行。

## 参考项目

| 项目 | 借鉴 / 使用方式 |
| --- | --- |
| [sho-luv/mdedit](https://github.com/sho-luv/mdedit) | Rust 终端编辑器，参考双列交互、三种视图和快捷键；其 README 未明确列出 LaTeX 支持 |
| [Splitmark/splitmark](https://github.com/Splitmark/splitmark) | 参考终端 Markdown 分屏编辑产品形态 |
| [charmbracelet/glow](https://github.com/charmbracelet/glow) | Go Markdown 阅读器，参考终端预览体验 |
| [charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea) | 直接使用终端事件循环和跨平台输入支持 |
| [charmbracelet/bubbles](https://github.com/charmbracelet/bubbles) | 直接使用预览滚动组件 |
| [charmbracelet/glamour](https://github.com/charmbracelet/glamour) | 直接使用 Markdown ANSI 渲染和代码高亮 |
| [doug/termtex](https://github.com/doug/termtex) | 直接使用 LaTeX 到 Unicode 的数学排版 |
| [junosuarez/termtex](https://github.com/junosuarez/termtex) | 后续图片公式方向的参考；依赖 Kitty Graphics Protocol 才能显示图形效果 |

编辑和文件保存逻辑在本项目中实现；没有复制上述完整编辑器的代码。依赖版本锁定在 `go.mod` 和 `go.sum`。
