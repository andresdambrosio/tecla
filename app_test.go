package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

func newTestApp(t *testing.T, path string) (*App, tcell.SimulationScreen) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	s := tcell.NewSimulationScreen("UTF-8")
	b, err := LoadBuffer(path)
	if err != nil {
		t.Fatal(err)
	}
	a, err := newAppWithScreen(s, b, path)
	if err != nil {
		t.Fatal(err)
	}
	s.SetSize(160, 40)
	t.Cleanup(func() { a.term.close() })
	return a, s
}

func screenText(s tcell.SimulationScreen) string {
	cells, w, h := s.GetContents()
	var sb strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := cells[y*w+x]
			if len(c.Runes) > 0 {
				sb.WriteRune(c.Runes[0])
			} else {
				sb.WriteByte(' ')
			}
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func key(a *App, k tcell.Key, r rune, m tcell.ModMask) {
	a.handle(tcell.NewEventKey(k, r, m))
	a.draw()
}

func typeStr(a *App, s string) {
	for _, r := range s {
		if r == '\n' {
			key(a, tcell.KeyEnter, 0, 0)
		} else {
			key(a, tcell.KeyRune, r, 0)
		}
	}
}

func TestEditingAndRender(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "hola.py")
	a, s := newTestApp(t, p)
	typeStr(a, "def hola():\nprint('hola')\n")
	if got := a.buf.Text(); got != "def hola():\n    print('hola')\n    " {
		t.Fatalf("texto inesperado: %q", got)
	}
	key(a, tcell.KeyCtrlZ, 0, tcell.ModCtrl)
	key(a, tcell.KeyCtrlS, 0, tcell.ModCtrl)
	data, _ := os.ReadFile(p)
	t.Logf("guardado: %q", data)
	for i := range a.themes {
		a.setTheme(i)
		a.draw()
		out := screenText(s)
		if !strings.Contains(out, "Archivo") || !strings.Contains(out, "print") {
			t.Fatalf("pantalla rara en tema %s:\n%s", a.T().Name, out)
		}
		if i == 0 || i == 2 {
			t.Logf("Tema %s:\n%s", a.T().Name, out)
		}
	}
}

func TestMenusAndDialogs(t *testing.T) {
	a, s := newTestApp(t, "")
	typeStr(a, "uno dos tres\ncuatro dos\n")
	key(a, tcell.KeyEsc, 0, 0)
	key(a, tcell.KeyRight, 0, 0)
	t.Logf("Menú Editar:\n%s", screenText(s))
	key(a, tcell.KeyEsc, 0, 0)
	a.replaceDialog()
	a.draw()
	t.Logf("Diálogo:\n%s", screenText(s))
	key(a, tcell.KeyCtrlU, 0, tcell.ModCtrl)
	typeStr(a, "dos")
	key(a, tcell.KeyEnter, 0, 0)
	typeStr(a, "2")
	key(a, tcell.KeyEnter, 0, 0)
	if got := a.buf.Text(); got != "uno 2 tres\ncuatro 2\n" {
		t.Fatalf("reemplazo: %q", got)
	}
	a.dlg = NewOpenDialog(a)
	a.draw()
	a.askAI()
	a.draw()
	t.Logf("IA:\n%s", screenText(s))
	key(a, tcell.KeyEsc, 0, 0)
}

func TestApplyCode(t *testing.T) {
	a, _ := newTestApp(t, "")
	typeStr(a, "x = 1\ny = 2\n")
	b := a.buf
	b.SetSel(Pos{0, 0}, Pos{1, 5})
	j := &AIJob{buf: b, hadSel: true, selS: Pos{0, 0}, selE: Pos{1, 5}, selText: b.TextRange(Pos{0, 0}, Pos{1, 5})}
	j.out = []byte("Listo:\n```python\nx = 10\ny = 20\n```\nfin")
	a.job = j
	b.Anchor = nil
	b.Cur = Pos{2, 0}
	a.applyAI()
	if got := b.Text(); got != "x = 10\ny = 20\n" {
		t.Fatalf("aplicar: %q", got)
	}
	a.pane = true
	a.draw()
}

func TestTerminalAndTree(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "hola.py")
	os.WriteFile(p, []byte("print('hola desde python', 6*7)\n"), 0o644)
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "otro.txt"), []byte("x\n"), 0o644)
	a, s := newTestApp(t, p)
	a.draw()
	a.runFile()
	var out string
	for i := 0; i < 60; i++ {
		time.Sleep(100 * time.Millisecond)
		a.draw()
		out = screenText(s)
		if strings.Contains(out, "hola desde python 42") {
			break
		}
	}
	if !strings.Contains(out, "hola desde python 42") {
		t.Fatalf("la terminal no mostró la salida:\n%s", out)
	}
	// árbol: desplegar sub/ y abrir otro.txt
	a.focus = focTree
	for i, n := range a.tree {
		if n.name == "sub" {
			a.treeSel = i
		}
	}
	key(a, tcell.KeyEnter, 0, 0)
	key(a, tcell.KeyDown, 0, 0)
	key(a, tcell.KeyEnter, 0, 0)
	if filepath.Base(a.buf.Path) != "otro.txt" || a.focus != focEditor {
		t.Fatalf("no abrió desde el árbol: %q foco=%d", a.buf.Path, a.focus)
	}
	key(a, tcell.KeyF6, 0, 0)
	t.Logf("pantalla:\n%s", screenText(s))
}
