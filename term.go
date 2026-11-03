package main

import (
	"bufio"
	"os"
	"os/exec"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/creack/pty"
	"github.com/gdamore/tcell/v2"
	"github.com/hinshun/vt10x"
	"golang.org/x/sys/unix"
)

// Term es una terminal real (shell sobre un PTY) emulada con vt10x y dibujada dentro de tecla.
type Term struct {
	vt         vt10x.Terminal
	pty        *os.File
	cmd        *exec.Cmd
	cols, rows int

	mu   sync.Mutex
	dead bool
}

func (t *Term) Dead() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.dead
}

func (a *App) startTerm(cols, rows int) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	cmd := exec.Command(shell, "-l")
	cmd.Dir = a.root
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor", "TECLA=1")
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	if err != nil {
		a.term = nil
		a.flash("No se pudo abrir la terminal: " + err.Error())
		return
	}
	t := &Term{pty: f, cmd: cmd, cols: cols, rows: rows}
	t.vt = vt10x.New(vt10x.WithWriter(f), vt10x.WithSize(cols, rows))
	a.term = t
	go func() {
		br := bufio.NewReaderSize(f, 1<<16)
		for {
			if err := t.vt.Parse(br); err != nil {
				break
			}
			_ = a.s.PostEvent(tcell.NewEventInterrupt(nil))
		}
		_ = cmd.Wait()
		t.mu.Lock()
		t.dead = true
		t.mu.Unlock()
		f.Close()
		_ = a.s.PostEvent(tcell.NewEventInterrupt(nil))
	}()
}

func (t *Term) resize(cols, rows int) {
	if cols == t.cols && rows == t.rows {
		return
	}
	t.cols, t.rows = cols, rows
	t.vt.Resize(cols, rows)
	_ = pty.Setsize(t.pty, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}

func (t *Term) write(b []byte) {
	if t == nil || t.Dead() {
		return
	}
	_, _ = t.pty.Write(b)
}

// idle dice si el shell está esperando un comando (no hay otro programa en primer plano).
func (t *Term) idle() bool {
	if t == nil || t.Dead() || t.cmd.Process == nil {
		return false
	}
	pg, err := unix.IoctlGetInt(int(t.pty.Fd()), unix.TIOCGPGRP)
	return err == nil && pg == t.cmd.Process.Pid
}

func (t *Term) close() {
	if t == nil || t.Dead() {
		return
	}
	if t.cmd.Process != nil {
		_ = t.cmd.Process.Signal(unix.SIGHUP)
	}
	t.pty.Close()
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// runInTerm escribe un comando en la terminal y la enfoca.
func (a *App) runInTerm(command string) {
	a.showTerm = true
	a.focus = focTerm
	a.pendingCmd = command
	a.ensureTerm()
}

// ensureTerm arranca (o reinicia) el shell y manda el comando pendiente, si lo hay.
func (a *App) ensureTerm() {
	if !a.termOn || a.rTerm.w < 6 {
		return // se arranca en el próximo draw, cuando conozcamos el tamaño
	}
	cols, rows := a.rTerm.w-2, a.rTerm.h-2
	if a.term == nil || a.term.Dead() {
		a.startTerm(cols, rows)
	} else {
		a.term.resize(cols, rows)
	}
	if a.pendingCmd != "" && a.term != nil {
		a.term.write([]byte("\x15" + a.pendingCmd + "\r")) // Ctrl+U limpia lo que hubiera escrito
		a.pendingCmd = ""
	}
}

var ansi16 = []int32{
	0x000000, 0xAA0000, 0x00AA00, 0xAA5500, 0x0000AA, 0xAA00AA, 0x00AAAA, 0xAAAAAA,
	0x555555, 0xFF5555, 0x55FF55, 0xFFFF55, 0x5555FF, 0xFF55FF, 0x55FFFF, 0xFFFFFF,
}

func vtColor(c vt10x.Color, fg, bg tcell.Color) tcell.Color {
	switch {
	case c == vt10x.DefaultFG:
		return fg
	case c >= vt10x.DefaultBG:
		return bg
	case c < 16:
		return hex(ansi16[c]) // paleta CGA, para que encaje con la estética
	case c < 256:
		return tcell.PaletteColor(int(c))
	default:
		return hex(int32(c))
	}
}

const (
	vtReverse   = 1 << 0
	vtUnderline = 1 << 1
	vtBold      = 1 << 2
	vtItalic    = 1 << 4
)

func (a *App) drawTerm() {
	T := a.T()
	r := a.rTerm
	focused := a.focus == focTerm
	a.fill(r.x, r.y, r.w, r.h, ' ', T.Pane)
	a.frame(r, focused, T.PaneFrame)
	title := " Terminal "
	if t := a.term; t != nil && !t.Dead() {
		if tt := strings.TrimSpace(t.vt.Title()); tt != "" {
			title = " " + tt + " "
		}
	}
	ts := T.PaneFrame
	if focused {
		ts = T.Title
	}
	a.putw(r.x+2, r.y, title, ts, r.w-4)
	a.ensureTerm()
	t := a.term
	if t == nil {
		return
	}
	if t.Dead() {
		a.putw(r.x+2, r.y+1, "[el shell terminó · Enter para abrir otro]", T.PaneDim, r.w-4)
		return
	}
	defFG, defBG, _ := T.Pane.Decompose()
	t.vt.Lock()
	cols, rows := t.vt.Size()
	for y := 0; y < rows && y < r.h-2; y++ {
		for x := 0; x < cols && x < r.w-2; x++ {
			g := t.vt.Cell(x, y)
			ch := g.Char
			if ch == 0 {
				ch = ' '
			}
			st := tcell.StyleDefault.Foreground(vtColor(g.FG, defFG, defBG)).Background(vtColor(g.BG, defFG, defBG))
			st = st.Bold(g.Mode&vtBold != 0).Underline(g.Mode&vtUnderline != 0).Italic(g.Mode&vtItalic != 0)
			a.s.SetContent(r.x+1+x, r.y+1+y, ch, nil, st)
		}
	}
	cur := t.vt.Cursor()
	vis := t.vt.CursorVisible()
	t.vt.Unlock()
	if focused && vis && a.dlg == nil && !a.menuOpen {
		a.cursorAt(r.x+1+cur.X, r.y+1+cur.Y)
	}
}

// termKey traduce una tecla de tcell a los bytes que espera un programa de terminal.
func (a *App) termKey(ev *tcell.EventKey) {
	t := a.term
	if t == nil || t.Dead() {
		if ev.Key() == tcell.KeyEnter {
			a.term = nil
			a.ensureTerm()
		}
		return
	}
	appCur := false
	t.vt.Lock()
	appCur = t.vt.Mode()&vt10x.ModeAppCursor != 0
	t.vt.Unlock()
	arrow := func(c byte) []byte {
		if appCur {
			return []byte{0x1b, 'O', c}
		}
		return []byte{0x1b, '[', c}
	}
	alt := ev.Modifiers()&tcell.ModAlt != 0
	var b []byte
	switch k := ev.Key(); k {
	case tcell.KeyRune:
		var buf [utf8.UTFMax]byte
		n := utf8.EncodeRune(buf[:], ev.Rune())
		b = buf[:n]
		if alt {
			b = append([]byte{0x1b}, b...)
		}
	case tcell.KeyEnter:
		b = []byte{'\r'}
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		b = []byte{0x7f}
		if alt {
			b = []byte{0x1b, 0x7f}
		}
	case tcell.KeyTab:
		b = []byte{'\t'}
	case tcell.KeyBacktab:
		b = []byte("\x1b[Z")
	case tcell.KeyEsc:
		b = []byte{0x1b}
	case tcell.KeyUp:
		b = arrow('A')
	case tcell.KeyDown:
		b = arrow('B')
	case tcell.KeyRight:
		b = arrow('C')
		if alt || ev.Modifiers()&tcell.ModCtrl != 0 {
			b = []byte("\x1bf")
		}
	case tcell.KeyLeft:
		b = arrow('D')
		if alt || ev.Modifiers()&tcell.ModCtrl != 0 {
			b = []byte("\x1bb")
		}
	case tcell.KeyHome:
		b = []byte("\x1b[H")
	case tcell.KeyEnd:
		b = []byte("\x1b[F")
	case tcell.KeyPgUp:
		b = []byte("\x1b[5~")
	case tcell.KeyPgDn:
		b = []byte("\x1b[6~")
	case tcell.KeyDelete:
		b = []byte("\x1b[3~")
	case tcell.KeyInsert:
		b = []byte("\x1b[2~")
	default:
		if k < 32 { // Ctrl+letra: tcell usa el mismo código ASCII
			b = []byte{byte(k)}
		}
	}
	if len(b) > 0 {
		t.write(b)
	}
}

func (a *App) termPaste(s string) {
	s = strings.ReplaceAll(normalizeNL(s), "\n", "\r")
	a.term.write([]byte(s))
}

// ---------- ejecutar el archivo actual ----------

func runCommand(path string) string {
	q := shellQuote(path)
	switch strings.ToLower(extOf(path)) {
	case ".py":
		return "python3 " + q
	case ".go":
		return "go run " + q
	case ".js", ".mjs", ".cjs":
		return "node " + q
	case ".ts":
		return "npx tsx " + q
	case ".rb":
		return "ruby " + q
	case ".sh", ".bash":
		return "bash " + q
	case ".zsh":
		return "zsh " + q
	case ".lua":
		return "lua " + q
	case ".pl":
		return "perl " + q
	case ".php":
		return "php " + q
	case ".swift":
		return "swift " + q
	case ".c":
		return "cc " + q + " -o /tmp/tecla-run && /tmp/tecla-run"
	case ".cpp", ".cc":
		return "c++ " + q + " -o /tmp/tecla-run && /tmp/tecla-run"
	case ".rs":
		return "rustc " + q + " -o /tmp/tecla-run && /tmp/tecla-run"
	}
	return ""
}

func extOf(p string) string {
	if i := strings.LastIndexByte(p, '.'); i > strings.LastIndexByte(p, '/') {
		return p[i:]
	}
	return ""
}

func (a *App) runFile() {
	b := a.buf
	if b.Path == "" {
		a.msgBox("Ejecutar", "Guarda el archivo primero.")
		return
	}
	cmd := runCommand(a.relPath(b.Path))
	if cmd == "" {
		a.flash("No sé cómo ejecutar este tipo de archivo · usa la terminal (F7)")
		return
	}
	if b.Dirty && !a.save(nil) {
		return
	}
	a.runInTerm(cmd)
}
