package ui

import "strings"

type font struct {
	height  int
	gap     int // space between letters
	letters map[rune][]string
}

var fontLarge = font{
	height: 6,
	gap:    0,
	letters: map[rune][]string{
		'M': {
			"███╗   ███╗",
			"████╗ ████║",
			"██╔████╔██║",
			"██║╚██╔╝██║",
			"██║ ╚═╝ ██║",
			"╚═╝     ╚═╝",
		},
		'A': {
			" █████╗ ",
			"██╔══██╗",
			"███████║",
			"██╔══██║",
			"██║  ██║",
			"╚═╝  ╚═╝",
		},
		'S': {
			"███████╗",
			"██╔════╝",
			"███████╗",
			"╚════██║",
			"███████║",
			"╚══════╝",
		},
		'T': {
			"████████╗",
			"╚══██╔══╝",
			"   ██║   ",
			"   ██║   ",
			"   ██║   ",
			"   ╚═╝   ",
		},
		'E': {
			"███████╗",
			"██╔════╝",
			"█████╗  ",
			"██╔══╝  ",
			"███████╗",
			"╚══════╝",
		},
		'R': {
			"██████╗ ",
			"██╔══██╗",
			"██████╔╝",
			"██╔══██╗",
			"██║  ██║",
			"╚═╝  ╚═╝",
		},
		'I': {
			"██╗",
			"██║",
			"██║",
			"██║",
			"██║",
			"╚═╝",
		},
		'N': {
			"███╗   ██╗",
			"████╗  ██║",
			"██╔██╗ ██║",
			"██║╚██╗██║",
			"██║ ╚████║",
			"╚═╝  ╚═══╝",
		},
		'D': {
			"██████╗ ",
			"██╔══██╗",
			"██║  ██║",
			"██║  ██║",
			"██████╔╝",
			"╚═════╝ ",
		},
	},
}

var fontMedium = font{
	height: 5,
	gap:    1,
	letters: map[rune][]string{
		'M': {
			"█▄ ▄█",
			"██▄██",
			"█ █ █",
			"█   █",
			"█   █",
		},
		'A': {
			" ▄█▄ ",
			"█   █",
			"█████",
			"█   █",
			"█   █",
		},
		'S': {
			"▄████",
			"█    ",
			" ███ ",
			"    █",
			"████▀",
		},
		'T': {
			"█████",
			"  █  ",
			"  █  ",
			"  █  ",
			"  █  ",
		},
		'E': {
			"████▄",
			"█    ",
			"███  ",
			"█    ",
			"████▀",
		},
		'R': {
			"████▄",
			"█   █",
			"████▀",
			"█  █ ",
			"█   █",
		},
		'I': {
			"█",
			"█",
			"█",
			"█",
			"█",
		},
		'N': {
			"█▄  █",
			"█ █ █",
			"█ █ █",
			"█  ▀█",
			"█   █",
		},
		'D': {
			"████▄",
			"█   █",
			"█   █",
			"█   █",
			"████▀",
		},
	},
}

func wordWidth(word string, f font) int {
	if len(word) == 0 {
		return 0
	}
	w := 0
	for i, ch := range word {
		rows := f.letters[ch]
		if len(rows) > 0 {
			// Use first row to measure width (rune count for display width)
			w += runeWidth(rows[0])
		}
		if i < len(word)-1 {
			w += f.gap
		}
	}
	return w
}

// runeWidth returns the display width of a string, counting each rune as 1.
func runeWidth(s string) int {
	return len([]rune(s))
}

func render(word string, f font) string {
	rows := make([]strings.Builder, f.height)
	for i, ch := range word {
		glyph := f.letters[ch]
		for row := 0; row < f.height; row++ {
			if i > 0 {
				for g := 0; g < f.gap; g++ {
					rows[row].WriteRune(' ')
				}
			}
			if row < len(glyph) {
				rows[row].WriteString(glyph[row])
			}
		}
	}
	lines := make([]string, f.height)
	for i := range rows {
		lines[i] = rows[i].String()
	}
	return strings.Join(lines, "\n")
}

func renderLogo(maxWidth int) string {
	const word = "MASTERMIND"
	const padding = 1 // leading space
	// Try largest font first
	if w := wordWidth(word, fontLarge) + padding; w > 0 && w <= maxWidth {
		return " " + strings.ReplaceAll(render(word, fontLarge), "\n", "\n ")
	}
	if w := wordWidth(word, fontMedium) + padding; w > 0 && w <= maxWidth {
		return " " + strings.ReplaceAll(render(word, fontMedium), "\n", "\n ")
	}
	return " MASTERMIND"
}
