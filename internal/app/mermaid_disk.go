package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const defaultMermaidCacheBytes int64 = 256 << 20
const mermaidCacheRecipe = "png-transparent-density3-2-1-v1"

type mermaidDiskCache struct {
	root     string
	limit    int64
	renderer string
}

type mermaidDiskMetadata struct {
	Schema    int       `json:"schema"`
	Key       string    `json:"key"`
	SourceKey string    `json:"source_key"`
	Renderer  string    `json:"renderer"`
	Recipe    string    `json:"recipe"`
	Scale     int       `json:"scale"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	PNGHash   string    `json:"png_sha256"`
	Created   time.Time `json:"created"`
}

func defaultMermaidCacheDir() (string, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "mdcli", "mermaid", "v1"), nil
}

// Resolve renderer identity without starting Node on cache hits. npm package
// metadata covers version changes; the executable stamp covers other installers.
func mermaidRendererIdentity(binary string) string {
	resolved, err := filepath.EvalSymlinks(binary)
	if err != nil {
		resolved = binary
	}
	identity := resolved
	if info, err := os.Stat(resolved); err == nil {
		identity += fmt.Sprintf("\x00%d\x00%d", info.Size(), info.ModTime().UnixNano())
	}
	dir := filepath.Dir(resolved)
	for i := 0; i < 5; i++ {
		raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
		if err == nil {
			var pkg struct{ Name, Version string }
			if json.Unmarshal(raw, &pkg) == nil && pkg.Name == "@mermaid-js/mermaid-cli" {
				identity += "\x00" + string(raw)
				break
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	// An explicit browser override can change the rendered result.
	identity += "\x00" + os.Getenv("PUPPETEER_EXECUTABLE_PATH")
	return fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
}

func (d *mermaidDiskCache) key(sourceKey string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(mermaidCacheRecipe+"\x00"+d.renderer+"\x00"+sourceKey)))
}

func cacheHashName(name string) bool {
	if len(name) != 64 {
		return false
	}
	for _, r := range name {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

func (d *mermaidDiskCache) lock(ctx context.Context) (*os.File, error) {
	if err := os.MkdirAll(d.root, 0700); err != nil {
		return nil, err
	}
	return lockMermaidFile(ctx, filepath.Join(d.root, "cache.lock"))
}

func (d *mermaidDiskCache) renderLock(ctx context.Context, key string) (*os.File, error) {
	dir := filepath.Join(d.root, "locks")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	// A fixed set of lock shards avoids accumulating one lock file per old image.
	return lockMermaidFile(ctx, filepath.Join(dir, key[:2]+".lock"))
}

func (d *mermaidDiskCache) epochLocked() (string, error) {
	path := filepath.Join(d.root, "epoch")
	if data, err := os.ReadFile(path); err == nil {
		token, decodeErr := hex.DecodeString(string(data))
		if decodeErr == nil && len(token) == 16 {
			return string(data), nil
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	return d.newEpochLocked()
}

func (d *mermaidDiskCache) newEpochLocked() (string, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	value := fmt.Sprintf("%x", token)
	if err := os.WriteFile(filepath.Join(d.root, "epoch"), []byte(value), 0600); err != nil {
		return "", err
	}
	return value, nil
}

func (d *mermaidDiskCache) epoch(ctx context.Context) (string, error) {
	lock, err := d.lock(ctx)
	if err != nil {
		return "", err
	}
	defer lock.Close()
	return d.epochLocked()
}

func (d *mermaidDiskCache) load(ctx context.Context, sourceKey string) ([]byte, mermaidDiskMetadata, bool, error) {
	var meta mermaidDiskMetadata
	lock, err := d.lock(ctx)
	if err != nil {
		return nil, meta, false, err
	}
	defer lock.Close()
	key := d.key(sourceKey)
	dir := filepath.Join(d.root, key)
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return nil, meta, false, nil
	}
	if err != nil {
		return nil, meta, false, err
	}
	invalid := func() ([]byte, mermaidDiskMetadata, bool, error) {
		return nil, meta, false, os.RemoveAll(dir)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return invalid()
	}
	raw, err := readCacheFile(filepath.Join(dir, "metadata.json"), 8192)
	if err != nil || json.Unmarshal(raw, &meta) != nil ||
		meta.Schema != 1 || meta.Key != key || meta.SourceKey != sourceKey ||
		meta.Renderer != d.renderer || meta.Recipe != mermaidCacheRecipe || meta.Scale < 1 || meta.Scale > 3 {
		return invalid()
	}
	data, err := readCacheFile(filepath.Join(dir, "diagram.png"), maxImageBytes)
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(data)) != meta.PNGHash {
		return invalid()
	}
	// Publication validates the pixels once. The stored checksum verifies those
	// same PNG bytes on reuse, so only dimensions need reading on the hit path.
	bounds, err := pngImageBounds(data)
	if err != nil || bounds.Dx() != meta.Width || bounds.Dy() != meta.Height {
		return invalid()
	}
	now := time.Now()
	if err := os.Chtimes(dir, now, now); err != nil {
		return nil, meta, false, err
	}
	return data, meta, true, nil
}

func readCacheFile(path string, maxBytes int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxBytes {
		return nil, fmt.Errorf("缓存文件无效")
	}
	return os.ReadFile(path)
}

func (d *mermaidDiskCache) touch(ctx context.Context, sourceKey string) error {
	lock, err := d.lock(ctx)
	if err != nil {
		return err
	}
	defer lock.Close()
	now := time.Now()
	err = os.Chtimes(filepath.Join(d.root, d.key(sourceKey)), now, now)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (d *mermaidDiskCache) publish(ctx context.Context, sourceKey, path string, scale int, epoch string) error {
	data, err := readCacheFile(path, maxImageBytes)
	if err != nil {
		return err
	}
	img, err := decodeImage(data)
	if err != nil {
		return err
	}
	key := d.key(sourceKey)
	meta := mermaidDiskMetadata{
		Schema: 1, Key: key, SourceKey: sourceKey,
		Renderer: d.renderer, Recipe: mermaidCacheRecipe, Scale: scale,
		Width: img.Bounds().Dx(), Height: img.Bounds().Dy(),
		PNGHash: fmt.Sprintf("%x", sha256.Sum256(data)), Created: time.Now().UTC(),
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	// A single artifact above the configured limit remains session-only.
	if int64(len(data)+len(raw)) > d.limit {
		return nil
	}
	lock, err := d.lock(ctx)
	if err != nil {
		return err
	}
	defer lock.Close()
	current, err := d.epochLocked()
	if err != nil {
		return err
	}
	if current != epoch {
		// A manual clear must not be undone by an in-flight job.
		return nil
	}
	stage, err := os.MkdirTemp(d.root, ".work-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := os.WriteFile(filepath.Join(stage, "diagram.png"), data, 0600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stage, "metadata.json"), raw, 0600); err != nil {
		return err
	}
	destination := filepath.Join(d.root, key)
	if err := os.RemoveAll(destination); err != nil {
		return err
	}
	if err := os.Rename(stage, destination); err != nil {
		return err
	}
	return d.pruneLocked()
}

func (d *mermaidDiskCache) prune(ctx context.Context) error {
	lock, err := d.lock(ctx)
	if err != nil {
		return err
	}
	defer lock.Close()
	return d.pruneLocked()
}

func (d *mermaidDiskCache) pruneLocked() error {
	entries, err := os.ReadDir(d.root)
	if err != nil {
		return err
	}
	type item struct {
		path   string
		size   int64
		access time.Time
	}
	var items []item
	var total int64
	for _, entry := range entries {
		path := filepath.Join(d.root, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if strings.HasPrefix(entry.Name(), ".work-") && time.Since(info.ModTime()) > time.Hour {
			if err := os.RemoveAll(path); err != nil {
				return err
			}
			continue
		}
		if !cacheHashName(entry.Name()) {
			continue
		}
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			if err := os.RemoveAll(path); err != nil {
				return err
			}
			continue
		}
		var size int64
		valid := true
		for _, name := range []string{"diagram.png", "metadata.json"} {
			f, err := os.Lstat(filepath.Join(path, name))
			if err != nil || !f.Mode().IsRegular() {
				valid = false
				break
			}
			size += f.Size()
		}
		if !valid {
			if err := os.RemoveAll(path); err != nil {
				return err
			}
			continue
		}
		total += size
		items = append(items, item{path, size, info.ModTime()})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].access.Equal(items[j].access) {
			return items[i].path < items[j].path
		}
		return items[i].access.Before(items[j].access)
	})
	for _, item := range items {
		if total <= d.limit {
			break
		}
		if err := os.RemoveAll(item.path); err != nil {
			return err
		}
		total -= item.size
	}
	return nil
}

func (d *mermaidDiskCache) clear(ctx context.Context) error {
	lock, err := d.lock(ctx)
	if err != nil {
		return err
	}
	defer lock.Close()
	if _, err := d.newEpochLocked(); err != nil {
		return err
	}
	entries, err := os.ReadDir(d.root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if cacheHashName(entry.Name()) || strings.HasPrefix(entry.Name(), ".work-") {
			if err := os.RemoveAll(filepath.Join(d.root, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}
