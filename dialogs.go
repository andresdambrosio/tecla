package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v2"
)

type Dialog interface {
	Draw(a *App)
	Key(a *App, ev *tcell.EventKey)
}

// ---------- campo de texto de una línea ----------

type field struct {
	r      []rune
	c, off int
}

func newField(s string) field { r := []rune(s); return field{r: r, c: len(r)} }

func (f *field) String() string { return string(f.r) }

func (f *field) set(s string) { f.r = []rune(s); f.c = len(f.r) }

func (f *field) insert(s string) {
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	rs := []rune(s)
	nr := append(append(append([]rune(nil), f.r[:f.c]...), rs...), f.r[f.c:]...)
	f.r = nr
	f.c += len(rs)
}

func (f *field) key(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyRune:
		f.insert(string(ev.Rune()))
	case tcell.KeyLeft:
		if f.c > 0 {
			f.c--
		}
	case tcell.KeyRight:
		if f.c < len(f.r) {
			f.c++
		}
	case tcell.KeyHome, tcell.KeyCtrlA:
		f.c = 0
	case tcell.KeyEnd, tcell.KeyCtrlE:
		f.c = len(f.r)
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if f.c > 0 {
			f.r = append(f.r[:f.c-1], f.r[f.c:]...)
			f.c--
		}
	case tcell.KeyDelete:
		if f.c < len(f.r) {
			f.r = append(f.r[:f.c], f.r[f.c+1:]...)
		}
	case tcell.KeyCtrlU:
		f.r, f.c = nil, 0
	case tcell.KeyCtrlW:
		i := f.c
		for i > 0 && f.r[i-1] == ' ' {
			i--
		}
		for i > 0 && f.r[i-1] != ' ' && f.r[i-1] != '/' {
			i--
		}
		f.r = append(f.r[:i], f.r[f.c:]...)
		f.c = i
	default:
		return false
	}
	return true
}

func (f *field) draw(a *App, x, y, w int, st tcell.Style) {
	if f.c < f.off {
		f.off = f.c
	}
	if f.c >= f.off+w {
		f.off = f.c - w + 1
	}
	a.fill(x, y, w, 1, ' ', st)
	for i := 0; i < w && f.off+i < len(f.r); i++ {
		a.s.SetContent(x+i, y, f.r[f.off+i], nil, st)
	}
	a.cursorAt(x+f.c-f.off, y)
}

// ---------- utilidades de diálogo ----------

func (a *App) dialogBox(w, h int, title string) (int, int, int, int) {
	sw, sh := a.s.Size()
	w = min(w, sw-4)
	h = min(h, sh-2)
	x, y := (sw-w)/2, max(1, (sh-h)/2-1)
	T := a.T()
	a.fill(x, y, w, h, ' ', T.Dialog)
	a.box(x, y, w, h, T.DlgBox, T.Dialog)
	a.shadow(x, y, w, h)
	if title != "" {
		t := " " + title + " "
		a.put(x+(w-strW(t))/2, y, t, T.DialogTitle)
	}
	return x, y, w, h
}

func (a *App) buttons(x, y, w int, labels []string, sel int) {
	T := a.T()
	parts := make([]string, len(labels))
	total := 0
	for i, l := range labels {
		parts[i] = T.BtnL + l + T.BtnR
		total += strW(parts[i])
	}
	total += 3 * (len(labels) - 1)
	cx := x + (w-total)/2
	for i, p := range parts {
		s := T.Button
		if i == sel {
			s = T.ButtonSel
		}
		cx = a.put(cx, y, p, s) + 3
	}
}

func (a *App) msgBox(title string, lines ...string) {
	a.dlg = &ConfirmDialog{Title: title, Lines: lines, Buttons: []string{"Aceptar"}}
}

// ---------- entrada de texto ----------

type InputDialog struct {
	Title, Label string
	f            field
	OK           func(a *App, s string)
}

func NewInput(title, label, init string, ok func(*App, string)) *InputDialog {
	return &InputDialog{Title: title, Label: label, f: newField(init), OK: ok}
}

func (d *InputDialog) Draw(a *App) {
	T := a.T()
	x, y, w, _ := a.dialogBox(max(50, strW(d.Label)+6), 8, d.Title)
	a.putw(x+3, y+2, d.Label, T.Dialog, w-6)
	d.f.draw(a, x+3, y+3, w-6, T.Field)
	a.buttons(x, y+5, w, []string{"Aceptar", "Cancelar"}, 0)
}

func (d *InputDialog) Key(a *App, ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyEnter:
		a.dlg = nil
		d.OK(a, d.f.String())
	case tcell.KeyEsc:
		a.dlg = nil
	default:
		d.f.key(ev)
	}
}

func (d *InputDialog) Paste(s string) { d.f.insert(s) }

// ---------- confirmación / mensaje ----------

type ConfirmDialog struct {
	Title   string
	Lines   []string
	Buttons []string
	sel     int
	Do      func(a *App, i int)
}

func (d *ConfirmDialog) Draw(a *App) {
	T := a.T()
	w := 40
	for _, l := range d.Lines {
		w = max(w, strW(l)+8)
	}
	x, y, w, h := a.dialogBox(w, len(d.Lines)+6, d.Title)
	for i, l := range d.Lines {
		a.putw(x+(w-strW(l))/2, y+2+i, l, T.Dialog, w-4)
	}
	a.buttons(x, y+h-2, w, d.Buttons, d.sel)
}

func (d *ConfirmDialog) choose(a *App, i int) {
	a.dlg = nil
	if d.Do != nil {
		d.Do(a, i)
	}
}

func (d *ConfirmDialog) Key(a *App, ev *tcell.EventKey) {
	n := len(d.Buttons)
	switch ev.Key() {
	case tcell.KeyLeft, tcell.KeyBacktab:
		d.sel = (d.sel - 1 + n) % n
	case tcell.KeyRight, tcell.KeyTab:
		d.sel = (d.sel + 1) % n
	case tcell.KeyEnter:
		d.choose(a, d.sel)
	case tcell.KeyEsc:
		a.dlg = nil
	case tcell.KeyRune:
		r := unicode.ToLower(ev.Rune())
		for i, b := range d.Buttons {
			if unicode.ToLower([]rune(b)[0]) == r {
				d.choose(a, i)
				return
			}
		}
	}
}

// ---------- texto largo (ayuda) ----------

type TextDialog struct {
	Title string
	Lines []string
	off   int
}

func (d *TextDialog) Draw(a *App) {
	T := a.T()
	w := 30
	for _, l := range d.Lines {
		w = max(w, strW(l)+4)
	}
	x, y, w, h := a.dialogBox(w, len(d.Lines)+4, d.Title)
	inner := h - 4
	d.off = max(0, min(d.off, len(d.Lines)-inner))
	for i := 0; i < inner && d.off+i < len(d.Lines); i++ {
		l := d.Lines[d.off+i]
		s := T.Dialog
		if l == strings.ToUpper(l) && strings.TrimSpace(l) != "" {
			s = T.DialogTitle
		}
		a.putw(x+2, y+1+i, l, s, w-4)
	}
	if len(d.Lines) > inner {
		a.put(x+w-3, y+1, "↑", T.Dialog)
		a.put(x+w-3, y+h-3, "↓", T.Dialog)
	}
	a.buttons(x, y+h-2, w, []string{"Aceptar"}, 0)
}

func (d *TextDialog) Key(a *App, ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyUp:
		d.off--
	case tcell.KeyDown:
		d.off++
	case tcell.KeyPgUp:
		d.off -= 10
	case tcell.KeyPgDn:
		d.off += 10
	case tcell.KeyEsc, tcell.KeyEnter, tcell.KeyF1:
		a.dlg = nil
	}
}

// ---------- abrir archivo ----------

type entry struct {
	name string
	dir  bool
}

type OpenDialog struct {
	f        field
	list     []entry
	sel, off int
	dir      string
	err      string
	Title    string
	Pick     func(a *App, path string) // si está, elige un archivo en vez de abrirlo
}

func NewOpenDialog(a *App) *OpenDialog {
	init := ""
	if a.buf.Path != "" {
		if d := filepath.Dir(a.buf.Path); d != "." {
			init = d + "/"
		}
	}
	d := &OpenDialog{f: newField(init), Title: "Abrir"}
	d.refresh()
	return d
}

// NewPickDialog es el mismo diálogo, pero para elegir un archivo sin abrirlo en el editor.
func NewPickDialog(a *App, title string, pick func(*App, string)) *OpenDialog {
	d := NewOpenDialog(a)
	d.Title, d.Pick = title, pick
	return d
}

func (d *OpenDialog) done(a *App, p string) {
	a.dlg = nil
	if d.Pick != nil {
		d.Pick(a, p)
		return
	}
	a.openPath(p)
}

func (d *OpenDialog) typedDir() string {
	t := d.f.String()
	if i := strings.LastIndex(t, "/"); i >= 0 {
		return t[:i+1]
	}
	return ""
}

func (d *OpenDialog) refresh() {
	t := d.f.String()
	td := d.typedDir()
	base := t[len(td):]
	dir := expandHome(td)
	if dir == "" {
		dir = "."
	}
	d.dir = dir
	d.list = nil
	d.err = ""
	ents, err := os.ReadDir(dir)
	if err != nil {
		d.err = "No se puede leer la carpeta"
		return
	}
	if base == "" {
		d.list = append(d.list, entry{"..", true})
	}
	lb := strings.ToLower(base)
	var dirs, files []entry
	for _, e := range ents {
		n := e.Name()
		if strings.HasPrefix(n, ".") && !strings.HasPrefix(base, ".") {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(n), lb) {
			continue
		}
		isDir := e.IsDir()
		if e.Type()&os.ModeSymlink != 0 {
			if fi, err := os.Stat(filepath.Join(dir, n)); err == nil {
				isDir = fi.IsDir()
			}
		}
		if isDir {
			dirs = append(dirs, entry{n, true})
		} else {
			files = append(files, entry{n, false})
		}
	}
	less := func(l []entry) func(i, j int) bool {
		return func(i, j int) bool { return strings.ToLower(l[i].name) < strings.ToLower(l[j].name) }
	}
	sort.Slice(dirs, less(dirs))
	sort.Slice(files, less(files))
	d.list = append(append(d.list, dirs...), files...)
	d.sel = max(0, min(d.sel, len(d.list)-1))
}

func (d *OpenDialog) choose(a *App, e entry, open bool) {
	td := d.typedDir()
	if e.name == ".." {
		base := expandHome(td)
		if base == "" {
			base = "."
		}
		nd := filepath.Clean(filepath.Join(base, ".."))
		if nd == "." {
			d.f.set("")
		} else {
			d.f.set(nd + "/")
		}
	} else if e.dir {
		d.f.set(td + e.name + "/")
	} else {
		p := td + e.name
		if open {
			d.done(a, expandHome(p))
			return
		}
		d.f.set(p)
	}
	d.sel = 0
	d.off = 0
	d.refresh()
}

func (d *OpenDialog) Key(a *App, ev *tcell.EventKey) {
	switch ev.Key() {
	case tcell.KeyEsc:
		a.dlg = nil
	case tcell.KeyUp:
		d.sel = max(0, d.sel-1)
	case tcell.KeyDown:
		d.sel = min(len(d.list)-1, d.sel+1)
	case tcell.KeyPgUp:
		d.sel = max(0, d.sel-10)
	case tcell.KeyPgDn:
		d.sel = min(len(d.list)-1, d.sel+10)
	case tcell.KeyTab:
		if len(d.list) > 0 {
			d.choose(a, d.list[d.sel], false)
		}
	case tcell.KeyEnter:
		t := expandHome(d.f.String())
		if t != "" && !strings.HasSuffix(t, "/") {
			fi, err := os.Stat(t)
			switch {
			case err == nil && fi.IsDir():
				d.f.set(d.f.String() + "/")
				d.refresh()
				return
			case err == nil, len(d.list) == 0:
				d.done(a, t)
				return
			}
		}
		if len(d.list) > 0 {
			d.choose(a, d.list[d.sel], true)
		}
	default:
		if d.f.key(ev) {
			d.sel, d.off = 0, 0
			d.refresh()
		}
	}
}

func (d *OpenDialog) Paste(s string) { d.f.insert(s); d.refresh() }

func (d *OpenDialog) Draw(a *App) {
	T := a.T()
	x, y, w, h := a.dialogBox(70, 22, d.Title)
	a.put(x+3, y+2, "Nombre del archivo:", T.Dialog)
	abs, _ := filepath.Abs(d.dir)
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(abs, home) {
		abs = "~" + abs[len(home):]
	}
	a.putw(x+3, y+5, "Carpeta: "+abs, T.Dialog, w-6)
	lh := h - 10
	lx, ly, lw := x+3, y+6, w-6
	a.fill(lx, ly, lw, lh, ' ', T.Field)
	if d.err != "" {
		a.put(lx+1, ly, d.err, T.Field)
	}
	if d.sel < d.off {
		d.off = d.sel
	}
	if d.sel >= d.off+lh {
		d.off = d.sel - lh + 1
	}
	for i := 0; i < lh && d.off+i < len(d.list); i++ {
		e := d.list[d.off+i]
		name := e.name
		if e.dir && name != ".." {
			name += "/"
		}
		if e.dir {
			name = "▸ " + name
		} else {
			name = "  " + name
		}
		s := T.Field
		if d.off+i == d.sel {
			s = T.ListSel
			a.fill(lx, ly+i, lw, 1, ' ', s)
		}
		a.putw(lx+1, ly+i, name, s, lw-2)
	}
	hint, ok := "↑↓ elegir · Tab completar · Enter abrir · un nombre nuevo crea el archivo", "Abrir"
	if d.Pick != nil {
		hint, ok = "↑↓ elegir · Tab completar · Enter aceptar", "Elegir"
	}
	a.putw(x+3, y+h-3, hint, T.Dialog, w-6)
	a.buttons(x, y+h-2, w, []string{ok, "Cancelar"}, 0)
	d.f.draw(a, x+3, y+3, w-6, T.Field)
}
