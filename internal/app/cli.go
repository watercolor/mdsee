package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

// Run parses command arguments and starts the terminal editor or print mode.
func Run(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("mdsee", flag.ContinueOnError)
	fs.SetOutput(out)
	modeFlag := fs.String("mode", "preview", "初始布局：split / edit / preview")
	theme := fs.String("theme", "dark", "界面与预览主题：dark / light / ascii")
	printOnly := fs.Bool("print", false, "输出预览后退出（可重定向）")
	width := fs.Int("width", 80, "--print 输出宽度")
	vim := fs.Bool("vim", true, "启用 Vim Normal 模式，--vim=false 关闭（F4 切换）")
	images := fs.String("images", "auto", "图片：auto / kitty / blocks / off；--print 输出文字")
	cacheDir := fs.String("mermaid-cache-dir", "", "Mermaid 磁盘缓存目录（默认系统用户缓存目录）")
	cacheMiB := fs.Int64("mermaid-cache-mib", defaultMermaidCacheBytes>>20, "Mermaid 磁盘缓存容量 MiB，0 关闭持久化")
	clearCache := fs.Bool("clear-mermaid-cache", false, "清理 Mermaid 磁盘缓存后退出")
	fs.Usage = func() {
		fmt.Fprint(out, "用法：mdsee [选项] [文件.md ...]\n不指定文件时编辑当前目录的 untitled.md。\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if *cacheMiB < 0 || *cacheMiB > (1<<63-1)/(1<<20) {
		return fmt.Errorf("Mermaid 缓存容量无效")
	}
	if *clearCache {
		root := *cacheDir
		if root == "" {
			var err error
			root, err = defaultMermaidCacheDir()
			if err != nil {
				return err
			}
		}
		disk := &mermaidDiskCache{root: root}
		if err := disk.clear(context.Background()); err != nil {
			return fmt.Errorf("清理 Mermaid 缓存：%w", err)
		}
		_, err := fmt.Fprintln(out, "Mermaid 磁盘缓存已清理："+root)
		return err
	}
	if *printOnly && fs.NArg() > 1 {
		return fmt.Errorf("--print 一次只能输出一个文件")
	}
	modes := map[string]viewMode{"split": splitMode, "edit": editMode, "preview": previewMode}
	mode, ok := modes[*modeFlag]
	if !ok {
		return fmt.Errorf("无效布局：%s", *modeFlag)
	}
	if *theme != "dark" && *theme != "light" && *theme != "ascii" {
		return fmt.Errorf("主题必须为 dark、light 或 ascii")
	}
	if *images != "auto" && *images != "kitty" && *images != "blocks" && *images != "off" {
		return fmt.Errorf("图片模式必须为 auto、kitty、blocks 或 off")
	}
	if *width < 20 || *width > 500 {
		return fmt.Errorf("输出宽度须在 20–500 之间")
	}
	path := fs.Arg(0)
	if path == "" {
		path = "untitled.md"
	}
	d, err := openDocument(path)
	if err != nil {
		return err
	}
	if *printOnly {
		if !d.exists {
			return fmt.Errorf("文件不存在：%s", path)
		}
		rendered, err := renderMarkdown(d.text, *width, *theme)
		if err != nil {
			return err
		}
		if f, ok := out.(*os.File); !ok || !term.IsTerminal(f.Fd()) {
			rendered = ansi.Strip(rendered)
		}
		_, err = fmt.Fprintln(out, rendered)
		return err
	}
	if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd()) {
		return fmt.Errorf("交互编辑需要终端；输出预览请使用 --print 文件.md")
	}
	m := newModel(d, mode, *theme, *vim)
	defer m.close()
	m.configureImages(*images)
	m.diagrams.configureDisk(*cacheDir, *cacheMiB*(1<<20))
	m.storeBuffer()
	for _, path := range fs.Args()[min(1, fs.NArg()):] {
		next, err := openDocument(path)
		if err != nil {
			return err
		}
		duplicate := false
		for _, b := range m.buffers {
			if b.doc.path == next.path {
				duplicate = true
			}
		}
		if !duplicate {
			m.buffers = append(m.buffers, documentBuffer{doc: next, editor: newEditor(next.text), vim: vimState{enabled: *vim}})
		}
	}
	defer func() { _, _ = fmt.Fprint(os.Stdout, deleteImageResources(m.imageIDs)) }()
	_, err = tea.NewProgram(m).Run()
	return err
}
