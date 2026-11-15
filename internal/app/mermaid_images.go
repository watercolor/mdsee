package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

type mermaidImageEntry struct {
	path, dir, problem string
	scale              int
	diskHit            bool
	loading            bool
	running            bool
	used               uint64
	finished           time.Time
	diskTouched        time.Time
}

type mermaidImageCache struct {
	mu           sync.Mutex
	binary, dir  string
	disk         *mermaidDiskCache
	cacheProblem string
	entries      map[string]*mermaidImageEntry
	clock        uint64
	ctx          context.Context
	cancel       context.CancelFunc
	workers      chan struct{}
	updates      chan<- struct{}
	wg           sync.WaitGroup
	closeOnce    sync.Once
	timeout      time.Duration
}

func newMermaidImageCache(updates chan<- struct{}) *mermaidImageCache {
	binary, _ := exec.LookPath("mmdc")
	ctx, cancel := context.WithCancel(context.Background())
	return &mermaidImageCache{binary: binary, entries: map[string]*mermaidImageEntry{}, ctx: ctx, cancel: cancel, workers: make(chan struct{}, 2), updates: updates, timeout: 20 * time.Second}
}

func (c *mermaidImageCache) close() {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.cancel()
		c.mu.Unlock()
		c.wg.Wait()
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.dir != "" {
			_ = os.RemoveAll(c.dir)
		}
	})
}

// Image decoding runs under the cache lock so eviction cannot delete its PNG mid-read.
// The image renderer retains the resulting rows for the current render and mapping probes.
func (c *mermaidImageCache) preview(source, theme string, images *imageRenderer) (string, string) {
	if c == nil || c.binary == "" {
		return "", ""
	}
	if len(source) > 64*1024 {
		return "", "Mermaid 超过 64 KiB，使用字符回退"
	}
	key := mermaidImageKey(source, theme)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ctx.Err() != nil {
		return "", ""
	}
	c.clock++
	if entry, ok := c.entries[key]; ok {
		entry.used = c.clock
		if entry.loading {
			return "", "mmdc 生成中"
		}
		if entry.problem == "" {
			if c.disk != nil && time.Since(entry.diskTouched) >= time.Minute {
				if err := c.disk.touch(c.ctx, key); err != nil {
					c.cacheProblem = "磁盘缓存更新失败"
				}
				entry.diskTouched = time.Now()
			}
			address := (&url.URL{Scheme: "file", Path: filepath.ToSlash(entry.path)}).String()
			preview := images.renderPreview(address, entry.scale)
			if preview.err != nil {
				return "", "mmdc 图片无法显示，使用字符回退"
			}
			return address, ""
		}
		if time.Since(entry.finished) < 30*time.Second {
			return "", entry.problem
		}
		_ = os.RemoveAll(entry.dir)
		delete(c.entries, key)
	}
	if len(c.entries) >= maxPreviewImages {
		oldest := ""
		var used uint64
		for key, entry := range c.entries {
			if !entry.loading && (oldest == "" || entry.used < used) {
				oldest, used = key, entry.used
			}
		}
		if oldest == "" {
			return "", "mmdc 队列已满，使用字符回退"
		}
		_ = os.RemoveAll(c.entries[oldest].dir)
		delete(c.entries, oldest)
	}

	if c.dir == "" {
		dir, err := os.MkdirTemp("", "mdcli-mermaid-*")
		if err != nil {
			return "", "无法创建 Mermaid 临时目录"
		}
		c.dir = dir
	}
	var epoch string
	if c.disk != nil {
		data, meta, hit, err := c.disk.load(c.ctx, key)
		if err != nil {
			c.cacheProblem = "磁盘缓存读取失败"
		}
		if hit {
			entry, err := c.restoreDisk(data, meta)
			if err == nil {
				entry.used = c.clock
				c.entries[key] = entry
				address := (&url.URL{Scheme: "file", Path: filepath.ToSlash(entry.path)}).String()
				if preview := images.renderPreview(address, entry.scale); preview.err == nil {
					return address, ""
				}
				_ = os.RemoveAll(entry.dir)
				delete(c.entries, key)
			}
		}
		var epochErr error
		epoch, epochErr = c.disk.epoch(c.ctx)
		if epochErr != nil {
			c.cacheProblem = "磁盘缓存不可用"
		}
	}

	entry := &mermaidImageEntry{loading: true, used: c.clock}
	c.entries[key] = entry
	c.wg.Add(1)
	c.notify()
	go c.generate(source, theme, epoch, entry)
	return "", "mmdc 生成中"
}

func (c *mermaidImageCache) generate(source, theme, epoch string, entry *mermaidImageEntry) {
	defer c.wg.Done()
	select {
	case c.workers <- struct{}{}:
		defer func() { <-c.workers }()
	case <-c.ctx.Done():
		return
	}
	c.mu.Lock()
	entry.running = true
	c.mu.Unlock()
	c.notify()
	key := mermaidImageKey(source, theme)
	if c.disk != nil && epoch != "" {
		lock, err := c.disk.renderLock(c.ctx, c.disk.key(key))
		if err == nil {
			defer lock.Close()
			data, meta, hit, loadErr := c.disk.load(c.ctx, key)
			if loadErr == nil && hit {
				c.mu.Lock()
				restored, restoreErr := c.restoreDisk(data, meta)
				if restoreErr == nil {
					*entry = *restored
					entry.used = c.clock
				}
				c.mu.Unlock()
				if restoreErr == nil {
					c.notify()
					return
				}
			}
		} else if c.ctx.Err() == nil {
			c.mu.Lock()
			c.cacheProblem = "磁盘缓存锁定失败"
			c.mu.Unlock()
			epoch = ""
		}
	}
	dir, err := os.MkdirTemp(c.dir, "diagram-*")
	path := filepath.Join(dir, "diagram.png")
	var logs bytes.Buffer
	renderScale := 1
	if err == nil {
		input := filepath.Join(dir, "diagram.mmd")
		err = os.WriteFile(input, []byte(source), 0600)
		if err == nil {
			selectedTheme := "default"
			if theme == "dark" {
				selectedTheme = "dark"
			}
			ctx, cancel := context.WithTimeout(c.ctx, c.timeout)
			for scale := 3; scale >= 1; scale-- {
				renderScale = scale
				logs.Reset()
				cmd := exec.CommandContext(ctx, c.binary, "-i", input, "-o", path, "-t", selectedTheme, "-b", "transparent", "-s", strconv.Itoa(scale))
				cmd.Stderr = &boundedMermaidLog{writer: &logs, remaining: 8192}
				cmd.WaitDelay = time.Second
				configureMermaidCommand(cmd)
				err = cmd.Run()
				if ctx.Err() != nil {
					err = ctx.Err()
				}
				if err != nil {
					break
				}
				_, err = readLocalImage(path)
				if err == nil || !(errors.Is(err, errImageBytesLimit) || errors.Is(err, errImagePixelsLimit)) {
					break
				}
			}
			cancel()
		}
		_ = os.Remove(input)
	}
	if err == nil && c.disk != nil && epoch != "" {
		if cacheErr := c.disk.publish(c.ctx, key, path, renderScale, epoch); cacheErr != nil {
			c.mu.Lock()
			c.cacheProblem = "磁盘缓存写入失败"
			c.mu.Unlock()
		}
	}
	c.mu.Lock()
	entry.loading = false
	entry.running = false
	entry.finished = time.Now()
	entry.diskTouched = entry.finished
	entry.dir = dir
	if err == nil {
		entry.path = path
		entry.scale = renderScale
	} else {
		entry.problem = "mmdc 渲染失败，使用字符回退"
		if strings.Contains(logs.String(), "Could not find") && (strings.Contains(logs.String(), "chrome") || strings.Contains(logs.String(), "browser")) {
			entry.problem = "mmdc 缺少浏览器，使用字符回退"
		}
		if err == context.DeadlineExceeded {
			entry.problem = "mmdc 渲染超时，使用字符回退"
		}
		if dir != "" {
			_ = os.RemoveAll(dir)
		}
	}
	c.mu.Unlock()
	c.notify()
}

func (r *imageMarkdownRenderer) renderDiagramNodes(raw []byte, root ast.Node) []byte {
	if r.images.mode == "off" {
		return raw
	}
	var codes []*ast.FencedCodeBlock
	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if code, ok := node.(*ast.FencedCodeBlock); entering && ok && string(code.Language(raw)) == "mermaid" {
			codes = append(codes, code)
		}
		return ast.WalkContinue, nil
	})
	for _, code := range codes {
		var source strings.Builder
		for i := 0; i < code.Lines().Len(); i++ {
			segment := code.Lines().At(i)
			source.Write(segment.Value(raw))
		}
		value := source.String()
		r.images.diagramKeys[mermaidImageKey(value, r.images.theme)] = struct{}{}
		address, hint := r.images.diagrams.preview(value, r.images.theme, r.images)
		parent := code.Parent()
		if address != "" {
			link := ast.NewLink()
			link.Destination = []byte(address)
			img := ast.NewImage(link)
			start := len(raw)
			raw = append(raw, []byte("Mermaid")...)
			img.AppendChild(img, ast.NewTextSegment(text.NewSegment(start, len(raw))))
			paragraph := ast.NewParagraph()
			paragraph.AppendChild(paragraph, img)
			parent.ReplaceChild(parent, code, paragraph)
		} else {
			rendered, err := renderMermaid(value, max(10, r.images.width-8))
			if err != nil {
				rendered = "Mermaid 未渲染（" + err.Error() + "）\n" + value
			}
			if hint != "" {
				rendered = hint + "\n" + rendered
			}
			start := len(raw)
			raw = append(raw, []byte(rendered+"\n")...)
			replacement := ast.NewFencedCodeBlock(nil)
			replacement.Lines().Append(text.NewSegment(start, len(raw)))
			parent.ReplaceChild(parent, code, replacement)
		}
	}
	return raw
}

func mermaidImageKey(source, theme string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(theme+"\x00"+strings.ReplaceAll(source, "\r\n", "\n"))))
}

func (c *mermaidImageCache) notify() {
	select {
	case c.updates <- struct{}{}:
	default:
	}
}

type mermaidProgress struct {
	failure                                   string
	total, completed, failed, running, queued int
	hits                                      int
	cacheProblem                              string
	detected                                  bool
}

func (p mermaidProgress) pending() bool { return p.running+p.queued > 0 }
func (p mermaidProgress) message() string {
	result := p.renderMessage()
	if p.hits > 0 {
		result += fmt.Sprintf(" · 缓存命中 %d", p.hits)
	}
	if p.cacheProblem != "" {
		result += " · " + p.cacheProblem
	}
	return result
}
func (p mermaidProgress) renderMessage() string {
	if !p.detected {
		return "未检测到 mmdc · Mermaid 字符渲染"
	}
	if p.pending() {
		return fmt.Sprintf("mmdc 渲染中 %d/%d · 运行 %d · 排队 %d", p.completed+p.failed, p.total, p.running, p.queued)
	}
	if p.failed > 0 {
		message := fmt.Sprintf("mmdc 完成 %d/%d · 字符回退 %d", p.completed, p.total, p.failed)
		if p.failure != "" {
			message += " · " + p.failure
		}
		return message
	}
	return fmt.Sprintf("mmdc 已完成 %d/%d", p.completed, p.total)
}
func (p mermaidProgress) compact() string {
	result := p.compactMessage()
	if p.hits > 0 {
		result += fmt.Sprintf(" 缓存%d", p.hits)
	}
	return result
}
func (p mermaidProgress) compactMessage() string {
	if !p.detected {
		return "Mermaid 字符渲染"
	}
	if p.pending() {
		return fmt.Sprintf("渲染 %d/%d", p.completed+p.failed, p.total)
	}
	if p.failed > 0 {
		return fmt.Sprintf("完成 %d/%d 回退%d", p.completed, p.total, p.failed)
	}
	return fmt.Sprintf("完成 %d/%d", p.completed, p.total)
}
func (c *mermaidImageCache) progress(keys []string) mermaidProgress {
	p := mermaidProgress{total: len(keys)}
	if c == nil {
		return p
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	p.detected = c.binary != ""
	p.cacheProblem = c.cacheProblem
	if !p.detected {
		return p
	}
	for _, key := range keys {
		entry, ok := c.entries[key]
		switch {
		case !ok:
			p.failed++
		case entry.loading && entry.running:
			p.running++
		case entry.loading:
			p.queued++
		case entry.problem != "":
			p.failed++
			if p.failure == "" {
				p.failure = strings.TrimSuffix(strings.TrimPrefix(entry.problem, "mmdc "), "，使用字符回退")
			}
		default:
			p.completed++
			if entry.diskHit {
				p.hits++
			}
		}
	}
	return p
}

type boundedMermaidLog struct {
	writer    io.Writer
	remaining int
}

func (w *boundedMermaidLog) Write(data []byte) (int, error) {
	size := len(data)
	if w.remaining > 0 {
		n, err := w.writer.Write(data[:min(size, w.remaining)])
		w.remaining -= n
		if err != nil {
			return n, err
		}
	}
	return size, nil
}

func (c *mermaidImageCache) configureDisk(root string, limit int64) {
	if limit <= 0 {
		return
	}
	if root == "" {
		var err error
		root, err = defaultMermaidCacheDir()
		if err != nil {
			c.cacheProblem = "磁盘缓存目录不可用"
			return
		}
	}
	disk := &mermaidDiskCache{root: root, limit: limit, renderer: mermaidRendererIdentity(c.binary)}
	if err := disk.prune(c.ctx); err != nil {
		c.cacheProblem = "磁盘缓存不可用"
		return
	}
	c.disk = disk
}

// Retain a session-local snapshot so clearing or evicting disk entries cannot
// remove an image currently being read, displayed, or used by scroll-map probes.
func (c *mermaidImageCache) restoreDisk(data []byte, meta mermaidDiskMetadata) (*mermaidImageEntry, error) {
	dir, err := os.MkdirTemp(c.dir, "diagram-*")
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "diagram.png")
	if err := os.WriteFile(path, data, 0600); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	now := time.Now()
	return &mermaidImageEntry{path: path, dir: dir, scale: meta.Scale, diskHit: true, finished: now, diskTouched: now}, nil
}
