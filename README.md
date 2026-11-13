# mdcli

Go 编写的终端 Markdown 编辑器。默认以 Vim Normal 模式打开仅预览界面，支持代码块高亮与 LaTeX 文本公式。按 Ctrl+P / F5 切换到双列或编辑布局。无需浏览器、Node.js 或本地 TeX 安装。

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
- 标题层级：dark / light 主题下一级标题加粗并带底色，二级标题加粗并加下划线，三级至六级加粗并用颜色区分；预览隐藏 `#` 标记。ascii 主题保留纯文本标题标记。
- 双列 / 仅编辑 / 仅预览；宽度小于 70 列时双列布局自动显示单个面板，通过 Ctrl+W / F6 切换。
- 编辑与预览默认双向关联滚动：编辑区带动预览，滚动预览也带动原文；F7 可关闭联动，分别浏览。
- 默认启用 Vim 核心编辑模式：F4 即时切换，`--vim=false` 可在启动时关闭。
- 单行底部状态栏：左侧快捷键提示或操作消息，右侧模式、联动状态与光标位置；窄屏自动精简提示。
- `$...$` 行内公式和 `$$...$$` 块公式，以 Unicode 字符排版。
- 保留制表符、末尾换行、UTF-8 BOM、统一的 LF 或 CRLF 格式。
- 保存采用同目录临时文件再重命名；保存前检查外部修改，避免覆盖已检测到的更改。
- 退出时提示未保存内容。平时不自动写入文件。

源码编辑区以原文显示；语法高亮位于预览中的代码块。长源码行横向滚动，预览正文按宽度折行。

标题样式示例：`./bin/mdcli examples/headings.md`。当前标题与正文使用相同字号；局部放大字体需要终端专用协议（例如 [Kitty Text Sizing](https://sw.kovidgoyal.net/kitty/text-sizing-protocol/)），本版未接入。粗体的具体字重取决于终端和字体设置。

如果终端设置了 `NO_COLOR=1`，颜色和底色会被关闭。macOS / Linux 可用 `env -u NO_COLOR ./bin/mdcli examples/headings.md` 临时启用颜色；这不会修改系统配置。

## 快捷键

| 快捷键 | 功能 |
| --- | --- |
| Ctrl+P / F5 | 双列 → 编辑 → 预览 |
| Ctrl+W / F6 | 双列模式下切换焦点；窄屏切换可见面板 |
| F4 | 普通编辑 / Vim 模式切换 |
| F7 | 开关双向关联滚动 |
| Ctrl+S / F2 | 保存 |
| Ctrl+Q | 退出；未保存时提示 |
| Ctrl+C | 普通模式退出；Vim 模式返回 Normal |
| S / D / Esc | 退出提示中：保存并退出 / 放弃修改 / 返回 |
| Ctrl+Z / Ctrl+Y | 撤销 / 重做 |
| 方向键、Home / End | 移动光标；预览焦点下滚动 |
| Ctrl+Home / Ctrl+End | 移动到文档首 / 尾 |
| PgUp / PgDn | 当前面板翻页 |
| Alt+↑ / Alt+↓ | 滚动预览；联动开启时原文跟随 |
| Alt+PgUp / Alt+PgDn | 预览翻页 |
| Tab | 插入制表符，显示为 4 列制表位 |
| Ctrl+K / Ctrl+U | 删除至行尾 / 行首 |
| F1 | 打开 / 关闭分组快捷键指南 |

粘贴使用终端自身的快捷键，例如 macOS 的 Cmd+V，或终端提供的 Ctrl+Shift+V。若 Ctrl+S 被终端流控拦截，可使用 F2。暂不提供鼠标编辑、文本选择及查找替换。

F1 帮助按编辑、窗口、浏览、保存分组，宽屏双列、窄屏单列。用方向键、j/k、PgUp/PgDn 滚动，Home/End 跳到顶部或底部，F1/Esc 返回编辑；查看帮助不会修改文档。

## Vim 模式

```sh
./bin/mdcli --vim examples/demo.md
```

`F4` 在普通编辑和 Vim 模式之间切换；启用时进入 **Normal**。状态栏显示 `VIM NORMAL` / `VIM INSERT`，按 `i` 输入，`Esc` 或 `Ctrl+C` 返回 Normal。预览区只接受浏览操作，即使从 Insert 切入预览，粘贴也不会修改原文。

光标使用终端原生形状：Normal 为稳定块状，Insert 与普通编辑为闪烁竖线。预览区、帮助页和退出提示中隐藏编辑光标，关闭帮助或切回编辑区后恢复。实际形状和闪烁由终端对光标控制协议的支持决定。

| 按键 | Normal 模式行为 |
| --- | --- |
| h / j / k / l | 左 / 下 / 上 / 右 |
| w / b / e | 下一个词 / 前一个词 / 词尾 |
| 0 / ^ / $ | 行首 / 首个非空白字符 / 行尾 |
| gg / G | 文档首 / 尾 |
| 20G / 20gg / :20 后回车 | 跳至源码第 20 行；越界时停在最后一行 |
| H / M / L | 当前可见屏幕的顶部 / 中间 / 底部行 |
| 3H / 3L | 从当前屏幕顶部 / 底部数第 3 行 |
| 5j / 3w | 下移 5 行 / 前进 3 个词 |
| 10\| | 跳至第 10 显示列，制表符按 4 列制表位计算 |
| 50% | 定位到文档 50% 处，支持 1%–100% |
| { / } | 上一个 / 下一个空行段落边界 |
| zz / zt / zb | 将当前行放到屏幕中间 / 顶部 / 底部 |
| i / a | 在光标前 / 后进入 Insert |
| I / A | 在行首 / 行尾进入 Insert |
| o / O | 下方 / 上方新建行并进入 Insert |
| x / dd / yy | 删除字符 / 删除行 / 复制行 |
| p / P | 在后 / 前粘贴内部寄存器内容 |
| u / Ctrl+R | 撤销 / 重做 |
| Ctrl+D / Ctrl+U | 半页下 / 上；带次数时按指定行数滚动 |
| Ctrl+F / Ctrl+B | 整页下 / 上 |
| :w / :q / :wq | 保存 / 退出 / 保存并退出 |
| :q! | 放弃未保存修改并退出 |

`:q` 遇到未保存修改会拒绝退出并提示；`:wq` 保存失败时保持编辑器打开。`yy`、`dd`、`x` 与 `p/P` 使用应用内寄存器，不访问系统剪贴板。

预览区支持 `h/j/k/l`、`gg/G`、半页和整页滚动，以及移动命令的次数前缀。`20G`、`20gg`、`:20`、`50%` 在预览区也按**源码行号**定位对应内容；预览映射尚未就绪时会在渲染完成后定位。预览区的 `H/M/L` 将当前可见内容的顶部 / 中间 / 底部映射到原文光标位置。按显示列、词、段落移动与 `zz/zt/zb` 用于原文编辑区。

次数前缀用于移动定位，输入中的数字会显示在状态栏；`Esc`、切换焦点或模式会取消尚未完成的前缀。超长次数限制为十亿，移动仍受文档边界限制。不带次数的 `0` 仍为行首，Insert 中的数字作为普通文字输入。

`Ctrl+W` 保持一次按键切换左右焦点，不作为 Vim 窗口命令前缀。当前实现为 Vim 核心操作子集，不支持 Visual、带次数的编辑操作（如 `2dd`、`3x`）、组合操作符（如 `dw`）、字符查找、全文搜索、标记、宏、插件；撤销仍按文本变更记录，不合并整个 Insert 会话。

## 关联滚动

默认开启，状态栏显示“联动开”，按 `F7` 切换。使用 Markdown 内容块在实际渲染结果中的位置作为锚点，块内按局部行数插值；复杂块、引用链接及公式的内部位置属于近似定位，不保证源码行与预览行一一对应。

滚动预览时，原文视口移动到对应内容，尽量保留光标；光标超出新视口才调整到最近的可见行。切回编辑区保持当前位置。编辑和窗口宽度变化会异步重建映射，过期映射不会用于移动光标。

## 命令行参数

选项放在文件名前：

```sh
mdcli --mode preview notes.md
mdcli --mode edit notes.md
mdcli --mode split --vim=false notes.md
mdcli --vim notes.md
mdcli --theme light notes.md
mdcli --print --width 100 notes.md
mdcli --help
```

`--theme` 可选 `dark`、`light`、`ascii`，控制 Markdown 预览样式。`--print` 输出到文件或管道时移除 ANSI 颜色。参数省略时使用仅预览布局、Vim Normal 模式和 dark 主题。

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
- 关联滚动按内容块锚点和块内插值定位；F7 关闭后两侧独立滚动。
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

Vim 与关联滚动更新已通过同样的测试、竞态检查、静态检查和三平台构建；macOS 真实终端另外验证了 `--vim`、F4、Normal / Insert、Ctrl+C、yy/p/u、`:q` / `:wq`、双向滚动、切焦点保持视口以及 F7 开关联动。

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
