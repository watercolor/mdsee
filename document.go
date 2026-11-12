package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxDocumentBytes = 2 * 1024 * 1024

type document struct {
	path, text, newline string
	bom, exists         bool
	diskHash            [32]byte
	mode                os.FileMode
}

func openDocument(path string) (*document, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	d := &document{path: abs, newline: "\n", mode: 0644}
	info, err := os.Stat(abs)
	if os.IsNotExist(err) {
		return d, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("不是普通文件：%s", path)
	}
	if info.Size() > maxDocumentBytes {
		return nil, fmt.Errorf("文件超过 2 MiB 编辑限制")
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	d.path = resolved
	raw, err := os.ReadFile(d.path)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(raw) {
		return nil, fmt.Errorf("仅支持 UTF-8 文件")
	}
	d.exists, d.diskHash, d.mode = true, sha256.Sum256(raw), info.Mode().Perm()
	d.bom = bytes.HasPrefix(raw, []byte{0xef, 0xbb, 0xbf})
	content := strings.TrimPrefix(string(raw), "\ufeff")
	if strings.Contains(content, "\r\n") {
		if strings.Contains(strings.ReplaceAll(content, "\r\n", ""), "\n") {
			return nil, fmt.Errorf("文件混用了 LF 和 CRLF，请先统一换行格式")
		}
		d.newline = "\r\n"
		content = strings.ReplaceAll(content, "\r\n", "\n")
	}
	for _, r := range content {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return nil, fmt.Errorf("文件包含不支持的控制字符 U+%04X", r)
		}
	}
	d.text = content
	return d, nil
}

func (d *document) save(text string) error {
	current, err := os.ReadFile(d.path)
	if d.exists {
		if err != nil {
			return fmt.Errorf("读取原文件：%w", err)
		}
		if sha256.Sum256(current) != d.diskHash {
			return fmt.Errorf("文件已被外部修改，未覆盖；请保留当前内容后重新打开")
		}
	} else if err == nil {
		return fmt.Errorf("目标文件已被其他程序创建，未覆盖")
	} else if !os.IsNotExist(err) {
		return err
	}
	data := strings.ReplaceAll(text, "\n", d.newline)
	if d.bom {
		data = "\ufeff" + data
	}
	f, err := os.CreateTemp(filepath.Dir(d.path), ".mdcli-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(d.mode); err != nil {
		f.Close()
		return err
	}
	if _, err = f.WriteString(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), d.path); err != nil {
		return err
	}
	d.text, d.exists, d.diskHash = text, true, sha256.Sum256([]byte(data))
	return nil
}
