package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

type App struct {
	s      tcell.Screen
	buf    *Buffer
	themes []Theme
	ti     int

	lineNums bool
	follow   bool
	quit     bool

	menus    []Menu
	menuOpen bool
	mi, ii   int
	menuX    []int
	ddX, ddW int

	dlg   Dialog
	msg   string
	msgAt time.Time

	clip     string
	pasting  bool
	pasteBuf []rune

	mouseDown bool
	dragging  bool
	lastClick time.Time
	lastPos   Pos

	lastFind string
	lastKey  time.Time
	curX     int
	curY     int
	history  []string

	// geometría
	tx, ty, tw, th int
	gw, cw         int
	rTree, rEd     rect
	rTerm, rAgent  rect
	treeOn, termOn bool // paneles realmente visibles (se ocultan si la ventana es chica)
	paneOn         bool
	focus          int

	// carpeta y árbol
	root     string
	showTree bool
	tree     []treeNode
	treeOpen map[string]bool
	treeSel  int
	treeOff  int
	treeAt   time.Time

	// terminal
	term       *Term
	showTerm   bool
	pendingCmd string

	// IA
	job        *AIJob
	jobs       []*AIJob
	pane       bool
	paneScroll int
	paneFollow bool
	engine     string
	agent      bool
	have       map[string]bool
	chat       field
	chatHi     int
	claudeSID  string

	// caché de resaltado
	hlCache []bool
	hlVer   int
	hlLang  *Lang
}

type rect struct{ x, y, w, h int }

func (r rect) has(x, y int) bool { return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h }

const (
	focEditor = iota
	focTree
	focTerm
	focAgent
)

func NewApp(path string) (*App, error) {
	var b *Buffer
	root := ""
	if path != "" {
		if fi, err := os.Stat(path); err == nil && fi.IsDir() {
			root, path = path, ""
		}
	}
	if path != "" {
		var err error
		if b, err = LoadBuffer(path); err != nil {
			return nil, err
		}
	} else {
		b = NewBuffer("")
	}
	s, err := tcell.NewScreen()
	if err != nil {
		return nil, err
	}
	a, err := newAppWithScreen(s, b, path)
	if err == nil && root != "" {
		a.root, _ = filepath.Abs(root)
		a.refreshTree()
	}
	return a, err
}

func newAppWithScreen(s tcell.Screen, b *Buffer, path string) (*App, error) {
	if err := s.Init(); err != nil {
		return nil, err
	}
	s.EnableMouse()
	s.EnablePaste()
	a := &App{s: s, buf: b, themes: Themes(), lineNums: true, engine: "claude", paneFollow: true, hlVer: -1,
		showTree: true, showTerm: true, pane: true, agent: true, treeOpen: map[string]bool{}}
	a.root, _ = os.Getwd()
	if path != "" {
		if abs, err := filepath.Abs(path); err == nil {
			if rel, err := filepath.Rel(a.root, abs); err != nil || strings.HasPrefix(rel, "..") {
				a.root = filepath.Dir(abs) // el archivo está fuera de la carpeta actual
			}
		}
	}
	a.have = map[string]bool{}
	for _, e := range []string{"claude", "codex"} {
		if _, err := exec.LookPath(e); err == nil {
			a.have[e] = true
		}
	}
	if !a.have["claude"] && a.have["codex"] {
		a.engine = "codex"
	}
	a.loadConfig()
	a.refreshTree()
	a.menus = buildMenus(a.themes)
	a.applyTheme()
	if path != "" {
		if _, err := os.Stat(path); err != nil {
			a.flash("Archivo nuevo: " + path)
		}
	}
	if a.msg == "" {
		a.flash("Bienvenido a tecla · F1 ayuda · F6 cambia de panel · Ctrl+K IA · F7 terminal")
	}
	go func() {
		for range time.Tick(265 * time.Millisecond) {
			_ = s.PostEvent(tcell.NewEventInterrupt(nil)) // reloj y parpadeo del cursor
		}
	}()
	return a, nil
}

func (a *App) T() *Theme { return &a.themes[a.ti] }

func (a *App) applyTheme() {
	a.s.SetStyle(a.T().Normal)
	a.s.SetCursorStyle(a.T().Cursor)
}

func (a *App) setTheme(i int) {
	a.ti = (i + len(a.themes)) % len(a.themes)
	a.applyTheme()
	a.saveConfig()
	a.flash("Tema: " + a.T().Name)
}

func (a *App) nextTheme() { a.setTheme(a.ti + 1) }

func (a *App) Run() {
	defer func() {
		if r := recover(); r != nil {
			a.s.Fini()
			rescue := a.buf.Path
			if rescue == "" {
				rescue = "sin-titulo"
			}
			rescue += ".rescate"
			a.term.close()
			_ = os.WriteFile(rescue, []byte(a.buf.Text()), 0o644)
			fmt.Fprintf(os.Stderr, "tecla se cayó; tu texto quedó en %s\n", rescue)
			panic(r)
		}
	}()
	for !a.quit {
		a.draw()
		ev := a.s.PollEvent()
		if ev == nil {
			break
		}
		a.handle(ev)
	}
	if a.job != nil && a.job.Running() {
		a.job.cancel()
	}
	a.term.close()
	a.s.Fini()
}

func (a *App) flash(m string) {
	a.msg = m
	a.msgAt = time.Now()
	time.AfterFunc(5*time.Second, func() { _ = a.s.PostEvent(tcell.NewEventInterrupt(nil)) })
}

// ---------- dibujo: utilidades ----------

func (a *App) put(x, y int, s string, st tcell.Style) int {
	for _, r := range s {
		a.s.SetContent(x, y, r, nil, st)
		x += runeW(r)
	}
	return x
}

// putw dibuja s recortado a maxw columnas.
func (a *App) putw(x, y int, s string, st tcell.Style, maxw int) int {
	end := x + maxw
	for _, r := range s {
		w := runeW(r)
		if x+w > end {
			break
		}
		a.s.SetContent(x, y, r, nil, st)
		x += w
	}
	return x
}

func (a *App) fill(x, y, w, h int, r rune, st tcell.Style) {
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			a.s.SetContent(i, j, r, nil, st)
		}
	}
}

func (a *App) box(x, y, w, h int, b Box, st tcell.Style) {
	for i := x + 1; i < x+w-1; i++ {
		a.s.SetContent(i, y, b.H, nil, st)
		a.s.SetContent(i, y+h-1, b.H, nil, st)
	}
	for j := y + 1; j < y+h-1; j++ {
		a.s.SetContent(x, j, b.V, nil, st)
		a.s.SetContent(x+w-1, j, b.V, nil, st)
	}
	a.s.SetContent(x, y, b.TL, nil, st)
	a.s.SetContent(x+w-1, y, b.TR, nil, st)
	a.s.SetContent(x, y+h-1, b.BL, nil, st)
	a.s.SetContent(x+w-1, y+h-1, b.BR, nil, st)
}

func (a *App) shadow(x, y, w, h int) {
	sw := a.T().ShadowW
	shade := func(i, j int) {
		c, comb, _, _ := a.s.GetContent(i, j)
		a.s.SetContent(i, j, c, comb, a.T().Shadow)
	}
	for j := y + 1; j <= y+h; j++ {
		for i := x + w; i < x+w+sw; i++ {
			shade(i, j)
		}
	}
	for i := x + sw; i < x+w; i++ {
		shade(i, y+h)
	}
}

func strW(s string) int { return runewidth.StringWidth(s) }

// ---------- dibujo principal ----------

func (a *App) layout(w, h int) {
	top, H := 1, h-2
	a.treeOn = a.showTree && w >= 60
	a.paneOn = a.pane && w >= 60
	a.termOn = a.showTerm && H >= 16
	treeW := max(20, min(34, w/7))
	paneW := max(32, min(72, w*3/10))
	center := func() int {
		c := w
		if a.treeOn {
			c -= treeW
		}
		if a.paneOn {
			c -= paneW
		}
		return c
	}
	if center() < 44 && a.treeOn {
		a.treeOn = false
	}
	if center() < 44 && a.paneOn {
		paneW = max(28, w-44)
		if center() < 40 {
			a.paneOn = false
		}
	}
	x0, x1 := 0, w
	if a.treeOn {
		a.rTree = rect{0, top, treeW, H}
		x0 = treeW
	}
	if a.paneOn {
		a.rAgent = rect{w - paneW, top, paneW, H}
		x1 = w - paneW
	}
	edH := H
	if a.termOn {
		th := max(6, H*3/10)
		a.rTerm = rect{x0, top + H - th, x1 - x0, th}
		edH = H - th
	}
	a.rEd = rect{x0, top, x1 - x0, edH}
	switch {
	case a.focus == focTree && !a.treeOn, a.focus == focTerm && !a.termOn, a.focus == focAgent && !a.paneOn:
		a.focus = focEditor
	}
	a.ty = top + 1
	a.th = max(1, edH-2)
	a.tx = x0 + 1
	a.tw = x1 - x0 - 2
	a.gw = 0
	if a.lineNums {
		a.gw = max(5, len(strconv.Itoa(len(a.buf.Lines)))+2)
	}
	a.cw = max(1, a.tw-a.gw)
}

func (a *App) scrollToCursor() {
	b := a.buf
	if b.Cur.Y < b.ScrollY {
		b.ScrollY = b.Cur.Y
	}
	if b.Cur.Y >= b.ScrollY+a.th {
		b.ScrollY = b.Cur.Y - a.th + 1
	}
	v := b.vcur()
	if v < b.ScrollX {
		b.ScrollX = max(0, v-a.cw/4)
	}
	if v >= b.ScrollX+a.cw {
		b.ScrollX = v - a.cw + a.cw/4 + 1
	}
}

// cursorAt marca dónde va el cursor; lo dibuja el editor (no la terminal) para que se vea en todos los temas.
func (a *App) cursorAt(x, y int) { a.curX, a.curY = x, y }

func (a *App) paintCursor() {
	if a.curX < 0 {
		return
	}
	// parpadeo estilo DOS; queda fijo justo después de teclear
	if time.Since(a.lastKey) > 600*time.Millisecond && (time.Since(a.lastKey)/(530*time.Millisecond))%2 == 1 {
		return
	}
	c, comb, st, _ := a.s.GetContent(a.curX, a.curY)
	fg, bg, _ := st.Decompose()
	a.s.SetContent(a.curX, a.curY, c, comb, st.Foreground(bg).Background(fg).Reverse(false))
}

func (a *App) draw() {
	T := a.T()
	w, h := a.s.Size()
	a.s.HideCursor()
	a.curX, a.curY = -1, -1
	a.fill(0, 0, w, h, ' ', T.Normal)
	if w < 40 || h < 10 {
		a.put(0, 0, "tecla: la ventana es muy chica", T.Normal)
		a.s.Show()
		return
	}
	a.layout(w, h)
	if a.follow {
		a.scrollToCursor()
		a.follow = false
	}
	a.drawMenuBar(w)
	if a.treeOn {
		a.drawTree()
	}
	a.drawWindow()
	if a.termOn {
		a.drawTerm()
	}
	if a.paneOn {
		a.drawPane()
	}
	a.drawStatus(w, h)
	if a.menuOpen {
		a.drawDropdown(w)
	}
	if a.dlg != nil {
		a.curX, a.curY = -1, -1
		a.dlg.Draw(a)
	} else if !a.menuOpen && a.focus == focEditor {
		b := a.buf
		cx := a.tx + a.gw + b.vcur() - b.ScrollX
		cy := a.ty + b.Cur.Y - b.ScrollY
		if cy >= a.ty && cy < a.ty+a.th && cx >= a.tx+a.gw && cx < a.tx+a.tw {
			a.cursorAt(cx, cy)
		}
	}
	a.paintCursor()
	a.s.Show()
}

func (a *App) drawMenuBar(w int) {
	T := a.T()
	a.fill(0, 0, w, 1, ' ', T.MenuBar)
	x := 1
	if T.Logo != "" {
		a.put(x+1, 0, T.Logo, T.MenuBar)
		x += 4
	}
	a.menuX = a.menuX[:0]
	for i, m := range a.menus {
		a.menuX = append(a.menuX, x)
		base, hot := T.MenuBar, T.MenuHot
		if a.menuOpen && a.mi == i {
			base, hot = T.MenuSel, T.MenuSelHot
		}
		label := []rune(" " + m.Title + " ")
		for j, r := range label {
			s := base
			if j-1 == m.Hot {
				s = hot
			}
			a.s.SetContent(x+j, 0, r, nil, s)
		}
		x += len(label) + 1
	}
	right := time.Now().Format("15:04") + " "
	if a.job != nil && a.job.Running() {
		right = spinner() + " " + engineName(a.job.Engine) + "  " + right
	}
	a.put(w-strW(right)-1, 0, right, T.MenuBar)
}

func spinner() string {
	fr := []string{"|", "/", "-", "\\"}
	return fr[(time.Now().UnixMilli()/150)%4]
}

func engineName(e string) string {
	if e == "codex" {
		return "Codex"
	}
	return "Claude"
}

func (a *App) fileTitle() string {
	b := a.buf
	name := "Sin título"
	if b.Path != "" {
		name = filepath.Base(b.Path)
	}
	if a.T().Upper {
		name = strings.ToUpper(name)
	}
	if b.Dirty {
		name = "•" + name
	}
	return name
}

func (a *App) drawWindow() {
	T := a.T()
	r := a.rEd
	bx := T.Box
	if a.focus == focEditor {
		bx = Double
	} else {
		bx = Single
	}
	a.box(r.x, r.y, r.w, r.h, bx, T.Frame)
	if T.MacTitle {
		for x := r.x + 1; x < r.x+r.w-1; x++ {
			a.s.SetContent(x, r.y, '≡', nil, T.Frame)
		}
		a.put(r.x+2, r.y, " □ ", T.Frame)
	}
	title := " " + a.fileTitle() + " "
	ts := T.Title
	if a.focus != focEditor {
		ts = T.Frame
	}
	a.putw(r.x+max(1, (r.w-strW(title))/2), r.y, title, ts, r.w-2)
	a.drawEditor()
	a.drawScrollbar(r.x + r.w - 1)
}

func (a *App) blockStates(l *Lang) []bool {
	b := a.buf
	if a.hlVer == b.Version && a.hlLang == l && len(a.hlCache) == len(b.Lines) {
		return a.hlCache
	}
	st := make([]bool, len(b.Lines))
	if l != nil && l.BS != "" {
		in := false
		for i, line := range b.Lines {
			st[i] = in
			in = highlight(line, l, in, nil, nil)
		}
	}
	a.hlCache, a.hlVer, a.hlLang = st, b.Version, l
	return st
}

func (a *App) drawEditor() {
	T := a.T()
	b := a.buf
	l := langFor(b.Path)
	states := a.blockStates(l)
	var sS, sE Pos
	hasSel := b.HasSel()
	if hasSel {
		sS, sE = b.SelRange()
	}
	inSel := func(y, x int) bool {
		p := Pos{y, x}
		return hasSel && !p.Less(sS) && p.Less(sE)
	}
	x0 := a.tx + a.gw
	styles := make([]tcell.Style, 0, 256)
	for r := 0; r < a.th; r++ {
		y := a.ty + r
		ly := b.ScrollY + r
		if ly >= len(b.Lines) {
			continue
		}
		if a.gw > 0 {
			num := fmt.Sprintf("%*d ", a.gw-1, ly+1)
			gs := T.Gutter
			if ly == b.Cur.Y {
				gs = gs.Bold(true)
			}
			a.put(a.tx, y, num, gs)
		}
		line := b.Lines[ly]
		if cap(styles) < len(line) {
			styles = make([]tcell.Style, len(line))
		}
		styles = styles[:len(line)]
		highlight(line, l, states[ly], T, styles)
		vc := 0
		for i, ch := range line {
			cwid := runeW(ch)
			if ch == '\t' {
				cwid = tabW - vc%tabW
			}
			s := styles[i]
			if inSel(ly, i) {
				s = T.Selection
			}
			sx := vc - b.ScrollX
			if ch == '\t' {
				for k := 0; k < cwid; k++ {
					if sx+k >= 0 && sx+k < a.cw {
						a.s.SetContent(x0+sx+k, y, ' ', nil, s)
					}
				}
			} else if sx >= 0 && sx+cwid <= a.cw {
				a.s.SetContent(x0+sx, y, ch, nil, s)
			}
			vc += cwid
			if vc-b.ScrollX >= a.cw {
				break
			}
		}
		if hasSel && ly < sE.Y && inSel(ly, len(line)) {
			sx := vcol(line, len(line)) - b.ScrollX
			if sx >= 0 && sx < a.cw {
				a.s.SetContent(x0+sx, y, ' ', nil, T.Selection)
			}
		}
	}
}

func (a *App) drawScrollbar(x int) {
	T := a.T()
	b := a.buf
	n := a.th
	if n < 3 {
		return
	}
	a.s.SetContent(x, a.ty, '↑', nil, T.Scroll)
	a.s.SetContent(x, a.ty+n-1, '↓', nil, T.Scroll)
	track := n - 2
	for i := 0; i < track; i++ {
		a.s.SetContent(x, a.ty+1+i, '░', nil, T.Scroll)
	}
	pos := 0
	if total := len(b.Lines); total > a.th && track > 1 {
		pos = b.ScrollY * (track - 1) / max(1, total-a.th)
		pos = max(0, min(track-1, pos))
	}
	a.s.SetContent(x, a.ty+1+pos, '█', nil, T.Thumb)
}

func (a *App) drawStatus(w, h int) {
	T := a.T()
	y := h - 1
	a.fill(0, y, w, 1, ' ', T.Status)
	b := a.buf
	right := fmt.Sprintf(" %s%s │ %05d:%03d ", engineName(a.engine), map[bool]string{true: "·agente", false: ""}[a.agent], b.Cur.Y+1, b.vcur()+1)
	left := " <F1=Ayuda> <Esc=Menú> <F6=Panel> <Ctrl+K=IA> <F7=Terminal> <F12=Ejecutar>"
	ls := T.Status
	switch a.focus {
	case focAgent:
		left = " Agente: Enter enviar · Tab modo · ↑↓ historial · PgUp/PgDn desplazar · F5 aplicar · Esc volver"
	case focTerm:
		left = " Terminal: F6 otro panel · F7 volver al editor · F12 ejecutar archivo"
	case focTree:
		left = " Archivos: Enter abrir · ←→ plegar · c comparar · r usar como raíz · Retroceso subir · Esc volver"
	}
	if a.msg != "" && time.Since(a.msgAt) < 5*time.Second {
		left = " " + a.msg
		ls = T.StatusMsg
	}
	a.putw(0, y, left, ls, w-strW(right))
	a.put(w-strW(right), y, right, T.Status)
}

// ---------- eventos ----------

func (a *App) handle(ev tcell.Event) {
	switch ev := ev.(type) {
	case *tcell.EventResize:
		a.s.Sync()
	case *tcell.EventPaste:
		if ev.Start() {
			a.pasting = true
			a.pasteBuf = a.pasteBuf[:0]
		} else {
			a.pasting = false
			a.doPaste(string(a.pasteBuf))
		}
	case *tcell.EventKey:
		if a.pasting {
			switch ev.Key() {
			case tcell.KeyRune:
				a.pasteBuf = append(a.pasteBuf, ev.Rune())
			case tcell.KeyEnter, tcell.KeyCtrlJ:
				a.pasteBuf = append(a.pasteBuf, '\n')
			case tcell.KeyTab:
				a.pasteBuf = append(a.pasteBuf, '\t')
			}
			return
		}
		a.handleKey(ev)
	case *tcell.EventMouse:
		a.handleMouse(ev)
	case *tcell.EventInterrupt:
		if j, ok := ev.Data().(*AIJob); ok && !j.handled {
			j.handled = true
			a.aiDone(j)
		}
	}
}

func (a *App) doPaste(t string) {
	if a.dlg != nil {
		if p, ok := a.dlg.(interface{ Paste(string) }); ok {
			p.Paste(t)
		}
		return
	}
	if a.menuOpen {
		return
	}
	switch a.focus {
	case focTerm:
		a.termPaste(t)
		return
	case focAgent:
		a.chat.insert(strings.ReplaceAll(normalizeNL(t), "\n", " "))
		return
	case focTree:
		return
	}
	a.buf.InsertText(t, "paste")
	a.buf.lastOp = ""
	a.follow = true
}

func (a *App) handleKey(ev *tcell.EventKey) {
	a.lastKey = time.Now()
	if a.dlg != nil {
		a.dlg.Key(a, ev)
		return
	}
	if a.menuOpen {
		a.menuKey(ev)
		return
	}
	if a.focus == focTerm && !(ev.Key() >= tcell.KeyF1 && ev.Key() <= tcell.KeyF64) {
		a.termKey(ev) // en la terminal sólo las teclas de función son de tecla
		return
	}
	switch ev.Key() {
	case tcell.KeyF1:
		a.showHelp()
		return
	case tcell.KeyF2, tcell.KeyCtrlT:
		a.toggleEngine()
		return
	case tcell.KeyF3:
		a.findNext()
		return
	case tcell.KeyF4:
		a.togglePane()
		return
	case tcell.KeyF5, tcell.KeyCtrlE:
		a.applyAI()
		return
	case tcell.KeyF6:
		a.cycleFocus(ev.Modifiers()&tcell.ModShift != 0)
		return
	case tcell.KeyF7:
		a.toggleTermFocus()
		return
	case tcell.KeyF8:
		a.cancelAI()
		return
	case tcell.KeyF9:
		if ev.Modifiers()&tcell.ModCtrl != 0 {
			a.runFile()
		} else {
			a.nextTheme()
		}
		return
	case tcell.KeyF12:
		a.runFile()
		return
	case tcell.KeyCtrlB:
		a.toggleTree()
		return
	case tcell.KeyF10:
		a.openMenu(0)
		return
	case tcell.KeyCtrlQ:
		a.tryQuit()
		return
	case tcell.KeyCtrlK:
		a.askAI()
		return
	case tcell.KeyCtrlS:
		a.save(nil)
		return
	case tcell.KeyCtrlO:
		a.openFile()
		return
	case tcell.KeyCtrlN:
		a.newFile()
		return
	case tcell.KeyCtrlF:
		a.findDialog()
		return
	case tcell.KeyCtrlR:
		a.replaceDialog()
		return
	case tcell.KeyCtrlG:
		a.gotoDialog()
		return
	}
	switch a.focus {
	case focAgent:
		a.agentKey(ev)
		return
	case focTree:
		a.treeKey(ev)
		return
	}
	a.editorKey(ev)
	a.follow = true
}

func (a *App) editorKey(ev *tcell.EventKey) {
	b := a.buf
	mod := ev.Modifiers()
	shift := mod&tcell.ModShift != 0
	word := mod&(tcell.ModAlt|tcell.ModCtrl) != 0
	switch ev.Key() {
	case tcell.KeyRune:
		if mod&tcell.ModAlt != 0 {
			switch ev.Rune() {
			case 'b':
				b.WordLeft(false)
				return
			case 'f':
				b.WordRight(false)
				return
			}
			if a.menuHot(ev.Rune()) {
				return
			}
		}
		b.TypeRune(ev.Rune())
	case tcell.KeyEnter:
		b.Newline()
	case tcell.KeyTab:
		b.Tab()
	case tcell.KeyBacktab:
		b.Unindent()
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if word {
			b.DeleteWordBack()
		} else {
			b.Backspace()
		}
	case tcell.KeyDelete:
		b.DeleteFwd()
	case tcell.KeyLeft:
		if word {
			b.WordLeft(shift)
		} else {
			b.Left(shift)
		}
	case tcell.KeyRight:
		if word {
			b.WordRight(shift)
		} else {
			b.Right(shift)
		}
	case tcell.KeyUp:
		b.Up(shift, 1)
	case tcell.KeyDown:
		b.Down(shift, 1)
	case tcell.KeyPgUp:
		b.Up(shift, max(1, a.th-1))
	case tcell.KeyPgDn:
		b.Down(shift, max(1, a.th-1))
	case tcell.KeyHome:
		if mod&tcell.ModCtrl != 0 {
			b.DocStart(shift)
		} else {
			b.Home(shift)
		}
	case tcell.KeyEnd:
		if mod&tcell.ModCtrl != 0 {
			b.DocEnd(shift)
		} else {
			b.End(shift)
		}
	case tcell.KeyCtrlA:
		b.SelectAll()
	case tcell.KeyCtrlC:
		a.copy()
	case tcell.KeyCtrlX:
		a.cut()
	case tcell.KeyCtrlV:
		a.paste()
	case tcell.KeyCtrlZ:
		if !b.Undo() {
			a.flash("Nada para deshacer")
		}
	case tcell.KeyCtrlY:
		if !b.Redo() {
			a.flash("Nada para rehacer")
		}
	case tcell.KeyCtrlD:
		b.DuplicateLine()
	case tcell.KeyCtrlUnderscore:
		a.toggleComment()
	case tcell.KeyEsc:
		if b.Anchor != nil {
			b.Anchor = nil
		} else {
			a.openMenu(0)
		}
	}
}

// ---------- ratón ----------

func (a *App) posAt(x, y int) Pos {
	b := a.buf
	ly := max(0, min(len(b.Lines)-1, b.ScrollY+y-a.ty))
	v := max(0, x-(a.tx+a.gw)+b.ScrollX)
	return Pos{ly, xFromV(b.Lines[ly], v)}
}

func (a *App) handleMouse(ev *tcell.EventMouse) {
	x, y := ev.Position()
	btn := ev.Buttons()
	b := a.buf
	inPane := a.paneOn && a.rAgent.has(x, y)
	inTree := a.treeOn && a.rTree.has(x, y)
	inTerm := a.termOn && a.rTerm.has(x, y)
	if btn&tcell.WheelUp != 0 || btn&tcell.WheelDown != 0 {
		d := 3
		if btn&tcell.WheelUp != 0 {
			d = -3
		}
		if v, ok := a.dlg.(*DiffView); ok {
			v.Wheel(d)
			return
		}
		if a.dlg != nil || a.menuOpen {
			return
		}
		switch {
		case inPane:
			a.scrollPane(d)
		case inTree:
			a.treeOff = max(0, a.treeOff+d)
			a.treeSel = max(a.treeOff, min(a.treeSel, a.treeOff+a.rTree.h-3))
		case inTerm:
			// sin historial de desplazamiento por ahora
		default:
			b.ScrollY = max(0, min(len(b.Lines)-1, b.ScrollY+d))
		}
		return
	}
	if btn&tcell.Button1 == 0 {
		if a.mouseDown && a.dragging && b.Anchor != nil && *b.Anchor == b.Cur {
			b.Anchor = nil
		}
		a.mouseDown, a.dragging = false, false
		return
	}
	if a.mouseDown {
		if a.dragging {
			if y < a.ty && b.ScrollY > 0 {
				b.ScrollY--
			} else if y >= a.ty+a.th && b.ScrollY < len(b.Lines)-1 {
				b.ScrollY++
			}
			b.Cur = a.posAt(x, y)
			b.WantV = b.vcur()
		}
		return
	}
	a.mouseDown = true
	if v, ok := a.dlg.(*DiffView); ok {
		v.Click(x, y)
	}
	if a.dlg != nil {
		return
	}
	if a.menuOpen {
		m := a.menus[a.mi]
		if x > a.ddX && x < a.ddX+a.ddW-1 && y >= 2 && y < 2+len(m.Items) {
			a.execItem(a.mi, y-2)
			return
		}
		if y == 0 {
			if i := a.menuAt(x); i >= 0 && i != a.mi {
				a.openMenu(i)
				return
			}
		}
		a.menuOpen = false
		return
	}
	if y == 0 {
		if i := a.menuAt(x); i >= 0 {
			a.openMenu(i)
		}
		return
	}
	if inPane {
		a.focus = focAgent
		return
	}
	if inTree {
		a.treeMouse(y)
		return
	}
	if inTerm {
		a.focus = focTerm
		return
	}
	if !a.rEd.has(x, y) {
		return
	}
	a.focus = focEditor
	if x == a.rEd.x+a.rEd.w-1 && y >= a.ty && y < a.ty+a.th {
		switch {
		case y == a.ty:
			b.ScrollY = max(0, b.ScrollY-1)
		case y == a.ty+a.th-1:
			b.ScrollY = min(len(b.Lines)-1, b.ScrollY+1)
		default:
			frac := float64(y-a.ty-1) / float64(max(1, a.th-3))
			b.ScrollY = max(0, min(len(b.Lines)-1, int(frac*float64(max(0, len(b.Lines)-a.th)))))
		}
		return
	}
	if y >= a.ty && y < a.ty+a.th && x >= a.tx {
		p := a.posAt(x, y)
		b.lastOp = ""
		if time.Since(a.lastClick) < 400*time.Millisecond && p == a.lastPos {
			s, e := b.WordAt(p)
			b.SetSel(s, e)
			a.lastClick = time.Time{}
			return
		}
		a.lastClick, a.lastPos = time.Now(), p
		b.Anchor = &Pos{p.Y, p.X}
		b.Cur = p
		b.WantV = b.vcur()
		a.dragging = true
	}
}

// ---------- portapapeles ----------

func (a *App) setClip(t string) {
	a.clip = t
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(t)
	_ = cmd.Run()
}

func (a *App) getClip() string {
	if out, err := exec.Command("pbpaste").Output(); err == nil {
		return string(out)
	}
	return a.clip
}

func (a *App) copy() {
	b := a.buf
	if !b.HasSel() {
		a.setClip(string(b.Lines[b.Cur.Y]) + "\n")
		a.flash("Línea copiada")
		return
	}
	s, e := b.SelRange()
	a.setClip(b.TextRange(s, e))
	a.flash("Copiado")
}

func (a *App) cut() {
	b := a.buf
	if !b.HasSel() {
		y := b.Cur.Y
		s := Pos{y, 0}
		e := Pos{y, len(b.Lines[y])}
		if y+1 < len(b.Lines) {
			e = Pos{y + 1, 0}
		}
		b.SetSel(s, e)
	}
	s, e := b.SelRange()
	a.setClip(b.TextRange(s, e))
	b.DeleteSel()
}

func (a *App) paste() {
	if t := a.getClip(); t != "" {
		a.buf.InsertText(t, "paste")
		a.buf.lastOp = ""
	}
}

func (a *App) toggleComment() {
	l := langFor(a.buf.Path)
	p := l.lineComment()
	if p == "" {
		a.flash("No sé cómo comentar este tipo de archivo")
		return
	}
	a.buf.ToggleComment(p)
}

// ---------- archivos ----------

func (a *App) save(after func()) bool {
	b := a.buf
	if b.Path == "" {
		a.saveAs(after)
		return false
	}
	if err := b.Save(b.Path); err != nil {
		a.msgBox("Error al guardar", err.Error())
		return false
	}
	a.flash(fmt.Sprintf("Guardado %s (%d líneas)", b.Path, len(b.Lines)))
	a.refreshTree()
	if after != nil {
		after()
	}
	return true
}

func (a *App) saveAs(after func()) {
	a.dlg = NewInput("Guardar como", "Nombre del archivo:", a.buf.Path, func(a *App, p string) {
		p = strings.TrimSpace(expandHome(p))
		if p == "" {
			return
		}
		do := func() {
			a.buf.Path = p
			if langFor(p) != nil && len(a.buf.Lines) <= 1 {
				a.buf.IndentTab = defaultTabs(p)
			}
			a.save(after)
		}
		if _, err := os.Stat(p); err == nil && p != a.buf.Path {
			a.dlg = &ConfirmDialog{Title: "Guardar como", Lines: []string{filepath.Base(p) + " ya existe.", "¿Reemplazarlo?"},
				Buttons: []string{"Sí", "No"}, Do: func(a *App, i int) {
					if i == 0 {
						do()
					}
				}}
			return
		}
		do()
	})
}

func (a *App) confirmDiscard(then func()) {
	if !a.buf.Dirty {
		then()
		return
	}
	name := "Sin título"
	if a.buf.Path != "" {
		name = filepath.Base(a.buf.Path)
	}
	a.dlg = &ConfirmDialog{
		Title:   "Cambios sin guardar",
		Lines:   []string{"El archivo " + name + " tiene cambios.", "¿Guardarlos antes de seguir?"},
		Buttons: []string{"Sí", "No", "Cancelar"},
		Do: func(a *App, i int) {
			switch i {
			case 0:
				a.save(then)
			case 1:
				then()
			}
		},
	}
}

func (a *App) newFile() {
	a.confirmDiscard(func() {
		a.buf = NewBuffer("")
		a.flash("Archivo nuevo")
	})
}

func (a *App) openFile() {
	a.confirmDiscard(func() { a.dlg = NewOpenDialog(a) })
}

func (a *App) openPath(p string) {
	nb, err := LoadBuffer(p)
	if err != nil {
		a.msgBox("No se pudo abrir", err.Error())
		return
	}
	a.buf = nb
	a.hlVer = -1
	if _, err := os.Stat(p); err != nil {
		a.flash("Archivo nuevo: " + p)
	} else {
		a.flash(fmt.Sprintf("%s · %d líneas", p, len(nb.Lines)))
	}
}

func (a *App) reload() {
	b := a.buf
	if b.Path == "" {
		return
	}
	do := func() {
		data, err := os.ReadFile(b.Path)
		if err != nil {
			a.msgBox("No se pudo recargar", err.Error())
			return
		}
		b.ReloadText(string(data))
		a.flash("Recargado desde el disco · Ctrl+Z deshace")
	}
	if b.Dirty {
		a.dlg = &ConfirmDialog{Title: "Recargar", Lines: []string{"Vas a perder los cambios sin guardar", "(puedes deshacer con Ctrl+Z)."},
			Buttons: []string{"Recargar", "Cancelar"}, Do: func(a *App, i int) {
				if i == 0 {
					do()
				}
			}}
		return
	}
	do()
}

func (a *App) tryQuit() {
	a.confirmDiscard(func() { a.quit = true })
}

// ---------- búsqueda ----------

func (a *App) findDialog() {
	init := a.lastFind
	if b := a.buf; b.HasSel() {
		if s, e := b.SelRange(); s.Y == e.Y {
			init = b.TextRange(s, e)
		}
	}
	a.dlg = NewInput("Buscar", "Texto a buscar (en minúsculas = ignora mayúsculas):", init, func(a *App, q string) {
		a.lastFind = q
		a.findNext()
	})
}

func (a *App) findNext() {
	if a.lastFind == "" {
		a.findDialog()
		return
	}
	b := a.buf
	s, e, ok := b.Find(a.lastFind, b.Cur)
	if !ok {
		a.flash("No se encontró «" + a.lastFind + "»")
		return
	}
	b.SetSel(s, e)
	a.follow = true
}

func (a *App) replaceDialog() {
	init := a.lastFind
	if b := a.buf; b.HasSel() {
		if s, e := b.SelRange(); s.Y == e.Y {
			init = b.TextRange(s, e)
		}
	}
	a.dlg = NewInput("Reemplazar", "Buscar:", init, func(a *App, q string) {
		if q == "" {
			return
		}
		a.lastFind = q
		a.dlg = NewInput("Reemplazar", "Reemplazar «"+q+"» por:", "", func(a *App, r string) {
			n := a.buf.ReplaceAll(q, r)
			a.flash(fmt.Sprintf("%d reemplazo(s)", n))
		})
	})
}

func (a *App) gotoDialog() {
	a.dlg = NewInput("Ir a línea", fmt.Sprintf("Número de línea (1-%d):", len(a.buf.Lines)), "", func(a *App, s string) {
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
			a.buf.GotoLine(n)
			a.follow = true
		}
	})
}

// ---------- panel ----------

func (a *App) togglePane() {
	a.pane = !a.pane
	if a.pane {
		a.focus = focAgent
	} else if a.focus == focAgent {
		a.focus = focEditor
	}
	a.saveConfig()
}

func (a *App) toggleTree() {
	a.showTree = !a.showTree
	if a.showTree {
		a.focus = focTree
		a.refreshTree()
	} else if a.focus == focTree {
		a.focus = focEditor
	}
	a.saveConfig()
}

func (a *App) toggleTerm() {
	a.showTerm = !a.showTerm
	if !a.showTerm && a.focus == focTerm {
		a.focus = focEditor
	}
	a.saveConfig()
}

// toggleTermFocus (F7): va a la terminal, o vuelve al editor si ya estaba ahí.
func (a *App) toggleTermFocus() {
	if a.focus == focTerm {
		a.focus = focEditor
		return
	}
	a.showTerm = true
	a.focus = focTerm
}

// cycleFocus (F6) recorre los paneles visibles: árbol → editor → terminal → agente.
func (a *App) cycleFocus(back bool) {
	order := []int{focEditor}
	if a.treeOn {
		order = []int{focTree, focEditor}
	}
	if a.termOn {
		order = append(order, focTerm)
	}
	if a.paneOn {
		order = append(order, focAgent)
	}
	i := 0
	for k, f := range order {
		if f == a.focus {
			i = k
		}
	}
	d := 1
	if back {
		d = len(order) - 1
	}
	a.focus = order[(i+d)%len(order)]
	if a.focus == focTree {
		a.refreshTree()
	}
}

func (a *App) toggleEngine() {
	next := "codex"
	if a.engine == "codex" {
		next = "claude"
	}
	a.engine = next
	a.saveConfig()
	if !a.have[next] {
		a.flash(engineName(next) + " no está instalado (no encontré `" + next + "` en el PATH)")
		return
	}
	a.flash("Motor de IA: " + engineName(next))
}

// ---------- configuración ----------

func configPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "tecla", "config")
}

func (a *App) loadConfig() {
	data, err := os.ReadFile(configPath())
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch k {
		case "tema":
			for i, t := range a.themes {
				if strings.EqualFold(t.Name, v) {
					a.ti = i
				}
			}
		case "motor":
			if v == "claude" || v == "codex" {
				a.engine = v
			}
		case "numeros":
			a.lineNums = v != "0"
		case "arbol":
			a.showTree = v != "0"
		case "terminal":
			a.showTerm = v != "0"
		case "panel_ia":
			a.pane = v != "0"
		case "agente":
			a.agent = v != "0"
		}
	}
}

func (a *App) saveConfig() {
	p := configPath()
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	b := func(v bool) string {
		if v {
			return "1"
		}
		return "0"
	}
	_ = os.WriteFile(p, []byte(fmt.Sprintf("tema=%s\nmotor=%s\nnumeros=%s\narbol=%s\nterminal=%s\npanel_ia=%s\nagente=%s\n",
		a.T().Name, a.engine, b(a.lineNums), b(a.showTree), b(a.showTerm), b(a.pane), b(a.agent))), 0o644)
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[1:])
	}
	return p
}
