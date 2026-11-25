package main

import (
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
)

// MarkdownView muestra el archivo del editor como Markdown con las fórmulas
// dibujadas. Ocupa la ventana del editor; Ctrl+P o Esc vuelve a editar.
type MarkdownView struct {
	buf       *Buffer
	ver, w    int
	lines     []mdLine
	off, hoff int
	page      int
	want      int  // línea del archivo que hay que mostrar arriba al abrir
	moved     bool // el usuario se desplazó: al cerrar, el cursor va ahí
	maxW      int
}

func (a *App) togglePreview() {
	if v, ok := a.dlg.(*MarkdownView); ok {
		v.close(a)
		return
	}
	if a.dlg != nil {
		return
	}
	if !isMarkdownPath(a.buf.Path) {
		a.flash("La vista previa es para archivos Markdown (.md)")
		return
	}
	a.menuOpen = false
	a.focus = focEditor
	a.dlg = &MarkdownView{buf: a.buf, ver: -1, want: a.buf.ScrollY}
}

func (v *MarkdownView) close(a *App) {
	a.dlg = nil
	if v.moved && v.off < len(v.lines) && a.buf == v.buf {
		y := v.lines[v.off].src
		v.buf.Anchor = nil
		v.buf.Cur = Pos{min(y, len(v.buf.Lines)-1), 0}
		v.buf.ScrollY = v.buf.Cur.Y
		v.buf.WantV = 0
	}
}

func (v *MarkdownView) layout(w int) {
	if v.ver == v.buf.Version && v.w == w {
		return
	}
	text := make([]string, len(v.buf.Lines))
	for i, l := range v.buf.Lines {
		text[i] = string(l)
	}
	top := -1
	if v.ver >= 0 && v.off < len(v.lines) {
		top = v.lines[v.off].src
	}
	v.lines = renderMarkdown(text, w)
	v.ver, v.w = v.buf.Version, w
	v.maxW = 0
	for _, l := range v.lines {
		v.maxW = max(v.maxW, len(l.cells))
	}
	if top < 0 {
		top = v.want
	}
	v.off = 0
	for i, l := range v.lines {
		if l.src >= top {
			v.off = i
			break
		}
	}
}

func (v *MarkdownView) clamp() {
	v.off = max(0, min(v.off, len(v.lines)-v.page))
	v.hoff = max(0, min(v.hoff, v.maxW-v.w))
}

func (v *MarkdownView) scroll(n int) {
	v.off += n
	v.moved = true
	v.clamp()
}

func (v *MarkdownView) Wheel(n int) { v.scroll(n) }

func (v *MarkdownView) Key(a *App, ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyEsc, tcell.KeyCtrlP:
		v.close(a)
	case tcell.KeyUp:
		v.scroll(-1)
	case tcell.KeyDown, tcell.KeyEnter:
		v.scroll(1)
	case tcell.KeyPgUp:
		v.scroll(-max(1, v.page-1))
	case tcell.KeyPgDn:
		v.scroll(max(1, v.page-1))
	case tcell.KeyHome:
		v.scroll(-len(v.lines))
	case tcell.KeyEnd:
		v.scroll(len(v.lines))
	case tcell.KeyLeft:
		v.hoff -= 8
		v.clamp()
	case tcell.KeyRight:
		v.hoff += 8
		v.clamp()
	case tcell.KeyF9:
		a.nextTheme()
	case tcell.KeyCtrlS:
		a.save(nil)
	case tcell.KeyCtrlQ:
		v.close(a)
		a.tryQuit()
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'q':
			v.close(a)
		case ' ':
			v.scroll(max(1, v.page-1))
		case 'b':
			v.scroll(-max(1, v.page-1))
		case 'g':
			v.scroll(-len(v.lines))
		case 'G':
			v.scroll(len(v.lines))
		case 'j':
			v.scroll(1)
		case 'k':
			v.scroll(-1)
		}
	}
}

func (v *MarkdownView) Draw(a *App) {
	T := a.T()
	r := a.rEd
	a.fill(r.x, r.y, r.w, r.h, ' ', T.Normal)
	a.box(r.x, r.y, r.w, r.h, Double, T.Frame)
	name := "Sin título"
	if v.buf.Path != "" {
		name = filepath.Base(v.buf.Path)
	}
	title := " Vista previa · " + name + " "
	if T.Upper {
		title = strings.ToUpper(title)
	}
	a.putw(r.x+max(1, (r.w-strW(title))/2), r.y, title, T.Title, r.w-2)
	x0, y0 := r.x+2, r.y+1
	w := max(10, r.w-4)
	v.page = max(1, r.h-2)
	v.layout(w)
	v.clamp()
	for i := 0; i < v.page && v.off+i < len(v.lines); i++ {
		cells := v.lines[v.off+i].cells
		for x := 0; x < w && v.hoff+x < len(cells); x++ {
			c := cells[v.hoff+x]
			if c.s == "" {
				continue
			}
			rs := []rune(c.s)
			a.s.SetContent(x0+x, y0+i, rs[0], rs[1:], c.k.style(T, c.at))
		}
	}
	// barra de desplazamiento como la del editor
	if n := v.page; n >= 3 {
		sx := r.x + r.w - 1
		a.s.SetContent(sx, y0, '↑', nil, T.Scroll)
		a.s.SetContent(sx, y0+n-1, '↓', nil, T.Scroll)
		track := n - 2
		for i := 0; i < track; i++ {
			a.s.SetContent(sx, y0+1+i, '░', nil, T.Scroll)
		}
		pos := 0
		if total := len(v.lines); total > n && track > 1 {
			pos = max(0, min(track-1, v.off*(track-1)/max(1, total-n)))
		}
		a.s.SetContent(sx, y0+1+pos, '█', nil, T.Thumb)
	}
	_, h := a.s.Size()
	sw, _ := a.s.Size()
	a.fill(0, h-1, sw, 1, ' ', T.Status)
	a.putw(0, h-1, " Vista previa: ↑↓ PgUp/PgDn desplazar · ←→ a los costados · Ctrl+P o Esc volver a editar", T.Status, sw)
}
