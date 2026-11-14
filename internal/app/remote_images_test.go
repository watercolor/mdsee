package app

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
)

func awaitRemoteImage(t *testing.T, cache *remoteImageCache) {
	t.Helper()
	select {
	case <-cache.updates:
	case <-time.After(3 * time.Second):
		t.Fatal("图片下载未完成")
	}
}

func TestRemoteImageDownloadRenderAndReuse(t *testing.T) {
	path := imageFixture(t)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	release := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Query().Get("size") != "small" {
			t.Error("图片 URL 查询参数丢失")
		}
		<-release
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(raw)
	}))
	defer server.Close()
	cache := newRemoteImageCache()
	defer cache.cancel()
	cache.client = server.Client()
	source := "# 前文\n\n![网络图片](" + server.URL + "/photo.png?size=small#view)\n\n后续正文\n"
	render := func(width int) string {
		r := newImageRenderer("blocks", path, "dark", width, 1, 0.5)
		r.remote = cache
		content, _, err := renderMappedMarkdown(source, width, "dark", r)
		if err != nil {
			t.Fatal(err)
		}
		return content
	}
	initial := render(60)
	if !strings.Contains(initial, "下载中") || !strings.Contains(initial, "后续正文") {
		t.Fatal("下载应在后台进行，并先展示正文")
	}
	close(release)
	awaitRemoteImage(t, cache)
	for _, width := range []int{60, 30, 80} {
		content := render(width)
		if !strings.Contains(content, "▀") || strings.Contains(content, "下载中") {
			t.Fatal("完成下载后应渲染图片")
		}
	}
	if requests.Load() != 1 {
		t.Fatal("窗口变化及滚动映射应复用网络缓存", requests.Load())
	}
}

func TestRemoteImageRedirectFailuresAndLimits(t *testing.T) {
	path := imageFixture(t)
	raw, _ := os.ReadFile(path)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "/valid", http.StatusFound)
		case "/valid":
			_, _ = w.Write(raw)
		case "/missing":
			http.NotFound(w, r)
		case "/bad":
			_, _ = io.WriteString(w, "not an image")
		case "/large":
			w.Header().Set("Content-Length", "10485761")
		case "/slow":
			<-r.Context().Done()
		}
	}))
	defer server.Close()
	for _, tc := range []struct{ path, want string }{{"/redirect", ""}, {"/missing", "404"}, {"/bad", "不支持或损坏"}, {"/large", "10 MiB"}, {"/slow", "超时"}} {
		t.Run(tc.path, func(t *testing.T) {
			cache := newRemoteImageCache()
			defer cache.cancel()
			cache.client.Timeout = 50 * time.Millisecond
			address := server.URL + tc.path
			_, _, loading := cache.lookup(address)
			if !loading {
				t.Fatal("应异步下载")
			}
			awaitRemoteImage(t, cache)
			data, err, loading := cache.lookup(address)
			if loading {
				t.Fatal("完成后不应重复下载")
			}
			if tc.want == "" {
				if err != nil || !bytes.Equal(data, raw) {
					t.Fatal("重定向图片下载失败", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

func TestRemoteImageCacheLimitDoesNotRedownloadOnRefresh(t *testing.T) {
	path := imageFixture(t)
	raw, _ := os.ReadFile(path)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); _, _ = w.Write(raw) }))
	defer server.Close()
	cache := newRemoteImageCache()
	defer cache.cancel()
	cache.limit = len(raw)
	first, second := server.URL+"/one", server.URL+"/two"
	cache.lookup(first)
	awaitRemoteImage(t, cache)
	cache.lookup(second)
	awaitRemoteImage(t, cache)
	for i := 0; i < 5; i++ {
		if _, err, loading := cache.lookup(first); err != nil || loading {
			t.Fatal("已有缓存不应被刷新淘汰")
		}
		if _, err, loading := cache.lookup(second); err == nil || loading {
			t.Fatal("超出缓存容量应保留失败状态")
		}
	}
	if requests.Load() != 2 || cache.bytes > cache.limit {
		t.Fatal("缓存满时不能在自动刷新中反复下载")
	}
}

func TestRemoteImageCancellation(t *testing.T) {
	started, canceled := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(canceled)
	}))
	defer server.Close()
	cache := newRemoteImageCache()
	defer cache.cancel()
	cache.lookup(server.URL + "/waiting")
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("请求没有开始")
	}
	cache.cancel()
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("退出时未取消下载")
	}
}

type remoteImageProgramModel struct{ *model }

func (m remoteImageProgramModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := m.model.Update(msg)
	if rendered, ok := msg.(renderedMsg); ok && strings.Contains(rendered.content, "▀") {
		return m, tea.Quit
	}
	return m, cmd
}

func TestRemoteImagesRefreshThroughProgram(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TMUX", "session")
	path := imageFixture(t)
	raw, _ := os.ReadFile(path)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(raw) }))
	defer server.Close()
	d := &document{path: filepath.Join(filepath.Dir(path), "note.md"), text: "![网络图片](" + server.URL + "/image.png)\n\n正文"}
	m := newModel(d, previewMode, "dark")
	defer m.remoteImages.cancel()
	m.configureImages("blocks")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := tea.NewProgram(remoteImageProgramModel{m}, tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithContext(ctx), tea.WithWindowSize(80, 24), tea.WithColorProfile(colorprofile.TrueColor), tea.WithoutSignalHandler()).Run()
	if err != nil {
		t.Fatal("下载完成后预览未自动刷新", err)
	}
	if !strings.Contains(m.preview.GetContent(), "▀") {
		t.Fatal("图片未进入预览视口")
	}
}
