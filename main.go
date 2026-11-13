package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

func run(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("mdcli", flag.ContinueOnError)
	fs.SetOutput(out)
	modeFlag := fs.String("mode", "split", "初始布局：split / edit / preview")
	theme := fs.String("theme", "dark", "预览主题：dark / light / ascii")
	printOnly := fs.Bool("print", false, "输出预览后退出（可重定向）")
	width := fs.Int("width", 80, "--print 输出宽度")
	vim := fs.Bool("vim", false, "启动时启用 Vim Normal 模式（F4 切换）")
	fs.Usage = func() {
		fmt.Fprint(out, "用法：mdcli [选项] [文件.md]\n不指定文件时编辑当前目录的 untitled.md。\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if fs.NArg() > 1 {
		return fmt.Errorf("一次只能打开一个文件；选项请放在文件名前")
	}
	modes := map[string]viewMode{"split": splitMode, "edit": editMode, "preview": previewMode}
	mode, ok := modes[*modeFlag]
	if !ok {
		return fmt.Errorf("无效布局：%s", *modeFlag)
	}
	if *theme != "dark" && *theme != "light" && *theme != "ascii" {
		return fmt.Errorf("主题必须为 dark、light 或 ascii")
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
	_, err = tea.NewProgram(newModel(d, mode, *theme, *vim)).Run()
	return err
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "mdcli:", err)
		os.Exit(1)
	}
}
