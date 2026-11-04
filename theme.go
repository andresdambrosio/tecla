package main

import "github.com/gdamore/tcell/v2"

type Box struct{ TL, TR, BL, BR, H, V, LT, RT rune }

var (
	Single = Box{'┌', '┐', '└', '┘', '─', '│', '├', '┤'}
	Double = Box{'╔', '╗', '╚', '╝', '═', '║', '╟', '╢'}
)

type Theme struct {
	Name string

	Normal, Keyword, Comment, String, Number tcell.Style
	Gutter, Selection                        tcell.Style
	Frame, Title                             tcell.Style

	MenuBar, MenuHot, MenuSel, MenuSelHot, MenuDis tcell.Style
	Status, StatusMsg                              tcell.Style

	Dialog, DialogTitle, Field, Button, ButtonSel, ListSel tcell.Style
	Shadow                                                 tcell.Style

	Pane, PaneFrame, PaneCode, PaneDim tcell.Style
	Scroll, Thumb                      tcell.Style

	DiffAdd, DiffDel, DiffAddHi, DiffDelHi, DiffFill tcell.Style

	Box, DlgBox Box
	ShadowW     int
	BtnL, BtnR  string
	MacTitle    bool
	Upper       bool
	Logo        string
	Cursor      tcell.CursorStyle
}

func hex(h int32) tcell.Color { return tcell.NewHexColor(h) }

func st(fg, bg int32) tcell.Style {
	return tcell.StyleDefault.Foreground(hex(fg)).Background(hex(bg))
}

func Themes() []Theme {
	return []Theme{
		dosTheme(),
		turboTheme(),
		macTheme(),
		phosphor("Ámbar", 0xFFB000, 0xC88A00, 0x6B4A00, 0x0C0700, 0x2A1C00),
		phosphor("Fósforo", 0x33FF66, 0x22BB4A, 0x116628, 0x020A04, 0x0A2612),
	}
}

// MS-DOS EDIT: texto gris sobre azul, menú gris, barra de estado cian.
func dosTheme() Theme {
	const (
		blue   = 0x0000AA
		gray   = 0xAAAAAA
		white  = 0xFFFFFF
		black  = 0x000000
		cyan   = 0x00AAAA
		bcyan  = 0x55FFFF
		yellow = 0xFFFF55
		bgreen = 0x55FF55
		dgray  = 0x555555
		bblue  = 0x5555FF
	)
	return Theme{
		Name:    "DOS",
		Normal:  st(gray, blue),
		Keyword: st(white, blue).Bold(true),
		Comment: st(bcyan, blue),
		String:  st(yellow, blue),
		Number:  st(bgreen, blue),
		Gutter:  st(bblue, blue),

		Selection: st(blue, gray),
		Frame:     st(gray, blue),
		Title:     st(blue, gray),

		MenuBar:    st(black, gray),
		MenuHot:    st(white, gray).Bold(true),
		MenuSel:    st(gray, black),
		MenuSelHot: st(white, black).Bold(true),
		MenuDis:    st(dgray, gray),
		Status:     st(black, cyan),
		StatusMsg:  st(white, cyan).Bold(true),

		Dialog:      st(black, gray),
		DialogTitle: st(black, gray).Bold(true),
		Field:       st(white, blue),
		Button:      st(black, gray),
		ButtonSel:   st(white, gray).Bold(true),
		ListSel:     st(gray, black),
		Shadow:      st(dgray, black),

		Pane:      st(gray, black),
		PaneFrame: st(gray, black),
		PaneCode:  st(bgreen, black),
		PaneDim:   st(dgray, black),
		Scroll:    st(blue, gray),
		Thumb:     st(black, gray),

		DiffAdd:   st(gray, 0x005500),
		DiffDel:   st(gray, 0x660000),
		DiffAddHi: st(black, bgreen),
		DiffDelHi: st(white, 0xCC2222).Bold(true),
		DiffFill:  st(0x2222CC, blue),

		Box: Single, DlgBox: Single, ShadowW: 2,
		BtnL: "< ", BtnR: " >",
		Upper:  true,
		Cursor: tcell.CursorStyleBlinkingUnderline,
	}
}

// Turbo C / Turbo Pascal: amarillo sobre azul, marcos dobles, botones verdes.
func turboTheme() Theme {
	const (
		blue   = 0x0000AA
		gray   = 0xAAAAAA
		white  = 0xFFFFFF
		black  = 0x000000
		cyan   = 0x00AAAA
		bcyan  = 0x55FFFF
		yellow = 0xFFFF55
		green  = 0x00AA00
		bgreen = 0x55FF55
		red    = 0xAA0000
		dgray  = 0x555555
	)
	return Theme{
		Name:    "Turbo",
		Normal:  st(yellow, blue),
		Keyword: st(white, blue).Bold(true),
		Comment: st(gray, blue),
		String:  st(bcyan, blue),
		Number:  st(bgreen, blue),
		Gutter:  st(cyan, blue),

		Selection: st(blue, cyan),
		Frame:     st(white, blue),
		Title:     st(white, blue).Bold(true),

		MenuBar:    st(black, gray),
		MenuHot:    st(red, gray),
		MenuSel:    st(black, green),
		MenuSelHot: st(red, green),
		MenuDis:    st(dgray, gray),
		Status:     st(black, gray),
		StatusMsg:  st(red, gray).Bold(true),

		Dialog:      st(black, gray),
		DialogTitle: st(white, gray).Bold(true),
		Field:       st(white, blue),
		Button:      st(black, green),
		ButtonSel:   st(white, green).Bold(true),
		ListSel:     st(white, cyan),
		Shadow:      st(dgray, black),

		Pane:      st(gray, black),
		PaneFrame: st(white, black),
		PaneCode:  st(yellow, black),
		PaneDim:   st(dgray, black),
		Scroll:    st(blue, cyan),
		Thumb:     st(blue, cyan),

		DiffAdd:   st(yellow, 0x005500),
		DiffDel:   st(yellow, 0x660000),
		DiffAddHi: st(black, bgreen),
		DiffDelHi: st(white, 0xCC2222).Bold(true),
		DiffFill:  st(0x2222CC, blue),

		Box: Double, DlgBox: Double, ShadowW: 2,
		BtnL: "[ ", BtnR: " ]",
		Upper:  true,
		Cursor: tcell.CursorStyleBlinkingUnderline,
	}
}

// Macintosh clásico (System 6): monocromo, barras rayadas, alertas con borde doble.
func macTheme() Theme {
	const (
		white = 0xFFFFFF
		black = 0x000000
		g1    = 0x444444
		g2    = 0x777777
		g3    = 0xAAAAAA
		g4    = 0xEEEEEE
	)
	return Theme{
		Name:    "Mac",
		Normal:  st(black, white),
		Keyword: st(black, white).Bold(true),
		Comment: st(g2, white).Italic(true),
		String:  st(g1, white),
		Number:  st(black, white),
		Gutter:  st(g3, white),

		Selection: st(white, black),
		Frame:     st(black, white),
		Title:     st(black, white).Bold(true),

		MenuBar:    st(black, white),
		MenuHot:    st(black, white),
		MenuSel:    st(white, black),
		MenuSelHot: st(white, black),
		MenuDis:    st(g3, white),
		Status:     st(black, white),
		StatusMsg:  st(black, white).Bold(true),

		Dialog:      st(black, white),
		DialogTitle: st(black, white).Bold(true),
		Field:       st(black, g4),
		Button:      st(black, white),
		ButtonSel:   st(white, black),
		ListSel:     st(white, black),
		Shadow:      st(black, black),

		Pane:      st(black, white),
		PaneFrame: st(black, white),
		PaneCode:  st(black, g4),
		PaneDim:   st(g2, white),
		Scroll:    st(black, white),
		Thumb:     st(black, white),

		DiffAdd:   st(black, g4),
		DiffDel:   st(black, 0xD8D8D8),
		DiffAddHi: st(white, black),
		DiffDelHi: st(white, g1).StrikeThrough(true),
		DiffFill:  st(g3, white),

		Box: Single, DlgBox: Double, ShadowW: 1,
		BtnL: "( ", BtnR: " )",
		MacTitle: true,
		Logo:     "◆",
		Cursor:   tcell.CursorStyleBlinkingBar,
	}
}

// Monitores monocromo de fósforo (ámbar / verde).
func phosphor(name string, bright, mid, dim, bg, tint int32) Theme {
	return Theme{
		Name:    name,
		Normal:  st(bright, bg),
		Keyword: st(bright, bg).Bold(true),
		Comment: st(dim, bg).Italic(true),
		String:  st(mid, bg),
		Number:  st(mid, bg),
		Gutter:  st(dim, bg),

		Selection: st(bg, bright),
		Frame:     st(mid, bg),
		Title:     st(bg, bright),

		MenuBar:    st(bg, mid),
		MenuHot:    st(bg, mid).Underline(true),
		MenuSel:    st(bg, bright),
		MenuSelHot: st(bg, bright).Underline(true),
		MenuDis:    st(dim, mid),
		Status:     st(bg, mid),
		StatusMsg:  st(bg, bright),

		Dialog:      st(bright, bg),
		DialogTitle: st(bg, bright),
		Field:       st(bright, tint),
		Button:      st(mid, bg),
		ButtonSel:   st(bg, bright),
		ListSel:     st(bg, bright),
		Shadow:      st(dim, 0x000000),

		Pane:      st(mid, bg),
		PaneFrame: st(dim, bg),
		PaneCode:  st(bright, tint),
		PaneDim:   st(dim, bg),
		Scroll:    st(dim, bg),
		Thumb:     st(bright, bg),

		DiffAdd:   st(bright, tint),
		DiffDel:   st(mid, tint),
		DiffAddHi: st(bg, bright),
		DiffDelHi: st(bg, mid).StrikeThrough(true),
		DiffFill:  st(dim, bg),

		Box: Single, DlgBox: Double, ShadowW: 1,
		BtnL: "[ ", BtnR: " ]",
		Upper:  true,
		Cursor: tcell.CursorStyleSteadyBlock,
	}
}
