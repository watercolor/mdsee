package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

func imageFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "图片 test.png")
	img := image.NewNRGBA(image.Rect(0, 0, 32, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			c := color.NRGBA{R: 255, A: 255}
			if y >= 8 {
				c = color.NRGBA{B: 255, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func imageRender(t *testing.T, path, source, mode string, width int) (string, []scrollAnchor, *imageRenderer) {
	t.Helper()
	r := newImageRenderer(mode, filepath.Join(filepath.Dir(path), "note.md"), "dark", width, 0x123400, 0.5)
	content, anchors, err := renderMappedMarkdown(source, width, "dark", r)
	if err != nil {
		t.Fatal(err)
	}
	return content, anchors, r
}

func TestLocalImagePaths(t *testing.T) {
	path := imageFixture(t)
	base := filepath.Dir(path)
	u := (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
	for _, destination := range []string{filepath.Base(path), "./" + filepath.Base(path), url.PathEscape(filepath.Base(path)), path, u} {
		got, err := localImagePath(base, destination)
		if err != nil || got != path {
			t.Errorf("%q: got %q, %v", destination, got, err)
		}
	}
	for _, destination := range []string{"https://example.com/a.png", "//example.com/a.png", "file://example.com/a.png", "data:image/png;base64,a", "file:relative.png", "", "%2f%2fexample.com/a.png", `\\server\image.png`} {
		if _, err := localImagePath(base, destination); err == nil {
			t.Errorf("accepted %q", destination)
		}
	}
}

func TestImagesRenderAndPreserveCodeAndInlineContent(t *testing.T) {
	path := imageFixture(t)
	name := url.PathEscape(filepath.Base(path))
	source := "# 前文\n\n![红蓝测试](" + name + ")\n\n## 后文\n\n正文。\n"
	content, anchors, r := imageRender(t, path, source, "blocks", 60)
	if !strings.Contains(content, "▀") || !strings.Contains(content, "38;2;255;0;0m") || !strings.Contains(content, "38;2;0;0;255m") {
		t.Fatalf("missing image colors: %q", content)
	}
	if len(r.cache) != 1 {
		t.Fatalf("map probes should reuse images, cache size %d", len(r.cache))
	}
	plain := ansi.Strip(content)
	if !strings.Contains(plain, "红蓝测试") || !strings.Contains(plain, "后文") || strings.Contains(plain, "MI0000Z") {
		t.Fatal(plain)
	}
	imageRow, headingRow := -1, -1
	for i, line := range strings.Split(plain, "\n") {
		if strings.Contains(line, "图片：") {
			imageRow = i
		}
		if strings.Contains(line, "后文") {
			headingRow = i
		}
		if ansi.StringWidth(line) > 60 {
			t.Fatalf("line exceeds preview width: %q", line)
		}
	}
	if got := mappedLine(anchors, 2, false); got != imageRow {
		t.Fatalf("image anchor %d, want %d", got, imageRow)
	}
	if got := mappedLine(anchors, 4, false); got != headingRow {
		t.Fatalf("heading anchor %d, want %d", got, headingRow)
	}
	for _, code := range []string{"`![图](" + name + ")`", "```md\n![图](" + name + ")\n```", "    ![图](" + name + ")\n", "正文 ![图](" + name + ") 后续", "| 图片 |\n| --- |\n| ![图](" + name + ") |"} {
		content, _, r := imageRender(t, path, code, "blocks", 40)
		if len(r.cache) != 0 || strings.Contains(content, "▀") {
			t.Fatalf("expanded inline/code image: %q", code)
		}
	}
}

func TestImageReferencesNestedAndDuplicate(t *testing.T) {
	path := imageFixture(t)
	name := url.PathEscape(filepath.Base(path))
	for _, source := range []string{
		"![引用][pic]\n\n[pic]: " + name,
		"> ![引用](" + name + ")\n",
		"- ![列表](" + name + ")\n",
		"[![链接](" + name + ")](https://example.com)\n",
		"![第一张](" + name + ")\n\n![第二张](" + name + ")\n",
	} {
		content, _, r := imageRender(t, path, source, "kitty", 40)
		if !strings.ContainsRune(content, kitty.Placeholder) || len(r.ids) != 1 {
			t.Fatalf("image not rendered once: %q; %q", source, content)
		}
		if !strings.Contains(r.graphics.String(), "U=1") {
			t.Fatal("missing virtual placement")
		}
	}
}

func TestImageFailuresAndUnsupportedSources(t *testing.T) {
	path := imageFixture(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	bad := filepath.Join(filepath.Dir(path), "bad.png")
	if err := os.WriteFile(bad, []byte("not an image"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ destination, message string }{
		{"ftp://example.com/a.png", "图片协议"}, {"missing.png", "无法读取图片"},
		{"bad.png", "不支持或损坏"}, {".", "普通文件"},
	} {
		content, _, _ := imageRender(t, path, "![图]("+tc.destination+")\n\n后续正文", "blocks", 80)
		if !strings.Contains(ansi.Strip(content), tc.message) || !strings.Contains(content, "后续正文") {
			t.Fatal(content)
		}
	}
	imageRender(t, path, "![图]("+server.URL+"/a.png)", "off", 80)
	if requests.Load() != 0 {
		t.Fatal("off mode sent a network request")
	}
	f, err := os.Create(filepath.Join(filepath.Dir(path), "big.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxImageBytes + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err := readLocalImage(f.Name()); err == nil || !strings.Contains(err.Error(), "10 MiB") {
		t.Fatal(err)
	}
	// A GIF header can advertise large dimensions without allocating the pixels.
	header := make([]byte, 13)
	copy(header, "GIF89a")
	binary.LittleEndian.PutUint16(header[6:], 6000)
	binary.LittleEndian.PutUint16(header[8:], 6000)
	header[10] = 0
	large := filepath.Join(filepath.Dir(path), "pixels.gif")
	if err := os.WriteFile(large, header, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readLocalImage(large); err == nil || !strings.Contains(err.Error(), "800 万") {
		t.Fatal(err)
	}
}

func TestSupportedImageFormatsAndTransparency(t *testing.T) {
	dir := t.TempDir()
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for _, format := range []string{"png", "jpeg", "gif"} {
		var out bytes.Buffer
		var err error
		switch format {
		case "png":
			err = png.Encode(&out, img)
		case "jpeg":
			err = jpeg.Encode(&out, img, nil)
		case "gif":
			err = gif.Encode(&out, img, nil)
		}
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "test."+format)
		if err := os.WriteFile(path, out.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := readLocalImage(path); err != nil {
			t.Fatalf("%s: %v", format, err)
		}
	}
	bg := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	if got := sampleImage(img, 1, 1, bg).NRGBAAt(0, 0); got != bg {
		t.Fatalf("transparent image should composite on background: %#v", got)
	}
}

func TestImageLayoutAndViewportClipping(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	path := imageFixture(t)
	source := "# 图片测试\n\n![图](" + url.PathEscape(filepath.Base(path)) + ")\n\n" + strings.Repeat("后续正文\n\n", 30)
	for _, width := range []int{30, 50, 80, 121} {
		for _, kind := range []string{"blocks", "kitty"} {
			m := testModel(t)
			m.doc.path = filepath.Join(filepath.Dir(path), "note.md")
			m.editor = newEditor(source)
			m.configureImages(kind)
			for _, mode := range []viewMode{splitMode, previewMode, editMode} {
				m.mode = mode
				m.resize(width, 18)
				content, anchors, r := imageRender(t, path, source, kind, m.previewWidth)
				m.preview.SetContent(content)
				m.mapping, m.mapSource = anchors, source
				for _, offset := range []int{0, 5, 15, 1000} {
					m.preview.SetYOffset(offset)
					view := m.View().Content
					if len(strings.Split(view, "\n")) > 18 {
						t.Fatal("view exceeds terminal height")
					}
					for _, line := range strings.Split(view, "\n") {
						if ansi.StringWidth(line) > width {
							t.Fatalf("view exceeds width %d: %q", width, line)
						}
					}
					if m.actualMode() == editMode || offset == 1000 {
						if strings.ContainsRune(view, kitty.Placeholder) || strings.Contains(view, "▀") {
							t.Fatal("image escaped viewport")
						}
					}
				}
				if kind == "kitty" && len(r.ids) != 1 {
					t.Fatal("unexpected image IDs")
				}
			}
		}
	}
}

func TestKittyDetectionColorAndRenderLifecycle(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TMUX", "")
	m := testModel(t)
	response := uv.KittyGraphicsEvent{Options: kitty.Options{ID: imageProbeID, VirtualPlacement: true}, Payload: []byte("OK")}
	m.Update(response)
	if m.imageMode != "blocks" {
		t.Fatal("image IDs require truecolor")
	}
	m.Update(tea.ColorProfileMsg{Profile: colorprofile.TrueColor})
	if m.imageMode != "kitty" {
		t.Fatal("supported terminal should use Kitty")
	}
	m.Update(tea.ColorProfileMsg{Profile: colorprofile.ANSI256})
	if m.imageMode != "blocks" {
		t.Fatal("reduced color profile corrupts image IDs")
	}
	m.Update(tea.ColorProfileMsg{Profile: colorprofile.ASCII})
	if m.imageMode != "off" {
		t.Fatal("a monochrome terminal must use text instead of solid blocks")
	}
	m.Update(tea.ColorProfileMsg{Profile: colorprofile.TrueColor})
	if m.imageMode != "kitty" {
		t.Fatal("restored truecolor should restore image display")
	}
	m.configureImages("off")
	m.Update(response)
	if m.imageMode != "off" {
		t.Fatal("off mode should ignore detection")
	}
	m.configureImages("kitty")
	m.busy, m.revision = true, 3
	m.preview.SetContent("old")
	msg := renderedMsg{revision: 3, content: "new", graphics: "upload", imageIDs: []int{42}}
	_, cmd := m.Update(msg)
	if cmd == nil || m.preview.GetContent() != "old" || !m.busy {
		t.Fatal("must upload before exposing image placeholders")
	}
	m.Update(imagesReadyMsg{msg})
	if m.preview.GetContent() != "new" || m.busy {
		t.Fatal("uploaded preview was not installed")
	}
	m.revision = 4
	_, cmd = m.Update(imagesReadyMsg{msg})
	if cmd == nil || !m.busy {
		t.Fatal("stale image result should trigger a new render")
	}
	if got := deleteImageResources([]int{42}); !strings.Contains(got, "d=I,i=42") || strings.Contains(got, "d=A") {
		t.Fatal("cleanup must target only owned resources")
	}
}

func TestITermImageAutoDetection(t *testing.T) {
	for _, tc := range []struct {
		name, version, mode, tmux, noColor, theme, want string
		profile                                         colorprofile.Profile
	}{
		{"current", "iTerm2 3.7.7", "auto", "", "", "dark", "kitty", colorprofile.TrueColor},
		{"minimum", "iTerm2 3.7.3", "auto", "", "", "dark", "kitty", colorprofile.TrueColor},
		{"future", "iTerm2 3.10.0", "auto", "", "", "dark", "kitty", colorprofile.TrueColor},
		{"old", "iTerm2 3.7.2", "auto", "", "", "dark", "blocks", colorprofile.TrueColor},
		{"unknown", "iTerm2", "auto", "", "", "dark", "blocks", colorprofile.TrueColor},
		{"malformed", "iTerm2 4.0.invalid", "auto", "", "", "dark", "blocks", colorprofile.TrueColor},
		{"prerelease", "iTerm2 3.7.3beta1", "auto", "", "", "dark", "blocks", colorprofile.TrueColor},
		{"other terminal", "WezTerm 20261113", "auto", "", "", "dark", "blocks", colorprofile.TrueColor},
		{"tmux", "iTerm2 3.7.7", "auto", "session", "", "dark", "blocks", colorprofile.TrueColor},
		{"no color", "iTerm2 3.7.7", "auto", "", "1", "dark", "off", colorprofile.TrueColor},
		{"ascii theme", "iTerm2 3.7.7", "auto", "", "", "ascii", "off", colorprofile.TrueColor},
		{"256 colors", "iTerm2 3.7.7", "auto", "", "", "dark", "blocks", colorprofile.ANSI256},
		{"monochrome", "iTerm2 3.7.7", "auto", "", "", "dark", "off", colorprofile.ASCII},
		{"explicit blocks", "iTerm2 3.7.7", "blocks", "", "", "dark", "blocks", colorprofile.TrueColor},
		{"explicit off", "iTerm2 3.7.7", "off", "", "", "dark", "off", colorprofile.TrueColor},
	} {
		for _, versionFirst := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/versionFirst=%v", tc.name, versionFirst), func(t *testing.T) {
				t.Setenv("TMUX", tc.tmux)
				t.Setenv("NO_COLOR", tc.noColor)
				m := testModel(t)
				m.theme = tc.theme
				m.configureImages(tc.mode)
				version := tea.TerminalVersionMsg{Name: tc.version}
				profile := tea.ColorProfileMsg{Profile: tc.profile}
				if versionFirst {
					m.Update(version)
					m.Update(profile)
				} else {
					m.Update(profile)
					m.Update(version)
				}
				if m.imageMode != tc.want {
					t.Fatalf("got %s, want %s", m.imageMode, tc.want)
				}
			})
		}
	}
}

func TestITermAutoThroughBubbleTea(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TMUX", "")
	path := imageFixture(t)
	d := &document{path: filepath.Join(filepath.Dir(path), "note.md"), text: "![图](" + url.PathEscape(filepath.Base(path)) + ")"}
	m := newModel(d, previewMode, "dark")
	defer m.remoteImages.cancel()
	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := tea.NewProgram(imageProgramModel{m},
		tea.WithInput(strings.NewReader("\x1bP>|iTerm2 3.7.7\x1b\\")), tea.WithOutput(&out), tea.WithContext(ctx),
		tea.WithColorProfile(colorprofile.TrueColor), tea.WithWindowSize(80, 24), tea.WithoutSignalHandler()).Run()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), ansi.RequestNameVersion) || !strings.ContainsRune(out.String(), kitty.Placeholder) {
		t.Fatal("auto mode must query terminal version and render image placeholders")
	}
	decodeKittyUpload(t, out.String())
}

func TestImageRendererPreservesOtherMarkdown(t *testing.T) {
	source := "# 标题\n\n**正文** $E=mc^2$\n\n> 引用\n\n- 列表\n\n```go\nfmt.Println(1)\n```\n\n| A | B |\n| --- | --- |\n| C | D |\n"
	for _, theme := range []string{"dark", "light"} {
		want, err := renderMarkdown(source, 60, theme)
		if err != nil {
			t.Fatal(err)
		}
		r := newImageRenderer("blocks", "note.md", theme, 60, 1, 0.5)
		got, _, err := renderMappedMarkdown(source, 60, theme, r)
		if err != nil || want != got {
			t.Fatalf("image rendering changed other Markdown (%s): %v", theme, err)
		}
	}
}

func TestImagesOffNoColorAndPrint(t *testing.T) {
	path := imageFixture(t)
	source := "![图](" + url.PathEscape(filepath.Base(path)) + ")"
	content, _, r := imageRender(t, path, source, "off", 60)
	if len(r.cache) != 0 || strings.Contains(content, "▀") {
		t.Fatal("off mode decoded an image")
	}
	t.Setenv("NO_COLOR", "1")
	m := testModel(t)
	m.configureImages("kitty")
	if m.imageMode != "off" {
		t.Fatal("NO_COLOR must disable image colors")
	}
	t.Setenv("NO_COLOR", "")
	m.theme = "ascii"
	m.configureImages("blocks")
	if m.imageMode != "off" {
		t.Fatal("ascii theme must keep text output")
	}
	t.Setenv("TMUX", "session")
	m.theme = "dark"
	m.configureImages("kitty")
	m.Update(tea.ColorProfileMsg{Profile: colorprofile.TrueColor})
	if m.imageMode != "blocks" {
		t.Fatal("tmux should use the compatible block renderer")
	}
	doc := filepath.Join(filepath.Dir(path), "note.md")
	if err := os.WriteFile(doc, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Run([]string{"--print", "--images", "kitty", doc}, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "\x1b") || strings.Contains(out.String(), "▀") || strings.ContainsRune(out.String(), kitty.Placeholder) {
		t.Fatal("print must stay textual")
	}
	if err := Run([]string{"--images", "invalid", doc}, &out); err == nil {
		t.Fatal("invalid image mode accepted")
	}
}

type imageProgramModel struct{ *model }

func (m imageProgramModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := m.model.Update(msg)
	if _, ok := msg.(imagesReadyMsg); ok {
		return m, tea.Sequence(cmd, tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg { return tea.Quit() }))
	}
	return m, cmd
}

func TestKittyThroughBubbleTeaRenderer(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TMUX", "")
	path := imageFixture(t)
	d := &document{path: filepath.Join(filepath.Dir(path), "note.md"), text: "![图](" + url.PathEscape(filepath.Base(path)) + ")"}
	m := newModel(d, previewMode, "dark")
	defer m.remoteImages.cancel()
	m.configureImages("kitty")
	m.imageIDBase = 0x123456
	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := tea.NewProgram(imageProgramModel{m}, tea.WithInput(nil), tea.WithOutput(&out), tea.WithContext(ctx),
		tea.WithColorProfile(colorprofile.TrueColor), tea.WithWindowSize(80, 24), tea.WithoutSignalHandler()).Run()
	if err != nil {
		t.Fatal(err)
	}
	output := out.String()
	start := strings.Index(output, "\x1b_G")
	place := strings.IndexRune(output, kitty.Placeholder)
	if start < 0 || place < start || !strings.Contains(output, "U=1") {
		t.Fatal("missing transmission before rendered placeholders")
	}
	if !strings.Contains(output, "38;2;18;52;86m") {
		t.Fatal("Bubble Tea changed the image ID foreground color")
	}
	if !strings.Contains(output, string([]rune{kitty.Placeholder, kitty.Diacritic(0), kitty.Diacritic(1)})) {
		t.Fatal("Bubble Tea lost placeholder coordinates")
	}
	img := decodeKittyUpload(t, output)
	if img.Bounds().Dx() != 32 || img.Bounds().Dy() != 16 {
		t.Fatalf("unexpected transmitted dimensions: %v", img.Bounds())
	}
	for _, pixel := range []struct {
		y    int
		want color.NRGBA
	}{{0, color.NRGBA{R: 255, A: 255}}, {15, color.NRGBA{B: 255, A: 255}}} {
		if got := color.NRGBAModel.Convert(img.At(0, pixel.y)); got != pixel.want {
			t.Fatalf("transmitted pixel at y=%d: got %v, want %v", pixel.y, got, pixel.want)
		}
	}
}

func decodeKittyUpload(t *testing.T, output string) image.Image {
	t.Helper()
	var encoded strings.Builder
	started := false
	for _, part := range strings.Split(output, "\x1b_G")[1:] {
		command, _, ok := strings.Cut(part, "\x1b\\")
		if !ok {
			t.Fatal("unterminated Kitty command")
		}
		header, payload, _ := strings.Cut(command, ";")
		if !started {
			if !strings.Contains(header, "a=T") {
				continue
			}
			started = true
		}
		encoded.WriteString(payload)
		if !strings.Contains(header, "m=1") {
			break
		}
	}
	data, err := base64.StdEncoding.DecodeString(encoded.String())
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Kitty upload must contain a decodable PNG (%d bytes): %v", len(data), err)
	}
	return img
}

func TestKittyDiagramPreservesPixelsAndScrollableHeight(t *testing.T) {
	path := filepath.Join(t.TempDir(), "diagram.png")
	img := image.NewNRGBA(image.Rect(0, 0, 600, 1200))
	for y := 0; y < 1200; y++ {
		img.SetNRGBA(y%600, y, color.NRGBA{R: 255, G: 125, A: 255})
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
	file.Close()
	r := newImageRenderer("kitty", path, "dark", 80, 0x123456, .5)
	preview := r.renderPreview(path, 1)
	if preview.err != nil || len(preview.rows) <= 24 || len(preview.rows) > 256 {
		t.Fatalf("diagram must scroll beyond thumbnail height: %d %v", len(preview.rows), preview.err)
	}
	decoded := decodeKittyUpload(t, r.graphics.String())
	if decoded.Bounds() != img.Bounds() {
		t.Fatalf("diagram downsampled: %v", decoded.Bounds())
	}
	if got := color.NRGBAModel.Convert(decoded.At(599, 1199)); got != img.NRGBAAt(599, 1199) {
		t.Fatal("lost one-pixel diagram line")
	}
	ordinary := newImageRenderer("kitty", path, "dark", 80, 0x123456, .5)
	if rows := ordinary.preview(path).rows; len(rows) > 24 {
		t.Fatal("ordinary image layout changed")
	}
}

func TestMermaidDisplaySizeIndependentOfResolution(t *testing.T) {
	for _, density := range []int{1, 2, 3} {
		t.Run(fmt.Sprint(density), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "diagram.png")
			img := image.NewNRGBA(image.Rect(0, 0, 320*density, 480*density))
			img.SetNRGBA(10, 10, color.NRGBA{R: 255, A: 255})
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(file, img); err != nil {
				t.Fatal(err)
			}
			file.Close()
			for _, width := range []int{80, 160} {
				r := newImageRenderer("kitty", path, "dark", width, 0x123456, .5)
				preview := r.renderPreview(path, density)
				if preview.err != nil {
					t.Fatal(preview.err)
				}
				if len(preview.rows) != 30 || ansi.StringWidth(preview.rows[0]) != 40 {
					t.Fatalf("density=%d width=%d produced %dx%d, want 40x30", density, width, ansi.StringWidth(preview.rows[0]), len(preview.rows))
				}
				decoded := decodeKittyUpload(t, r.graphics.String())
				if decoded.Bounds() != img.Bounds() {
					t.Fatal("display sizing reduced PNG resolution")
				}
			}
			narrow := newImageRenderer("kitty", path, "dark", 30, 0x123456, .5)
			preview := narrow.renderPreview(path, density)
			if ansi.StringWidth(preview.rows[0]) > 22 || len(preview.rows) != 17 {
				t.Fatal("wide diagram must fit a narrow pane proportionally")
			}
		})
	}
}
