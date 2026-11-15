# 开发指南

面向修改源码、运行测试和参与项目开发的读者。日常使用与操作说明见 [使用指南](USAGE.md)，功能规划见 [待办清单](FEATURE_BACKLOG.md)。

## 开发环境

使用 Go 1.25.8 或更新版本。依赖版本由 `go.mod` 和 `go.sum` 管理。

```sh
go build -o bin/mdsee ./cmd/mdsee
./bin/mdsee examples/demo.md
```

Windows 构建使用 `go build -o bin/mdsee.exe ./cmd/mdsee`。

## 目录结构

```text
cmd/mdsee/main.go       命令入口
internal/app/          终端界面、编辑、导航、Markdown 和图片渲染
  app.go
  app_test.go          同包测试，其他模块同样使用 *_test.go
  cli.go
  ...
main.go                兼容根目录构建和运行的薄入口
docs/                  使用指南、开发指南和功能规划
examples/              示例 Markdown 和图片
```

Go 按包组织代码，`*_test.go` 与被测源码放在同一目录，执行 `go test` 时参与编译。当前应用代码集中在 `internal/app` 包。`cmd/mdsee` 为主要入口，根目录也支持 `go build .` 和 `go run .`。布局参考 [Go 官方项目组织指南](https://go.dev/doc/modules/layout)。

## 测试与跨平台构建

```sh
go test ./...
go test -race ./...
go vet ./...

CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o dist/mdsee-darwin-arm64 ./cmd/mdsee
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/mdsee-linux-amd64 ./cmd/mdsee
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o dist/mdsee-windows-amd64.exe ./cmd/mdsee
```

交叉构建覆盖 macOS arm64、Linux amd64 和 Windows amd64。原生交互验证按操作系统、终端和功能记录在下文。

CI 工作流位于 [ci.yml](../.github/workflows/ci.yml)，配置了三平台的测试、静态检查和构建。

## 预览与滚动映射

默认开启，状态栏显示“联动开”，按 `F7` 切换。使用 Markdown 内容块在实际渲染结果中的位置作为锚点，块内按局部行数插值；复杂块、引用链接及公式的内部位置属于近似定位，不保证源码行与预览行一一对应。

滚动预览时，原文视口移动到对应内容，尽量保留光标；光标超出新视口才调整到最近的可见行。切回编辑区保持当前位置。编辑和窗口宽度变化会异步重建映射，过期映射不会用于移动光标。

Mermaid 图片生成、磁盘缓存格式和图片协议的详细行为见 [使用指南](USAGE.md#mermaid-渲染)。相关实现位于 `internal/app/mermaid_images.go`、`mermaid_disk.go`、`images.go` 和 `scroll.go`。

## 本地验证记录

以下为开发过程中的验证记录，日期、平台和终端限定了对应结果的范围。

2026-11-12 本地验证：`go test ./...`、`go test -race ./...`、`go vet ./...` 通过；上述三平台构建通过。macOS arm64 的真实 tmux 终端会话已验证布局切换、预览翻页、中文输入、多行粘贴、撤销/重做、保存、窄屏焦点切换和未保存退出提示。Linux、Windows 尚未进行原生交互实测；仓库中的 GitHub Actions 配置尚未运行。

Vim 与关联滚动更新已通过同样的测试、竞态检查、静态检查和三平台构建；macOS 真实终端另外验证了 `--vim`、F4、Normal / Insert、Ctrl+C、yy/p/u、`:q` / `:wq`、双向滚动、切焦点保持视口以及 F7 开关联动。

本地图片更新已通过自动测试、竞态检查、静态检查和 macOS arm64 / Linux amd64 / Windows amd64 构建。macOS 的真实 tmux 会话验证了默认图片预览、双列布局、编辑与帮助页隐藏图像、窄屏缩放、图片滚出与滚回、正常退出。Kitty 模式验证了协议编码及 Bubble Tea 实际输出中的传输顺序、定位字符和颜色 ID 保留；用户已在 iTerm2 确认原图显示正常，Kitty 和 Ghostty 尚待单独实测。

2026-11-13 网络图片更新通过了 HTTP/HTTPS 下载、自动刷新、缓存复用、重定向、失败提示、超时与退出取消测试，以及竞态检查、静态检查和三平台构建。macOS 的真实 tmux 会话已验证从 HTTPS 地址下载 Go 标志后自动显示彩色图像、窄屏缩放和布局切换。

2026-11-14 Mermaid 更新已通过自动测试、竞态检查、`go vet` 和构建。测试覆盖 mmdc 检测、后台完成后刷新、超时取消、缺少浏览器与损坏 PNG 回退、缓存和临时目录清理，以及 1 / 2 / 3 倍出图的显示尺寸一致性和上传像素保留。Windows、Linux 构建通过，原生交互仍待验证。

本机使用 mmdc 12.2.0 与 chrome-headless-shell 156.0.8078.47，六类示例均已实际生成 PNG，终端状态栏显示完成 6/6。原图及缩放后显示效果已由用户在 iTerm2 确认。示例时序图由 650×586 提升到 1950×1758，终端显示尺寸按原始排版大小计算。

持久化缓存更新已通过跨会话复用、高清倍率保留、LRU 容量淘汰、损坏重建、同图并发生成合并、锁等待取消和清理期间旧任务完成的测试。macOS 真实 tmux 会话验证了首次生成六张图、退出后重新启动显示“缓存命中 6”，以及清理磁盘缓存后活动会话继续显示和重排图片。Windows、Linux 已交叉构建，持久化缓存的原生交互仍待验证。

2026-11-15 多图启动优化通过了原始 PNG 逐字节传输、Kitty 分块边界和滚动映射探测的回归测试。滚动映射修复了代码围栏后接分隔线时拆分图表、误发起额外 mmdc 任务的问题。一次对照实测使用 1524 行、9 张缓存图、约 4000 万像素的文档，单次 Kitty 预览由约 1.7 秒降至 0.23 秒。140×40 PTY 响应 iTerm2 能力查询的启动测试中，到“缓存命中 9”状态输出由约 8.4 秒降至 2.0 秒，该测量包含应用启动和协议输出。

## 技术依赖与参考

| 项目 | 借鉴 / 使用方式 |
| --- | --- |
| [sho-luv/mdedit](https://github.com/sho-luv/mdedit) | Rust 终端编辑器，参考双列交互、三种视图和快捷键 |
| [Splitmark/splitmark](https://github.com/Splitmark/splitmark) | 参考终端 Markdown 分屏编辑产品形态 |
| [charmbracelet/glow](https://github.com/charmbracelet/glow) | Go Markdown 阅读器，参考终端预览体验 |
| [charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea) | 直接使用终端事件循环和跨平台输入支持 |
| [charmbracelet/bubbles](https://github.com/charmbracelet/bubbles) | 直接使用预览滚动组件 |
| [charmbracelet/glamour](https://github.com/charmbracelet/glamour) | 直接使用 Markdown ANSI 渲染 |
| [alecthomas/chroma](https://github.com/alecthomas/chroma) | 直接用于预览代码块的语法高亮 |
| [bahdotsh/mdterm](https://github.com/bahdotsh/mdterm) | 参考终端阅读交互与深浅配色 |
| [spacedock-dev/mermaidtext](https://github.com/spacedock-dev/mermaidtext) | 直接使用 Mermaid 字符渲染库，版本固定为 v0.1.1 |
| [mermaid-js/mermaid-cli](https://github.com/mermaid-js/mermaid-cli) | 检测本机 mmdc，后台生成 Mermaid PNG |
| [doug/termtex](https://github.com/doug/termtex) | 直接使用 LaTeX 到 Unicode 的数学排版 |
| [junosuarez/termtex](https://github.com/junosuarez/termtex) | 后续图片公式方向的参考；依赖 Kitty Graphics Protocol 才能显示图形效果 |

编辑和文件保存逻辑在本项目中实现。依赖版本锁定在 `go.mod` 和 `go.sum`。
