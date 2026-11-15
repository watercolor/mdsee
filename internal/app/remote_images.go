package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const remoteImageCacheBytes = 32 * 1024 * 1024

type remoteImageEntry struct {
	data     []byte
	err      error
	loading  bool
	finished time.Time
	used     uint64
}

type remoteImageCache struct {
	mu           sync.Mutex
	entries      map[string]*remoteImageEntry
	bytes, limit int
	clock        uint64
	client       *http.Client
	ctx          context.Context
	cancel       context.CancelFunc
	workers      chan struct{}
	updates      chan struct{}
}

func newRemoteImageCache() *remoteImageCache {
	ctx, cancel := context.WithCancel(context.Background())
	return &remoteImageCache{
		entries: make(map[string]*remoteImageEntry), limit: remoteImageCacheBytes,
		client: &http.Client{Timeout: 8 * time.Second}, ctx: ctx, cancel: cancel,
		workers: make(chan struct{}, 4), updates: make(chan struct{}, 1),
	}
}

func remoteImageURL(destination string) (string, bool, error) {
	u, err := url.Parse(destination)
	if err != nil {
		return "", false, fmt.Errorf("图片 URL 无效")
	}
	if u.Scheme == "" && strings.HasPrefix(destination, "//") {
		u.Scheme = "https"
	}
	if !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		return "", false, nil
	}
	if u.Hostname() == "" {
		return "", true, fmt.Errorf("网络图片 URL 缺少主机名")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Fragment = ""
	return u.String(), true, nil
}

// Completed downloads are shared by render jobs, including after pane resizing.
func (c *remoteImageCache) lookup(address string) ([]byte, error, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clock++
	if entry, ok := c.entries[address]; ok {
		if entry.err == nil || time.Since(entry.finished) < 30*time.Second {
			entry.used = c.clock
			return entry.data, entry.err, entry.loading
		}
		c.remove(address)
	}
	if c.ctx.Err() != nil {
		return nil, fmt.Errorf("图片下载已取消"), false
	}
	for len(c.entries) >= maxPreviewImages {
		if !c.evict("") {
			return nil, fmt.Errorf("图片下载队列已满"), false
		}
	}
	entry := &remoteImageEntry{loading: true, used: c.clock}
	c.entries[address] = entry
	go c.load(address, entry)
	return nil, nil, true
}

func (c *remoteImageCache) remove(address string) {
	c.bytes -= len(c.entries[address].data)
	delete(c.entries, address)
}

// The caller holds mu. Downloads in progress are never evicted.
func (c *remoteImageCache) evict(except string) bool {
	oldest := ""
	var used uint64
	for address, entry := range c.entries {
		if address == except || entry.loading {
			continue
		}
		if oldest == "" || entry.used < used {
			oldest, used = address, entry.used
		}
	}
	if oldest == "" {
		return false
	}
	c.remove(oldest)
	return true
}

func (c *remoteImageCache) load(address string, entry *remoteImageEntry) {
	select {
	case c.workers <- struct{}{}:
		defer func() { <-c.workers }()
	case <-c.ctx.Done():
		return
	}
	data, err := c.download(address)
	c.mu.Lock()
	entry.loading = false
	entry.finished = time.Now()
	if err == nil {
		// Keep active document entries stable across automatic redraws. Evicting
		// one here would cause the next redraw to download it again indefinitely.
		if c.bytes+len(data) > c.limit {
			err = fmt.Errorf("网络图片缓存空间不足")
		} else {
			entry.data = data
			c.bytes += len(data)
		}
	}
	entry.err = err
	c.mu.Unlock()
	select {
	case c.updates <- struct{}{}:
	default:
	}
}

func (c *remoteImageCache) download(address string) ([]byte, error) {
	req, err := http.NewRequestWithContext(c.ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, fmt.Errorf("网络图片 URL 无效")
	}
	req.Header.Set("User-Agent", "mdsee")
	req.Header.Set("Accept", "image/png,image/jpeg,image/gif,image/*;q=0.8")
	response, err := c.client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("网络图片下载超时")
		}
		return nil, fmt.Errorf("网络图片下载失败")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("网络图片下载失败（HTTP %d）", response.StatusCode)
	}
	if response.ContentLength > maxImageBytes {
		return nil, fmt.Errorf("图片超过 10 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxImageBytes+1))
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("网络图片下载超时")
		}
		return nil, fmt.Errorf("网络图片下载中断")
	}
	if len(data) > maxImageBytes {
		return nil, fmt.Errorf("图片超过 10 MiB")
	}
	if _, err := decodeImage(data); err != nil {
		return nil, err
	}
	return data, nil
}
