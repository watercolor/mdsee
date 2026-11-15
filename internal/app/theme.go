package app

// Colors follow mdterm's dark and light palettes. Code tokens are highlighted
// locally with Chroma so both themes remain independent of the terminal palette.
type themePalette struct {
	background, foreground, border, title, muted string
	codeBackground, codeText, codeLabel          string
	inlineForeground, inlineBackground           string
	overlay, selection                           string
	headings                                     [6]string
	codeString, codeKeyword, codeNumber          string
	codeComment, codeFunction                    string
}

func paletteFor(theme string) themePalette {
	if theme == "light" {
		return themePalette{
			background: "#eff1f5", foreground: "#4c4f69", border: "#acb0be", title: "#5c5f77", muted: "#6c6f85",
			codeBackground: "#e6e9ef", codeText: "#323232", codeLabel: "#6c6f85",
			inlineForeground: "#b35202", inlineBackground: "#e6e9ef", overlay: "#e6e9ef", selection: "#bcc0cc",
			headings:   [6]string{"#20202a", "#1e66f5", "#8839ef", "#40a02b", "#df8e1d", "#6c6f85"},
			codeString: "#183691", codeKeyword: "#a71d5d", codeNumber: "#0086b3", codeComment: "#6a737d", codeFunction: "#795da3",
		}
	}
	return themePalette{
		background: "#1e1e2e", foreground: "#cdd6f4", border: "#44475a", title: "#9399b2", muted: "#7f849c",
		codeBackground: "#1e202a", codeText: "#c0c5ce", codeLabel: "#7f849c",
		inlineForeground: "#f2cd93", inlineBackground: "#282a36", overlay: "#282a36", selection: "#45475a",
		headings:   [6]string{"#cdd6f4", "#89b4fa", "#cba6f7", "#a6e3a1", "#f9e2af", "#7f849c"},
		codeString: "#a3be8c", codeKeyword: "#b48ead", codeNumber: "#d08770", codeComment: "#65737e", codeFunction: "#8fa1b3",
	}
}
