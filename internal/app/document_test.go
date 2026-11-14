package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocumentRoundTrip(t *testing.T) {
	for _, content := range []string{"# 中文\n\n```go\n\tfmt.Println(1)\n```\n", "\ufeff# 标题\r\n\tcode\r\n", "no trailing newline", "", "\n\n"} {
		t.Run(content, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "note.md")
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			d, err := openDocument(path)
			if err != nil {
				t.Fatal(err)
			}
			e := newEditor(d.text)
			if err := d.save(e.text()); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != content {
				t.Fatalf("文件内容变化：%q != %q", got, content)
			}
		})
	}
}

func TestSaveProtectsExternalChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	d, err := openDocument(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("external"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := d.save("mine"); err == nil {
		t.Fatal("应拒绝覆盖外部修改")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "external" {
		t.Fatal("外部内容被覆盖")
	}
	if d.text != "original" {
		t.Fatal("失败的保存不应更新已保存快照")
	}
}

func TestNewDocumentDoesNotWriteUntilSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.md")
	d, err := openDocument(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("打开时不应创建文件")
	}
	if err := d.save("新文件\n"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "新文件\n" {
		t.Fatalf("got %q", got)
	}
}

func TestRejectInvalidDocuments(t *testing.T) {
	for _, content := range []string{"\xff", "a\x00b", "a\r\nb\nc", strings.Repeat("a", maxDocumentBytes+1)} {
		path := filepath.Join(t.TempDir(), "invalid.md")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := openDocument(path); err == nil {
			t.Fatal("应报告不支持的内容")
		}
	}
}
