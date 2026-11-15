package app

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	glamansi "charm.land/glamour/v2/ansi"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

const (
	maxImageBytes    = 10 * 1024 * 1024
	maxImagePixels   = 8_000_000
	maxPreviewImages = 32
	imageProbeID     = 0x6d6463
)

// iTerm2 supports placeholders without implementing Kitty's newer feature query.
// 3.7.3 fixes placeholders whose image IDs omit the optional high-byte diacritic.
func itermSupportsImagePlaceholders(name string) bool {
	version, ok := strings.CutPrefix(name, "iTerm2 ")
	if !ok {
		return false
	}
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return false
	}
	var numbers [3]uint64
	for i, part := range parts {
		n, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return false
		}
		numbers[i] = n
	}
	for i, minimum := range [3]uint64{3, 7, 3} {
		if numbers[i] != minimum {
			return numbers[i] > minimum
		}
	}
	return true
}

// Each render job owns its cache; scroll-map probes reuse the decoded previews.
type imageRenderer struct {
	mode, baseDir, theme string
	width, idBase        int
	cellRatio            float64
	cache                map[string]imagePreview
	remote               *remoteImageCache
	diagrams             *mermaidImageCache
	diagramKeys          map[string]struct{}
	graphics             strings.Builder
	ids                  []int
}

type imagePreview struct {
	rows    []string
	err     error
	loading bool
}

func newImageRenderer(mode, path, theme string, width, idBase int, ratio float64) *imageRenderer {
	if ratio <= 0 {
		ratio = 0.5
	}
	return &imageRenderer{mode: mode, baseDir: filepath.Dir(path), theme: theme, width: width,
		idBase: idBase, cellRatio: ratio, cache: make(map[string]imagePreview), diagramKeys: make(map[string]struct{})}
}

func localImagePath(baseDir, destination string) (string, error) {
	// Native Windows drive letters contain ':' but are not URL schemes.
	if filepath.VolumeName(destination) != "" && filepath.IsAbs(destination) && !strings.HasPrefix(destination, "//") && !strings.HasPrefix(destination, `\\`) {
		return filepath.Clean(destination), nil
	}
	u, err := url.Parse(destination)
	if err != nil {
		return "", fmt.Errorf("图片路径无效")
	}
	if u.Scheme != "" && u.Scheme != "file" || u.Host != "" || strings.HasPrefix(destination, `\\`) {
		return "", fmt.Errorf("不支持的图片协议或外部路径")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("本地图片路径不支持查询参数或片段")
	}
	path := filepath.FromSlash(u.Path)
	if strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, "//") {
		return "", fmt.Errorf("暂不加载网络路径图片")
	}
	if u.Scheme == "file" {
		// file:///C:/... is the conventional Windows file URL form.
		if len(path) > 3 && path[0] == filepath.Separator && path[2] == ':' {
			path = path[1:]
		}
		if !filepath.IsAbs(path) {
			return "", fmt.Errorf("file URL 必须使用绝对路径")
		}
	}
	if path == "" {
		return "", fmt.Errorf("图片路径为空")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	return filepath.Clean(path), nil
}

var errImageBytesLimit = errors.New("图片超过 10 MiB")
var errImagePixelsLimit = errors.New("图片超过 800 万像素")

func readLocalImage(path string) (image.Image, error) {
	raw, err := readLocalImageData(path)
	if err != nil {
		return nil, err
	}
	return decodeImage(raw)
}

func readLocalImageData(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("无法读取图片")
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("图片必须是普通文件")
	}
	if info.Size() > maxImageBytes {
		return nil, errImageBytesLimit
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("无法读取图片")
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxImageBytes+1))
	if err != nil || len(raw) > maxImageBytes {
		return nil, fmt.Errorf("图片读取失败或超过 10 MiB")
	}
	return raw, nil
}

func pngImageBounds(raw []byte) (image.Rectangle, error) {
	cfg, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return image.Rectangle{}, fmt.Errorf("PNG 图片头无效")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > maxImagePixels {
		return image.Rectangle{}, errImagePixelsLimit
	}
	return image.Rect(0, 0, cfg.Width, cfg.Height), nil
}

func decodeImage(raw []byte) (image.Image, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("不支持或损坏的图片（支持 PNG、JPEG、GIF）")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > maxImagePixels {
		return nil, errImagePixelsLimit
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("图片解码失败")
	}
	return img, nil
}

func imageDimensions(bounds image.Rectangle, width int, ratio float64, maxRows int) (int, int) {
	cols := max(1, min(100, width, bounds.Dx()))
	rows := max(1, int(math.Ceil(float64(cols)*float64(bounds.Dy())/float64(bounds.Dx())*ratio)))
	if rows > maxRows {
		rows = maxRows
		cols = max(1, int(math.Round(float64(rows)*float64(bounds.Dx())/float64(bounds.Dy())/ratio)))
	}
	return cols, rows
}

func (r *imageRenderer) preview(destination string) imagePreview {
	return r.renderPreview(destination, 0)
}

// diagramScale is the PNG's render density. Zero denotes an ordinary image.
func (r *imageRenderer) renderPreview(destination string, diagramScale int) imagePreview {
	path, network, err := remoteImageURL(destination)
	if err == nil && !network {
		path, err = localImagePath(r.baseDir, destination)
	}
	if err != nil {
		return imagePreview{err: err}
	}
	if cached, ok := r.cache[path]; ok {
		return cached
	}
	if len(r.cache) >= maxPreviewImages {
		return imagePreview{err: fmt.Errorf("每次预览最多加载 32 张图片")}
	}
	p := imagePreview{}
	var img image.Image
	var originalPNG []byte
	var bounds image.Rectangle
	if !network && diagramScale > 0 && r.mode == "kitty" {
		originalPNG, err = readLocalImageData(path)
		if err == nil {
			bounds, err = pngImageBounds(originalPNG)
		}
	} else if network {
		if r.remote == nil {
			return imagePreview{err: fmt.Errorf("网络图片下载未启用")}
		}
		var data []byte
		data, err, p.loading = r.remote.lookup(path)
		if p.loading {
			r.cache[path] = p
			return p
		}
		if err == nil {
			img, err = decodeImage(data)
		}
	} else {
		img, err = readLocalImage(path)
	}
	if err != nil {
		p.err = err
	} else {
		if img != nil {
			bounds = img.Bounds()
		}
		maxRows := 24
		if diagramScale > 0 && r.mode == "kitty" {
			maxRows = 256
		}
		width := max(1, r.width-8)
		if diagramScale > 0 {
			// Mermaid's default 16px labels map approximately to two columns for CJK.
			// Remove PNG density before layout so higher resolution never enlarges the diagram.
			naturalColumns := max(1, int(math.Ceil(float64(bounds.Dx())/float64(diagramScale)/8)))
			width = min(width, naturalColumns)
		}
		cols, rows := imageDimensions(bounds, width, r.cellRatio, maxRows)
		if r.mode == "kitty" {
			id := r.idBase + len(r.ids)
			// Keep diagram text and thin lines at source resolution for Retina displays.
			transmitted := img
			if diagramScale == 0 {
				scale := min(1.0, float64(cols*12)/float64(img.Bounds().Dx()), float64(rows*24)/float64(img.Bounds().Dy()))
				transmitted = sampleImage(img, max(1, int(float64(img.Bounds().Dx())*scale)), max(1, int(float64(img.Bounds().Dy())*scale)), color.NRGBA{})
			}
			var payload bytes.Buffer
			options := &kitty.Options{Action: kitty.TransmitAndPut,
				ID: id, Transmission: kitty.Direct, Format: kitty.PNG, Quite: 2, Chunk: true, VirtualPlacement: true, Columns: cols, Rows: rows}
			if originalPNG != nil {
				err = writeKittyPNG(&payload, originalPNG, options)
			} else {
				err = kitty.EncodeGraphics(&payload, transmitted, options)
			}
			if err != nil {
				p.err = fmt.Errorf("图片编码失败")
			} else {
				r.graphics.Write(payload.Bytes())
				r.ids = append(r.ids, id)
				p.rows = kittyImageRows(id, cols, rows)
			}
		} else {
			red, green, blue, _ := lipgloss.Color(paletteFor(r.theme).background).RGBA()
			bg := color.NRGBA{R: uint8(red >> 8), G: uint8(green >> 8), B: uint8(blue >> 8), A: 255}
			p.rows = blockImageRows(sampleImage(img, cols, rows*2, bg))
		}
	}
	r.cache[path] = p
	return p
}

// Generated PNGs already passed image validation. Send their compressed bytes
// directly so each render avoids allocating pixels and recompressing the image.
func writeKittyPNG(w io.Writer, raw []byte, options *kitty.Options) error {
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(raw)))
	base64.StdEncoding.Encode(encoded, raw)
	for start := 0; start < len(encoded); start += kitty.MaxChunkSize {
		end := min(start+kitty.MaxChunkSize, len(encoded))
		header := []string{fmt.Sprintf("q=%d", options.Quite)}
		if start == 0 {
			header = options.Options()
		}
		more := "m=0"
		if end < len(encoded) {
			more = "m=1"
		}
		header = append(header, more)
		if _, err := io.WriteString(w, ansi.KittyGraphics(encoded[start:end], header...)); err != nil {
			return err
		}
	}
	return nil
}

func sampleImage(src image.Image, width, height int, background color.NRGBA) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, width, height))
	b := src.Bounds()
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			c := color.NRGBAModel.Convert(src.At(b.Min.X+x*b.Dx()/width, b.Min.Y+y*b.Dy()/height)).(color.NRGBA)
			if background.A != 0 {
				a := uint32(c.A)
				c.R = uint8((uint32(c.R)*a + uint32(background.R)*(255-a)) / 255)
				c.G = uint8((uint32(c.G)*a + uint32(background.G)*(255-a)) / 255)
				c.B = uint8((uint32(c.B)*a + uint32(background.B)*(255-a)) / 255)
				c.A = 255
			}
			dst.SetNRGBA(x, y, c)
		}
	}
	return dst
}

func blockImageRows(img *image.NRGBA) []string {
	var rows []string
	for y := 0; y < img.Bounds().Dy(); y += 2 {
		var row strings.Builder
		for x := 0; x < img.Bounds().Dx(); x++ {
			a, b := img.NRGBAAt(x, y), img.NRGBAAt(x, min(y+1, img.Bounds().Dy()-1))
			fmt.Fprintf(&row, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀", a.R, a.G, a.B, b.R, b.G, b.B)
		}
		row.WriteString("\x1b[0m")
		rows = append(rows, row.String())
	}
	return rows
}

func kittyImageRows(id, cols, height int) []string {
	rows := make([]string, height)
	for y := range rows {
		var row strings.Builder
		fmt.Fprintf(&row, "\x1b[38;2;%d;%d;%dm", id>>16&255, id>>8&255, id&255)
		for x := 0; x < cols; x++ {
			row.WriteRune(kitty.Placeholder)
			row.WriteRune(kitty.Diacritic(y))
			row.WriteRune(kitty.Diacritic(x))
		}
		row.WriteString("\x1b[0m")
		rows[y] = row.String()
	}
	return rows
}

func deleteImageResources(ids []int) string {
	var out strings.Builder
	for _, id := range ids {
		fmt.Fprintf(&out, "\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", id)
	}
	return out.String()
}

type imageMarkdownRenderer struct {
	md     goldmark.Markdown
	images *imageRenderer
	code   *codeBlockRenderer
}

func newImageMarkdownRenderer(width int, style glamansi.StyleConfig, images *imageRenderer) *imageMarkdownRenderer {
	md := goldmark.New(goldmark.WithExtensions(extension.GFM, extension.DefinitionList),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()))
	md.SetRenderer(renderer.NewRenderer(renderer.WithNodeRenderers(util.Prioritized(
		glamansi.NewRenderer(glamansi.Options{WordWrap: max(10, width), Styles: style}), 1000))))
	return &imageMarkdownRenderer{md: md, images: images, code: newCodeBlockRenderer(images.theme)}
}

// Only images occupying their own paragraph expand into rows. Inline images keep
// Glamour's text representation, so tables and surrounding prose retain their layout.
func (r *imageMarkdownRenderer) Render(source string) (string, error) {
	raw := []byte(source)
	root := r.md.Parser().Parse(text.NewReader(raw))
	raw = r.renderDiagramNodes(raw, root)
	raw, codeBlocks := replaceCodeNodes(raw, root)
	prefix := "MI"
	for strings.Contains(source, prefix) {
		prefix += "I"
	}
	type replacement struct {
		token, caption string
		preview        imagePreview
	}
	var replacements []replacement
	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		img, ok := node.(*ast.Image)
		if !entering || !ok || r.images.mode == "off" {
			return ast.WalkContinue, nil
		}
		target := ast.Node(img)
		if p := target.Parent(); p != nil && p.Kind() == ast.KindLink && p.ChildCount() == 1 {
			target = p
		}
		parent := target.Parent()
		if parent == nil || parent.ChildCount() != 1 || (parent.Kind() != ast.KindParagraph && parent.Kind() != ast.KindTextBlock) {
			return ast.WalkSkipChildren, nil
		}
		token := fmt.Sprintf("%s%04dZ", prefix, len(replacements))
		destination := string(util.UnescapePunctuations(util.ResolveNumericReferences(util.ResolveEntityNames(img.Destination))))
		caption := strings.TrimSpace(string(img.Text(raw)))
		if caption == "" {
			caption = destination
		}
		caption = strings.Map(func(c rune) rune {
			if unicode.IsControl(c) {
				return ' '
			}
			return c
		}, caption)
		replacements = append(replacements, replacement{token, caption, r.images.preview(destination)})
		start := len(raw)
		raw = append(raw, token...)
		parent.ReplaceChild(parent, target, ast.NewTextSegment(text.NewSegment(start, len(raw))))
		return ast.WalkSkipChildren, nil
	})
	var out bytes.Buffer
	if err := r.md.Renderer().Render(&out, raw, root); err != nil {
		return "", err
	}
	lines := strings.Split(out.String(), "\n")
	var result []string
	for _, line := range lines {
		if rows, ok := r.code.replaceLine(line, codeBlocks, r.images.width); ok {
			result = append(result, rows...)
			continue
		}
		replaced := false
		for _, p := range replacements {
			i := strings.Index(line, p.token)
			if i < 0 {
				continue
			}
			prefix := line[:i] + "\x1b[0m"
			available := max(1, r.images.width-ansi.StringWidth(prefix)-2)
			caption := "图片：" + p.caption
			if p.preview.loading {
				caption = "图片：下载中 · " + p.caption
			} else if p.preview.err != nil {
				caption = "图片：" + p.preview.err.Error() + " · " + p.caption
			}
			result = append(result, prefix+ansi.Truncate(caption, available, "…")+"\x1b[0m")
			for _, row := range p.preview.rows {
				result = append(result, prefix+ansi.Truncate(row, available, ""))
			}
			replaced = true
			break
		}
		if !replaced {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n"), nil
}
