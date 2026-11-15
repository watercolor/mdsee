# mdcli

Go 编写的终端 Markdown 编辑器。默认以 Vim Normal 模式打开仅预览界面，支持代码块高亮与 LaTeX 文本公式。按 Ctrl+P / F5 切换到双列或编辑布局。无需浏览器、Node.js 或本地 TeX 安装。

## 运行

构建需要 **Go 1.25.8 或更新版本**。

```sh
go build -o bin/mdcli ./cmd/mdcli
./bin/mdcli examples/demo.md
./bin/mdcli notes.md examples/demo.md
```

文件不存在时打开空白缓冲区，第一次保存才创建文件；父目录必须已存在。不指定路径时打开当前目录的 `untitled.md`。

Windows：

```powershell
go build -o bin/mdcli.exe ./cmd/mdcli
.\bin\mdcli.exe examples\demo.md
```

## 目录结构

```text
cmd/mdcli/main.go       命令入口
internal/app/          终端界面、编辑、导航、Markdown 和图片渲染
  app.go
  app_test.go          同包测试，其他模块同样使用 *_test.go
  cli.go
  ...
main.go                兼容根目录构建和运行的薄入口
docs/                  功能待办文档
examples/              示例 Markdown 和图片
```

Go 按包组织代码，`*_test.go` 与被测源码放在同一目录，执行 `go test` 时参与编译。当前应用代码集中在 `internal/app` 包。`cmd/mdcli` 为主要入口，根目录也支持 `go build .` 和 `go run .`。布局参考 [Go 官方项目组织指南](https://go.dev/doc/modules/layout)。

## 功能

- UTF-8 Markdown 编辑：中文、多行粘贴、制表符、行号、长文档滚动、撤销和重做。
- 实时预览：标题、列表、任务列表、引用、表格、链接、行内代码与代码块语法高亮。
- 图片预览支持本地文件与 HTTP/HTTPS 网络图片，格式包括 PNG、JPEG、GIF 首帧，可使用 Kitty 原图显示或终端彩色字符预览。
- 深浅配色参考 mdterm，统一界面背景、正文、标题、目录和帮助面板。标题加粗并按层级区分颜色，预览隐藏 `#` 标记。ascii 主题保留纯文本标题标记。
- 预览代码块带语言标签、圆角边框、独立底色和语法高亮；长行按可用宽度折行，支持引用和列表内的代码块。
- 双列 / 仅编辑 / 仅预览；宽度小于 70 列时双列布局自动显示单个面板，通过 Ctrl+W / F6 切换。
- 编辑与预览默认双向关联滚动：编辑区带动预览，滚动预览也带动原文；F7 可关闭联动，分别浏览。
- 默认启用 Vim 核心编辑模式：F4 即时切换，`--vim=false` 可在启动时关闭。
- 底部显示当前焦点下的常用操作或操作消息，预览提示包括目录、搜索、匹配跳转、翻页和退出；右侧显示模式、联动状态与光标位置，窄屏预览优先保留目录、搜索和退出。Mermaid 状态显示在标题栏右侧。
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
| q | 仅预览布局或双列预览焦点下退出；检查全部缓冲区的未保存修改 |
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

预览区获得焦点时，`o` 打开目录，`/` 搜索，`n` / `N` 跳转匹配，空格向下翻页，`q` 退出。搜索输入、目录和帮助等面板会接收各自的按键。底部精简提示省略 F4、F7 和 Ctrl+W，这些功能可在 F1 中查看。

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
mdcli --images blocks examples/images.md
mdcli --print --width 100 notes.md
mdcli --help
```

`--theme` 可选 `dark`、`light`、`ascii`，控制界面和 Markdown 预览样式。`--print` 输出到文件或管道时移除 ANSI 颜色。参数省略时使用仅预览布局、Vim Normal 模式和 dark 主题。

深色采用 mdterm 的深蓝灰背景、浅色正文、蓝紫色标题和暖色行内代码。浅色采用浅灰背景、深色正文和对应的蓝紫色标题，代码区使用稍深的灰色底。代码语法高亮由 Chroma 提供，深色参考 base16-ocean，浅色参考 GitHub。未标注语言的代码块显示 `text`，未知语言按文本显示。代码块边框和标签也用于 `--print`，ASCII 输出保持纯文本样式。

```sh
./bin/mdcli --theme dark examples/demo.md
./bin/mdcli --theme light examples/demo.md
```

主题会设置应用运行期间的终端默认前景和背景，退出时由 Bubble Tea 恢复终端默认颜色。`NO_COLOR` 和 ASCII 模式使用终端原有默认颜色。配色参考见 [mdterm 主题源码](https://github.com/bahdotsh/mdterm/blob/main/src/theme.rs)。

预览中按空格键或 PgDn 向下翻一页，PgUp 向上翻页。编辑区和搜索输入中的空格用于输入文字。

## 搜索、目录与多文件

- 预览与 Vim Normal 中按 `/` 在底部输入搜索，Enter 跳到下一处，`n` / `N` 循环查找下一处和上一处，也支持 `3n` 等次数。空输入按 Enter 重复上次搜索，Esc 取消本次输入。匹配高亮并显示计数，采用逐行、区分大小写的字面匹配。`F3` / `Shift+F3` 也可切换匹配。
- 预览中按 `o` 打开居中的目录浮层，显示标题缩进、H1–H6 层级、当前位置和选中高亮。`j/k` 或方向键选择，Enter 同时定位源码与预览，Esc 或 `o` 关闭。按 `/` 输入标题模糊过滤，Esc 返回列表选择，再按 Esc 关闭。`F8` 在编辑区也能打开目录，支持 ATX 和 Setext 标题。`Alt+[` / `Alt+]` 跳到上一章和下一章。
- 命令行可传入多个文件。预览与 Vim Normal 中用 `Tab` / `Shift+Tab` 切换前后文件，`F10` 打开缓冲区列表，`Ctrl+Tab` / `Ctrl+Shift+Tab` 也可切换。各文件保留光标、滚动位置、撤销历史、搜索和未保存内容。Vim Insert 与普通编辑中的 Tab 用于缩进，Vim Normal 中的 `o` 在当前行下方新建行。
- `F9` 扫描 Git 仓库根目录，仓库外扫描当前目录，递归发现 `.md` 和 `.markdown` 文件，跳过 `.git`、`node_modules` 和 `vendor` 目录。输入路径片段过滤，Enter 打开文件或切换到已有缓冲区。
- `Ctrl+S` 保存当前文件。退出时检查全部缓冲区，选择 S 保存全部后退出，D 放弃全部修改后退出。`:wq` 保存全部后退出，`:q` 检查全部文件，`:q!` 放弃全部修改退出。

文件选择面板使用 Esc 返回。Vim 中的 `Ctrl+F` / `Ctrl+B` 分别向下和向上翻整页。

## Mermaid 渲染

启动时检测 `PATH` 中的 `mmdc`。在图片预览开启时，先查找持久化缓存，未命中时通过本机 Mermaid CLI 异步生成 PNG，再通过现有 Kitty 或彩色字符图片管线显示。

- 生成期间先显示 `mermaidtext` 字符版本。标题栏右侧显示当前文档的完成进度、运行数和排队数，窄屏显示精简进度，底部保留操作提示。PNG 完成后自动刷新，失败时显示字符回退及缺少浏览器等原因。
- 未安装 `mmdc`、生成失败、20 秒超时或 PNG 无法读取时，使用 `mermaidtext` 字符回退。
- PNG 优先按 3 倍分辨率生成，超过单图大小或像素限制时依次尝试 2 倍、1 倍。Kitty 模式保留 Mermaid 原始像素，显示尺寸按生成倍率还原到原始排版大小，小图自然显示，宽图按面板宽度缩小，高图最多 256 行，可用空格或方向键滚动浏览。
- 持久化缓存以 Mermaid 源码、主题、mmdc 安装标识和出图参数版本计算 SHA-256。同一张图跨文件、跨次启动复用，命中后直接显示，标题栏提示“缓存命中 N”。源码的 CRLF 换行统一为 LF，保留缩进。调整窗口时复用 PNG，按当前面板尺寸缩放。
- 命中后校验 PNG 的 SHA-256 和尺寸头。Kitty 模式直接传输原始 PNG 压缩数据，省去逐图解码、重新压缩的开销；彩色字符模式按需解码采样。
- 会话内最多保留 32 个图，使用最久未访问的已完成图为新图腾出空间，最多同时执行两个 `mmdc`。多个 mdcli 进程请求同一缓存图时通过文件锁合并生成。失败结果只在会话内保留 30 秒，之后重绘时可重试。
- 临时目录保存生成任务和会话副本。退出时取消任务并清理这些临时文件，成功的磁盘缓存继续保留。macOS、Linux 和 Windows 会尝试结束渲染进程树。
- `--images off`、ASCII 主题、无颜色终端及 `--print` 使用字符渲染。

### Mermaid 缓存容量与清理

默认容量为 **256 MiB**，统计缓存中的 PNG 和 JSON 元数据大小。目录通过 Go 的 `os.UserCacheDir()` 获取，macOS 默认在 `~/Library/Caches/mdcli/mermaid/v1`，可通过 `--mermaid-cache-dir` 指定。

- 启动和成功写入新图后检查容量，超限时按最近使用时间淘汰旧图，直到回到上限以内。会话内持续使用的图片最多每分钟更新一次磁盘访问时间。
- 容量内的有效图片持续保留。源码、主题或渲染器变化后生成新缓存，旧图留待容量淘汰。mmdc 标识包含可执行文件路径、大小、修改时间，以及可读取的 npm 包元数据和 `PUPPETEER_EXECUTABLE_PATH`。更换字体或浏览器后可手动清理，重新生成。
- 损坏图片在读取时删除并重新生成。崩溃残留的 `.work-*` 写入目录超过一小时后，在启动或写入时清理。单张缓存大于容量上限时只保留会话副本，磁盘缓存不可用时继续会话渲染并在标题栏提示。
- 手动清理会删除当前缓存目录内的图片、元数据和残留写入目录。已打开的图片继续使用会话副本，清理前已排队或正在运行的任务完成后只供当前会话显示。用于进程协调的锁文件和清理标记保留，锁文件最多 256 个分片，容量统计以图片和元数据为准。

```sh
# 设置为 128 MiB，每次启动可指定
./bin/mdcli --mermaid-cache-mib 128 examples/mermaid.md

# 关闭持久化，继续使用会话缓存
./bin/mdcli --mermaid-cache-mib 0 examples/mermaid.md

# 清理默认缓存并退出
./bin/mdcli --clear-mermaid-cache

# 使用自定义缓存目录，清理时指定相同目录
./bin/mdcli --mermaid-cache-dir /tmp/mdcli-cache examples/mermaid.md
./bin/mdcli --mermaid-cache-dir /tmp/mdcli-cache --clear-mermaid-cache
```

### 渲染器与安装

字符引擎使用 `mermaidtext v0.1.1`，覆盖流程图、时序图、状态图、类图和 ER 图的部分语法。思维导图和甘特图需要 `mmdc` 图片路径，字符引擎会为这两类图保留带框的源码。布局超宽或语法不支持时也会保留源码。每个图的源码最多 64 KiB，生成图片沿用单图 10 MiB、800 万像素等限制。

安装和检查 Mermaid CLI 后重新启动 mdcli，即可使用图片路径。

```sh
npm install -g @mermaid-js/mermaid-cli
mmdc --version
./bin/mdcli examples/mermaid.md
```

`mmdc` 还需要可用的浏览器运行环境，安装与故障处理见 [Mermaid CLI](https://github.com/mermaid-js/mermaid-cli)。字符引擎的语法范围、宽度回退和 Apache-2.0 许可证见 [mermaidtext](https://github.com/spacedock-dev/mermaidtext)。

如果提示缺少 `chrome-headless-shell`，需要通过当前 mmdc 所使用的 Puppeteer 安装匹配版本的浏览器。安装完成后重新打开 mdcli，可清除本次会话的失败缓存。显示大小按原始排版尺寸计算，默认以图中文字接近终端正文大小为目标，调整窗口时会重新计算布局。


## 图片预览

图片需独占一个 Markdown 段落，前后用空行分隔：

```markdown
![图片说明](assets/photo.png)

![含空格的路径](<assets/my photo.jpg>)
```

相对路径以 Markdown 文件所在目录为基准，支持绝对路径、百分号编码路径、`file:///` 本地绝对路径及引用式图片。网络图片使用 HTTP 或 HTTPS 地址，查询参数会随请求发送，支持重定向。支持 PNG、JPEG 和 GIF 首帧。行内图片、表格中的图片和 HTML `<img>` 保留文字表现。SVG、WebP 和 GIF 动画尚未接入。

```markdown
![Go 标志](https://go.dev/blog/go-brand/Go-Logo/PNG/Go-Logo_Blue.png)
```

网络图片自动在后台下载，预览先展示“下载中”，完成后刷新图像和滚动映射。同一地址在当前会话中复用下载缓存，调整窗口或切换布局时重新缩放。最多同时下载 4 张，请求超时为 8 秒。缓存最多保存 32 个地址、32 MiB 图片数据，容量不足时在图片位置显示提示。下载失败的结果保留 30 秒，之后再次编辑文档或调整窗口时可重试。退出会取消尚未完成的下载。

`--images` 控制显示方式：

| 模式 | 行为 |
| --- | --- |
| `auto`（默认） | 查询 Kitty Unicode 占位符支持和终端版本；支持占位符或识别为 iTerm2 3.7.3 及以上正式版，且具备真彩色时显示原图，否则使用彩色半块字符预览 |
| `kitty` | 显式使用 Kitty Unicode 占位符，需要终端支持该特性及真彩色；适用于不支持能力查询的兼容终端 |
| `blocks` | 将图片缩小为彩色半块字符，适用于普通终端；细节精度低于原图 |
| `off` | 显示图片说明和路径，关闭本地读取与网络图片下载 |

tmux 内使用 `blocks`，包括显式指定 `kitty` 时；本版未接入 tmux 图片透传。无颜色能力的终端、`NO_COLOR` 非空、`--theme ascii` 或 `--print` 时使用文字形式。彩色字符在 256 色终端上由终端渲染库降低颜色精度。

iTerm2 的自动识别使用终端实际返回的名称和版本，不依赖 `TERM_PROGRAM` 环境变量。iTerm2 3.7.3 及以上正式版可直接运行 `./bin/mdcli examples/images.md`；未识别的版本仍可用 `--images kitty` 手动尝试。版本下限依据 [iTerm2 更新记录](https://iterm2.com/downloads.html)中对图片 ID 高位占位符的修复。

图片按可用宽度和终端字符比例缩放，普通图片最高 24 行、最宽 100 列，Kitty 模式的 Mermaid 图片最高 256 行。无法取得字符尺寸时按宽高比 1:2 估算。滚动和分屏裁剪跟随文本视口，切换到编辑或帮助页会隐藏图像。编辑文档或调整窗口时重新读取本地图片，网络图片复用缓存。每次预览最多加载 32 个不同图片地址，单图不超过 10 MiB、800 万像素，缺失、损坏或超限时在图片位置显示提示。

示例：`./bin/mdcli examples/images.md`。Kitty 实现依据其[图形协议](https://sw.kovidgoyal.net/kitty/graphics-protocol/#unicode-placeholders)，没有新增浏览器、Node.js 或外部图片工具依赖。

## 公式支持范围

使用 [doug/termtex](https://github.com/doug/termtex) 渲染数学表达式，常用分数、上下标、希腊字母、求和、积分、矩阵等采用字符网格排版。公式始终以原始 LaTeX 保存到 Markdown 文件。

这不是完整 TeX 引擎：不执行宏包、自定义宏或任意 LaTeX 文档，不保证与 KaTeX / MathJax 的排版一致。未完成、解析失败的公式不会阻止编辑。复杂公式的准确显示取决于 termtex 支持范围和终端字体；公式仍使用文本排版，不转换为图片。

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
- 图片显示精度取决于终端能力和 `--images` 模式，网络图片只在图片渲染开启时下载。
- 外部修改检测不是跨进程文件锁。检测到冲突时保存失败并保留内存中的编辑内容。
- 支持普通中文字符；组合字符和复杂 Emoji 的光标移动仍按 Unicode 码点处理。
- 终端至少 30 列 × 10 行，推荐 100 列 × 30 行；字体应支持中文和数学 Unicode 字符。

## 测试与跨平台构建

```sh
go test ./...
go test -race ./...
go vet ./...

CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o dist/mdcli-darwin-arm64 ./cmd/mdcli
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/mdcli-linux-amd64 ./cmd/mdcli
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o dist/mdcli-windows-amd64.exe ./cmd/mdcli
```

跨平台编译成功不代表已在对应操作系统和每种终端中完成交互验证。

2026-11-12 本地验证：`go test ./...`、`go test -race ./...`、`go vet ./...` 通过；上述三平台构建通过。macOS arm64 的真实 tmux 终端会话已验证布局切换、预览翻页、中文输入、多行粘贴、撤销/重做、保存、窄屏焦点切换和未保存退出提示。Linux、Windows 尚未进行原生交互实测；仓库中的 GitHub Actions 配置尚未运行。

Vim 与关联滚动更新已通过同样的测试、竞态检查、静态检查和三平台构建；macOS 真实终端另外验证了 `--vim`、F4、Normal / Insert、Ctrl+C、yy/p/u、`:q` / `:wq`、双向滚动、切焦点保持视口以及 F7 开关联动。

本地图片更新已通过自动测试、竞态检查、静态检查和 macOS arm64 / Linux amd64 / Windows amd64 构建。macOS 的真实 tmux 会话验证了默认图片预览、双列布局、编辑与帮助页隐藏图像、窄屏缩放、图片滚出与滚回、正常退出。Kitty 模式验证了协议编码及 Bubble Tea 实际输出中的传输顺序、定位字符和颜色 ID 保留；用户已在 iTerm2 确认原图显示正常，Kitty 和 Ghostty 尚待单独实测。

2026-11-13 网络图片更新通过了 HTTP/HTTPS 下载、自动刷新、缓存复用、重定向、失败提示、超时与退出取消测试，以及竞态检查、静态检查和三平台构建。macOS 的真实 tmux 会话已验证从 HTTPS 地址下载 Go 标志后自动显示彩色图像、窄屏缩放和布局切换。

2026-11-14 Mermaid 更新已通过自动测试、竞态检查、`go vet` 和构建。测试覆盖 mmdc 检测、后台完成后刷新、超时取消、缺少浏览器与损坏 PNG 回退、缓存和临时目录清理，以及 1 / 2 / 3 倍出图的显示尺寸一致性和上传像素保留。Windows、Linux 构建通过，原生交互仍待验证。

本机使用 mmdc 12.2.0 与 chrome-headless-shell 156.0.8078.47，六类示例均已实际生成 PNG，终端状态栏显示完成 6/6。原图及缩放后显示效果已由用户在 iTerm2 确认。示例时序图由 650×586 提升到 1950×1758，终端显示尺寸按原始排版大小计算。

持久化缓存更新已通过跨会话复用、高清倍率保留、LRU 容量淘汰、损坏重建、同图并发生成合并、锁等待取消和清理期间旧任务完成的测试。macOS 真实 tmux 会话验证了首次生成六张图、退出后重新启动显示“缓存命中 6”，以及清理磁盘缓存后活动会话继续显示和重排图片。Windows、Linux 已交叉构建，持久化缓存的原生交互仍待验证。

2026-11-15 多图启动优化通过了原始 PNG 逐字节传输、Kitty 分块边界和滚动映射探测的回归测试。滚动映射修复了代码围栏后接分隔线时拆分图表、误发起额外 mmdc 任务的问题。一次对照实测使用 1524 行、9 张缓存图、约 4000 万像素的文档，单次 Kitty 预览由约 1.7 秒降至 0.23 秒。140×40 PTY 响应 iTerm2 能力查询的启动测试中，到“缓存命中 9”状态输出由约 8.4 秒降至 2.0 秒，该测量包含应用启动和协议输出。

## 参考项目

| 项目 | 借鉴 / 使用方式 |
| --- | --- |
| [sho-luv/mdedit](https://github.com/sho-luv/mdedit) | Rust 终端编辑器，参考双列交互、三种视图和快捷键；其 README 未明确列出 LaTeX 支持 |
| [Splitmark/splitmark](https://github.com/Splitmark/splitmark) | 参考终端 Markdown 分屏编辑产品形态 |
| [charmbracelet/glow](https://github.com/charmbracelet/glow) | Go Markdown 阅读器，参考终端预览体验 |
| [charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea) | 直接使用终端事件循环和跨平台输入支持 |
| [charmbracelet/bubbles](https://github.com/charmbracelet/bubbles) | 直接使用预览滚动组件 |
| [charmbracelet/glamour](https://github.com/charmbracelet/glamour) | 直接使用 Markdown ANSI 渲染和代码高亮 |
| [spacedock-dev/mermaidtext](https://github.com/spacedock-dev/mermaidtext) | 直接使用 Mermaid 字符渲染库，版本固定为 v0.1.1 |
| [mermaid-js/mermaid-cli](https://github.com/mermaid-js/mermaid-cli) | 检测本机 mmdc，后台生成 Mermaid PNG |
| [doug/termtex](https://github.com/doug/termtex) | 直接使用 LaTeX 到 Unicode 的数学排版 |
| [junosuarez/termtex](https://github.com/junosuarez/termtex) | 后续图片公式方向的参考；依赖 Kitty Graphics Protocol 才能显示图形效果 |

编辑和文件保存逻辑在本项目中实现；没有复制上述完整编辑器的代码。依赖版本锁定在 `go.mod` 和 `go.sum`。
