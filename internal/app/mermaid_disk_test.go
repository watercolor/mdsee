package app

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func diskTestPNG(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "diagram.png")
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewNRGBA(image.Rect(0, 0, 120, 60))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buffer.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func publishDiskTest(t *testing.T, disk *mermaidDiskCache, source, path string) {
	t.Helper()
	epoch, err := disk.epoch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := disk.publish(context.Background(), source, path, 3, epoch); err != nil {
		t.Fatal(err)
	}
}

func TestMermaidDiskAcrossSessionsAndFooter(t *testing.T) {
	root := t.TempDir()
	md := "```mermaid\ngraph LR\nA[DPI] --> B\n```"
	key := mermaidImageKey("graph LR\nA[DPI] --> B\n", "dark")
	first := helperMermaidCache(t)
	first.configureDisk(root, defaultMermaidCacheBytes)
	diagramRenderWithCache(t, first, "kitty", md)
	awaitMermaid(t, first)
	temp := first.dir
	first.close()
	if _, err := os.Stat(temp); !os.IsNotExist(err) {
		t.Fatal("session files retained", err)
	}

	second := helperMermaidCache(t)
	second.configureDisk(root, defaultMermaidCacheBytes)
	second.timeout = time.Nanosecond
	content, images := diagramRenderWithCache(t, second, "kitty", md)
	if strings.Contains(content, "生成中") || len(images.ids) != 1 {
		t.Fatal("persistent hit must display synchronously without mmdc", content)
	}
	if img := decodeKittyUpload(t, images.graphics.String()); img.Bounds().Dx() != 120 {
		t.Fatal("cached image lost source pixels", img.Bounds())
	}
	entry := second.entries[key]
	if entry == nil || !entry.diskHit || entry.scale != 3 {
		t.Fatalf("cache metadata not restored: %+v", entry)
	}
	m := testModel(t)
	m.diagrams.close()
	m.diagrams = second
	m.diagramKeys = []string{key}
	m.width = 120
	if header := m.headerLine(" MDCLI note.md"); !strings.Contains(header, "缓存命中 1") {
		t.Fatal(header)
	}
	m.status = "已切换文件"
	if header := m.headerLine(" MDCLI note.md"); !strings.Contains(header, "缓存命中 1") {
		t.Fatal(header)
	}
	if footer := m.statusLine("NORMAL"); !strings.Contains(footer, m.status) {
		t.Fatal("cache status replaced the action message", footer)
	}
	m.width = 40
	if header := m.headerLine(" MDCLI note.md"); !strings.Contains(header, "缓存1") {
		t.Fatal(header)
	}
	old := time.Now().Add(-time.Hour)
	cachePath := filepath.Join(root, second.disk.key(key))
	if err := os.Chtimes(cachePath, old, old); err != nil {
		t.Fatal(err)
	}
	entry.diskTouched = old
	diagramRenderWithCache(t, second, "kitty", md)
	if info, err := os.Stat(cachePath); err != nil || !info.ModTime().After(old) {
		t.Fatal("active session did not update disk LRU", err)
	}
	if err := second.disk.clear(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, images = diagramRenderWithCache(t, second, "kitty", md)
	if len(images.ids) != 1 {
		t.Fatal("clearing disk cache removed the active session image")
	}
}

func TestMermaidDiskLRUCapacityAndNoExpiry(t *testing.T) {
	ctx := context.Background()
	disk := &mermaidDiskCache{root: t.TempDir(), limit: defaultMermaidCacheBytes, renderer: "test"}
	path := diskTestPNG(t)
	var total, middleSize int64
	for i, key := range []string{"a", "b", "c"} {
		publishDiskTest(t, disk, key, path)
		dir := filepath.Join(disk.root, disk.key(key))
		old := time.Now().Add(-time.Duration(100-i) * 24 * time.Hour)
		if err := os.Chtimes(dir, old, old); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"diagram.png", "metadata.json"} {
			info, err := os.Stat(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			total += info.Size()
			if key == "b" {
				middleSize += info.Size()
			}
		}
	}
	if err := disk.prune(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, hit, err := disk.load(ctx, "a"); err != nil || !hit {
		t.Fatal("old cached images should remain below the limit", err)
	}
	disk.limit = total - middleSize
	if err := disk.prune(ctx); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"a", "c"} {
		if _, _, hit, err := disk.load(ctx, key); err != nil || !hit {
			t.Fatal("recent entry unexpectedly evicted", key, err)
		}
	}
	if _, _, hit, err := disk.load(ctx, "b"); err != nil || hit {
		t.Fatal("least recently used image was retained", err)
	}
	disk.limit = 1
	publishDiskTest(t, disk, "oversize", path)
	if _, _, hit, err := disk.load(ctx, "oversize"); err != nil || hit {
		t.Fatal("oversized artifact persisted", err)
	}
}

func TestMermaidDiskKeyAndCorruption(t *testing.T) {
	ctx := context.Background()
	disk := &mermaidDiskCache{root: t.TempDir(), limit: defaultMermaidCacheBytes, renderer: "test"}
	path := diskTestPNG(t)
	key := mermaidImageKey("graph LR\r\nA-->B\r\n", "dark")
	if key != mermaidImageKey("graph LR\nA-->B\n", "dark") {
		t.Fatal("line endings changed cache identity")
	}
	publishDiskTest(t, disk, key, path)
	for _, other := range []string{mermaidImageKey("graph LR\nA-->C\n", "dark"), mermaidImageKey("graph LR\nA-->B\n", "light")} {
		if _, _, hit, err := disk.load(ctx, other); err != nil || hit {
			t.Fatal("source or theme changes must invalidate the image", err)
		}
	}
	other := *disk
	other.renderer = "new-version"
	if _, _, hit, err := other.load(ctx, key); err != nil || hit {
		t.Fatal("renderer changes must invalidate the image", err)
	}
	for _, name := range []string{"diagram.png", "metadata.json"} {
		publishDiskTest(t, disk, key, path)
		dir := filepath.Join(disk.root, disk.key(key))
		if err := os.WriteFile(filepath.Join(dir, name), []byte("corrupt"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, hit, err := disk.load(ctx, key); err != nil || hit {
			t.Fatal("corrupt cache should be a recoverable miss", name, err)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("corrupt artifact not removed", err)
		}
	}
}

func TestMermaidDiskClearAndAbandonedWrites(t *testing.T) {
	ctx := context.Background()
	disk := &mermaidDiskCache{root: t.TempDir(), limit: defaultMermaidCacheBytes, renderer: "test"}
	path := diskTestPNG(t)
	publishDiskTest(t, disk, "ready", path)
	epoch, err := disk.epoch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".work-old", ".work-recent", "keep"} {
		if err := os.Mkdir(filepath.Join(disk.root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(disk.root, ".work-old"), old, old); err != nil {
		t.Fatal(err)
	}
	if err := disk.prune(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(disk.root, ".work-old")); !os.IsNotExist(err) {
		t.Fatal("abandoned write retained", err)
	}
	if _, err := os.Stat(filepath.Join(disk.root, ".work-recent")); err != nil {
		t.Fatal("recent staging directory removed", err)
	}
	var output bytes.Buffer
	if err := Run([]string{"--mermaid-cache-dir", disk.root, "--clear-mermaid-cache"}, &output); err != nil || !strings.Contains(output.String(), "已清理") {
		t.Fatal("clear command failed", output.String(), err)
	}
	if err := disk.publish(ctx, "queued", path, 3, epoch); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"ready", "queued"} {
		if _, _, hit, err := disk.load(ctx, key); err != nil || hit {
			t.Fatal("clear was undone by an older job", key, err)
		}
	}
	if _, err := os.Stat(filepath.Join(disk.root, "keep")); err != nil {
		t.Fatal("unknown files removed", err)
	}
}

func TestMermaidDiskConcurrentRenderAndLockCancellation(t *testing.T) {
	root := t.TempDir()
	countPath := filepath.Join(t.TempDir(), "count")
	t.Setenv("MDCLI_MMDC_COUNT", countPath)
	first, second := helperMermaidCache(t), helperMermaidCache(t)
	first.configureDisk(root, defaultMermaidCacheBytes)
	second.configureDisk(root, defaultMermaidCacheBytes)
	source := "graph LR\nA-->B\n"
	key := first.disk.key(mermaidImageKey(source, "dark"))
	lock, err := first.disk.renderLock(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	md := "```mermaid\n" + source + "```"
	diagramRenderWithCache(t, first, "kitty", md)
	diagramRenderWithCache(t, second, "kitty", md)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := second.disk.renderLock(ctx, key); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("lock wait ignored cancellation", err)
	}
	lock.Close()
	awaitMermaid(t, first)
	awaitMermaid(t, second)
	raw, err := os.ReadFile(countPath)
	if err != nil || string(raw) != "render\n" {
		t.Fatal("concurrent sessions launched duplicate renders", string(raw), err)
	}
	if first.progress([]string{mermaidImageKey(source, "dark")}).hits+second.progress([]string{mermaidImageKey(source, "dark")}).hits != 1 {
		t.Fatal("waiting session did not record its disk hit")
	}
}

func TestMermaidDiskClearQueuedAndDisabled(t *testing.T) {
	c := helperMermaidCache(t)
	c.configureDisk(t.TempDir(), defaultMermaidCacheBytes)
	c.workers <- struct{}{}
	c.workers <- struct{}{}
	md := "```mermaid\ngraph LR\nA-->B\n```"
	diagramRenderWithCache(t, c, "kitty", md)
	if err := c.disk.clear(context.Background()); err != nil {
		t.Fatal(err)
	}
	<-c.workers
	<-c.workers
	awaitMermaid(t, c)
	key := mermaidImageKey("graph LR\nA-->B\n", "dark")
	if _, _, hit, err := c.disk.load(context.Background(), key); err != nil || hit {
		t.Fatal("a queued job repopulated the cleared cache", err)
	}
	_, images := diagramRenderWithCache(t, c, "kitty", md)
	if len(images.ids) != 1 {
		t.Fatal("cache clear affected rendering")
	}
	for _, limit := range []int64{0, 1} {
		disabled := helperMermaidCache(t)
		root := filepath.Join(t.TempDir(), "cache")
		disabled.configureDisk(root, limit)
		diagramRenderWithCache(t, disabled, "kitty", md)
		awaitMermaid(t, disabled)
		_, images := diagramRenderWithCache(t, disabled, "kitty", md)
		if len(images.ids) != 1 {
			t.Fatal("session rendering requires persistence", limit)
		}
		if limit == 0 {
			if _, err := os.Stat(root); !os.IsNotExist(err) {
				t.Fatal("disabled cache created a directory", err)
			}
		} else if _, _, hit, err := disabled.disk.load(context.Background(), key); err != nil || hit {
			t.Fatal("single image exceeds disk budget", err)
		}
	}
	var output bytes.Buffer
	if err := Run([]string{"--mermaid-cache-mib", "-1"}, &output); err == nil {
		t.Fatal("negative cache budget accepted")
	}
}

func TestMermaidDiskUnavailableAndPrintIndependent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(root, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	c := helperMermaidCache(t)
	c.configureDisk(root, defaultMermaidCacheBytes)
	if c.disk != nil || c.cacheProblem == "" {
		t.Fatal("unusable cache should disable persistence with a status hint")
	}
	md := "```mermaid\ngraph LR\nA-->B\n```"
	diagramRenderWithCache(t, c, "kitty", md)
	awaitMermaid(t, c)
	_, images := diagramRenderWithCache(t, c, "kitty", md)
	if len(images.ids) != 1 {
		t.Fatal("unavailable disk cache prevented image generation")
	}
	file := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(file, []byte(md), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	var output bytes.Buffer
	if err := Run([]string{"--print", file}, &output); err != nil {
		t.Fatal("print mode depends on the disk cache", err)
	}
}

func TestMermaidRendererIdentityInvalidation(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "dist")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "cli.js")
	if err := os.WriteFile(binary, []byte("mmdc"), 0600); err != nil {
		t.Fatal(err)
	}
	first := mermaidRendererIdentity(binary)
	if first != mermaidRendererIdentity(binary) {
		t.Fatal("renderer identity unstable")
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"@mermaid-js/mermaid-cli","version":"2.0"}`), 0600); err != nil {
		t.Fatal(err)
	}
	second := mermaidRendererIdentity(binary)
	if first == second {
		t.Fatal("npm version changes did not invalidate the cache")
	}
	t.Setenv("PUPPETEER_EXECUTABLE_PATH", "/other/chrome")
	if second == mermaidRendererIdentity(binary) {
		t.Fatal("browser override did not invalidate the cache")
	}
}

func TestMermaidDiskDamagedEpochRecovery(t *testing.T) {
	disk := &mermaidDiskCache{root: t.TempDir(), limit: defaultMermaidCacheBytes, renderer: "test"}
	if err := os.WriteFile(filepath.Join(disk.root, "epoch"), []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	publishDiskTest(t, disk, "recovered", diskTestPNG(t))
	if _, _, hit, err := disk.load(context.Background(), "recovered"); err != nil || !hit {
		t.Fatal("damaged clear marker disabled persistence", err)
	}
}
