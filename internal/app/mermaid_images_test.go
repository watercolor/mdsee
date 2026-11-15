package app

import (
	"bytes"
	tea "charm.land/bubbletea/v2"
	"context"
	"encoding/binary"
	"github.com/charmbracelet/colorprofile"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

// The test binary stands in for mmdc so process and PNG handling work on all platforms.
func init() {
	if os.Getenv("MDSEE_MMDC_HELPER") != "1" {
		return
	}
	args := os.Args
	for _, arg := range args {
		if arg == "-w" {
			os.Exit(2)
		}
	}
	input, output := "", ""
	scale := 1
	for i := 1; i+1 < len(args); i++ {
		switch args[i] {
		case "-s":
			scale, _ = strconv.Atoi(args[i+1])
		case "-i":
			input = args[i+1]
		case "-o":
			output = args[i+1]
		}
	}
	if input == "" || output == "" {
		os.Exit(3)
	}
	if countPath := os.Getenv("MDSEE_MMDC_COUNT"); countPath != "" {
		file, err := os.OpenFile(countPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			os.Exit(7)
		}
		file.WriteString("render\n")
		file.Close()
	}
	data, err := os.ReadFile(input)
	if err != nil {
		os.Exit(4)
	}
	if strings.Contains(string(data), "BROWSER") {
		os.Stderr.WriteString("Could not find chrome-headless-shell")
		os.Exit(1)
	}
	if strings.Contains(string(data), "FAIL") {
		os.Exit(1)
	}
	if strings.Contains(string(data), "SLOW") {
		time.Sleep(10 * time.Second)
	}
	if strings.Contains(string(data), "INVALID") {
		os.WriteFile(output, []byte("bad PNG"), 0600)
		os.Exit(0)
	}
	width, height := 40, 20
	if strings.Contains(string(data), "DPI") || strings.Contains(string(data), "LARGE") {
		width *= scale
		height *= scale
	}
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	file, err := os.Create(output)
	if err != nil {
		os.Exit(5)
	}
	if png.Encode(file, img) != nil {
		os.Exit(6)
	}
	file.Close()
	if strings.Contains(string(data), "LARGE") && scale == 3 {
		raw, _ := os.ReadFile(output)
		binary.BigEndian.PutUint32(raw[16:20], 3000)
		binary.BigEndian.PutUint32(raw[20:24], 3000)
		binary.BigEndian.PutUint32(raw[29:33], crc32.ChecksumIEEE(raw[12:29]))
		os.WriteFile(output, raw, 0600)
	}
	os.Exit(0)
}

// Child copies of the test binary handle the mmdc arguments before test flag parsing.
func helperMermaidCache(t *testing.T) *mermaidImageCache {
	t.Helper()
	t.Setenv("MDSEE_MMDC_HELPER", "1")
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c := newMermaidImageCache(make(chan struct{}, 32))
	c.binary = binary
	t.Cleanup(c.close)
	return c
}
func awaitMermaid(t *testing.T, c *mermaidImageCache) {
	t.Helper()
	c.wg.Wait()
}
func diagramRenderWithCache(t *testing.T, c *mermaidImageCache, mode, source string) (string, *imageRenderer) {
	t.Helper()
	images := newImageRenderer(mode, filepath.Join(t.TempDir(), "doc.md"), "dark", 80, 0x123400, .5)
	images.diagrams = c
	content, _, err := renderMappedMarkdown(source, 80, "dark", images)
	if err != nil {
		t.Fatal(err)
	}
	return content, images
}

func TestMermaidPNGPreferenceCachingAndCleanup(t *testing.T) {
	c := helperMermaidCache(t)
	source := "# 标题\n\n```mermaid\ngraph LR\nA --> B\n```\n\n![local](missing.png)\n\n## 后文"
	initial, _ := diagramRenderWithCache(t, c, "kitty", source)
	if !strings.Contains(initial, "mmdc 生成中") {
		t.Fatal("must render text while PNG is pending")
	}
	awaitMermaid(t, c)
	content, images := diagramRenderWithCache(t, c, "kitty", source)
	if !strings.ContainsRune(content, kitty.Placeholder) || len(images.ids) != 1 {
		t.Fatal("missing generated image")
	}
	decodeKittyUpload(t, images.graphics.String())
	if !strings.Contains(ansi.Strip(content), "后文") {
		t.Fatal("lost surrounding content")
	}
	c.mu.Lock()
	if len(c.entries) != 1 {
		t.Fatal("scroll mapping must not duplicate generation")
	}
	var path string
	for _, entry := range c.entries {
		path = entry.path
	}
	dir := c.dir
	c.mu.Unlock()
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	diagramRenderWithCache(t, c, "blocks", source)
	after, _ := os.Stat(path)
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("cache regenerated unchanged source")
	}
	c.close()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("session temporary directory was not removed")
	}
}

func TestMermaidPNGFailureTimeoutAndUnsupportedFallback(t *testing.T) {
	for _, source := range []string{"FAIL", "INVALID", "SLOW"} {
		t.Run(source, func(t *testing.T) {
			c := helperMermaidCache(t)
			c.timeout = 100 * time.Millisecond
			md := "```mermaid\ngraph LR\nA[" + source + "] --> B\n```"
			diagramRenderWithCache(t, c, "blocks", md)
			awaitMermaid(t, c)
			content, images := diagramRenderWithCache(t, c, "blocks", md)
			if len(images.ids) != 0 || strings.Contains(content, "▀") || !strings.Contains(content, "字符回退") || !strings.Contains(content, source) {
				t.Fatal("failed mmdc must retain text diagram")
			}
		})
	}
	output, err := renderMermaid("mindmap\n root\n  child", 60)
	if err != nil || !strings.Contains(output, "字符回退") || !strings.Contains(output, "child") {
		t.Fatal(output, err)
	}
}

func TestMermaidDetectionOffPrintAndFences(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	c := newMermaidImageCache(make(chan struct{}, 1))
	defer c.close()
	if c.binary != "" {
		t.Fatal("unexpected mmdc detection")
	}
	content, _ := diagramRenderWithCache(t, c, "blocks", "```mermaid\nsequenceDiagram\nA->>B: hello\n```")
	if strings.Contains(content, "mmdc") || !strings.Contains(content, "hello") {
		t.Fatal(content)
	}
	helper := helperMermaidCache(t)
	source := "```mermaid\ngraph LR\nA --> B\n```"
	diagramRenderWithCache(t, helper, "off", source)
	if len(helper.entries) != 0 {
		t.Fatal("off mode must not start mmdc")
	}
	path := filepath.Join(t.TempDir(), "doc.md")
	os.WriteFile(path, []byte(source), 0600)
	var output bytes.Buffer
	if err := Run([]string{"--print", path}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "A") || strings.Contains(output.String(), "mmdc") {
		t.Fatal(output.String())
	}
	code := "````markdown\n" + source + "\n````"
	diagramRenderWithCache(t, helper, "blocks", code)
	if len(helper.entries) != 0 {
		t.Fatal("literal fenced example must not invoke mmdc")
	}
}

func TestMermaidCloseCancelsAndDrains(t *testing.T) {
	c := helperMermaidCache(t)
	c.timeout = 10 * time.Second
	for i := 0; i < 4; i++ {
		source := "```mermaid\ngraph LR\nA[SLOW" + strings.Repeat("x", i) + "] --> B\n```"
		diagramRenderWithCache(t, c, "blocks", source)
	}
	started := time.Now()
	c.close()
	if time.Since(started) > 3*time.Second {
		t.Fatal("exit waited for rendering timeout")
	}
}

func TestMermaidMmdcDiscoveryAndNestedImages(t *testing.T) {
	t.Setenv("MDSEE_MMDC_HELPER", "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "mmdc commands 中文")
	os.MkdirAll(dir, 0700)
	name := "mmdc"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	target := filepath.Join(dir, name)
	if err := os.Link(executable, target); err != nil {
		data, err := os.ReadFile(executable)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	c := newMermaidImageCache(make(chan struct{}, 10))
	defer c.close()
	if c.binary == "" {
		t.Fatal("mmdc was not discovered on PATH")
	}
	for _, prefix := range []string{"> ", "  "} {
		source := prefix + "```mermaid\n" + prefix + "graph LR\n" + prefix + "A --> B\n" + prefix + "```\n\n## 后文"
		diagramRenderWithCache(t, c, "kitty", source)
		awaitMermaid(t, c)
		rendered, images := diagramRenderWithCache(t, c, "kitty", source)
		if len(images.ids) != 1 || !strings.ContainsRune(rendered, kitty.Placeholder) {
			t.Fatalf("nested PNG missing: %q", rendered)
		}
	}
}

func TestMermaidAutoRefreshThroughBubbleTea(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TMUX", "")
	m := testModel(t)
	m.diagrams.close()
	m.diagrams = helperMermaidCache(t)
	m.diagrams.updates = m.remoteImages.updates
	m.editor = newEditor("```mermaid\ngraph LR\nA --> B\n```")
	m.configureImages("kitty")
	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := tea.NewProgram(imageProgramModel{m}, tea.WithInput(nil), tea.WithOutput(&out), tea.WithContext(ctx), tea.WithColorProfile(colorprofile.TrueColor), tea.WithWindowSize(80, 24), tea.WithoutSignalHandler()).Run()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.ContainsRune(out.String(), kitty.Placeholder) {
		t.Fatal("PNG completion must refresh the preview")
	}
	decodeKittyUpload(t, out.String())
}

func TestMermaidStatusProgressAndCurrentDocument(t *testing.T) {
	m := testModel(t)
	m.diagrams.binary = "mmdc"
	m.diagrams.entries = map[string]*mermaidImageEntry{
		"ready":          {path: "ready.png"},
		"running":        {loading: true, running: true},
		"queued":         {loading: true},
		"failed":         {problem: "mmdc 缺少浏览器，使用字符回退"},
		"other-document": {loading: true},
	}
	m.diagramKeys = []string{"ready", "running", "queued"}
	p := m.diagrams.progress(m.diagramKeys)
	if p.completed != 1 || p.running != 1 || p.queued != 1 || p.total != 3 {
		t.Fatalf("incorrect progress %+v", p)
	}
	m.status = "已保存"
	m.width = 100
	header := m.headerLine(" MDSEE note.md")
	for _, want := range []string{"mmdc 渲染中 1/3", "运行 1", "排队 1"} {
		if !strings.Contains(header, want) {
			t.Fatal(header)
		}
	}
	m.width = 30
	header = m.headerLine(" MDSEE note.md")
	if !strings.Contains(header, "渲染 1/3") || ansi.StringWidth(header) > 30 {
		t.Fatal(header)
	}
	m.status = ""
	m.width = 100
	m.diagramKeys = []string{"ready"}
	if !strings.Contains(m.headerLine(" MDSEE note.md"), "已完成 1/1") {
		t.Fatal("other documents must not keep this document pending")
	}
	m.diagramKeys = []string{"failed"}
	if header := m.headerLine(" MDSEE note.md"); !strings.Contains(header, "缺少浏览器") || !strings.Contains(header, "字符回退") {
		t.Fatal(header)
	}
	m.diagrams.binary = ""
	if !strings.Contains(m.headerLine(" MDSEE note.md"), "未检测到 mmdc") {
		t.Fatal("missing executable status")
	}
	m.switchBuffer(0)
	if len(m.diagramKeys) != 0 {
		t.Fatal("buffer switch must clear previous progress")
	}
}

func TestMermaidMissingBrowserStatus(t *testing.T) {
	c := helperMermaidCache(t)
	source := "```mermaid\ngraph LR\nA[BROWSER] --> B\n```"
	diagramRenderWithCache(t, c, "blocks", source)
	awaitMermaid(t, c)
	content, _ := diagramRenderWithCache(t, c, "blocks", source)
	if !strings.Contains(content, "缺少浏览器") {
		t.Fatal(content)
	}
}

func TestMermaidHighDPIAndOversizeRetry(t *testing.T) {
	for _, tc := range []struct {
		label         string
		width, height int
	}{{"DPI", 120, 60}, {"LARGE", 80, 40}} {
		t.Run(tc.label, func(t *testing.T) {
			c := helperMermaidCache(t)
			source := "```mermaid\ngraph LR\nA[" + tc.label + "] --> B\n```"
			diagramRenderWithCache(t, c, "kitty", source)
			awaitMermaid(t, c)
			_, images := diagramRenderWithCache(t, c, "kitty", source)
			img := decodeKittyUpload(t, images.graphics.String())
			if img.Bounds().Dx() != tc.width || img.Bounds().Dy() != tc.height {
				t.Fatalf("resolution lost or oversize retry failed: %v", img.Bounds())
			}
			for _, preview := range images.cache {
				if len(preview.rows) == 0 || ansi.StringWidth(preview.rows[0]) != 5 {
					t.Fatal("render density must be removed from layout, including retry at 2x")
				}
			}
		})
	}
}
