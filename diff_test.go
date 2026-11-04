package main

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func toLines(s string) [][]rune {
	parts := strings.Split(s, "\n")
	out := make([][]rune, len(parts))
	for i, p := range parts {
		out[i] = []rune(p)
	}
	return out
}

func lcsLen(a, b []int) int {
	prev := make([]int, len(b)+1)
	for i := range a {
		cur := make([]int, len(b)+1)
		for j := range b {
			if a[i] == b[j] {
				cur[j+1] = prev[j] + 1
			} else {
				cur[j+1] = max(cur[j], prev[j+1])
			}
		}
		prev = cur
	}
	return prev[len(b)]
}

// checkMatch verifica que las parejas sean iguales y crecientes; devuelve cuántas hay.
func checkMatch(t *testing.T, a, b, m []int) int {
	t.Helper()
	last, n := -1, 0
	for i, j := range m {
		if j < 0 {
			continue
		}
		if j <= last || j >= len(b) || a[i] != b[j] {
			t.Fatalf("pareja inválida %d→%d en a=%v b=%v m=%v", i, j, a, b, m)
		}
		last = j
		n++
	}
	return n
}

func randSeq(r *rand.Rand, n, alphabet int) []int {
	s := make([]int, n)
	for i := range s {
		s[i] = r.Intn(alphabet)
	}
	return s
}

func TestDiffMatchIsMinimal(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 3000; i++ {
		a, b := randSeq(r, r.Intn(40), 1+r.Intn(6)), randSeq(r, r.Intn(40), 1+r.Intn(6))
		if got, want := checkMatch(t, a, b, diffMatch(a, b)), lcsLen(a, b); got != want {
			t.Fatalf("a=%v b=%v: %d parejas, el óptimo es %d", a, b, got, want)
		}
	}
}

func TestDiffMatchWithLimitStaysValid(t *testing.T) {
	defer func(v int) { diffMaxD = v }(diffMaxD)
	diffMaxD = 4
	r := rand.New(rand.NewSource(2))
	for i := 0; i < 3000; i++ {
		a, b := randSeq(r, r.Intn(60), 1+r.Intn(5)), randSeq(r, r.Intn(60), 1+r.Intn(5))
		checkMatch(t, a, b, diffMatch(a, b))
	}
}

func TestRowsCoverBothSides(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	words := []string{"a b", "a c", "x := 1", "x := 2", "", "func f()", "return nil"}
	gen := func() [][]rune {
		ls := make([][]rune, 1+r.Intn(30))
		for i := range ls {
			ls[i] = []rune(words[r.Intn(len(words))])
		}
		return ls
	}
	for i := 0; i < 1000; i++ {
		L, R := gen(), gen()
		nl, nr := 0, 0
		for _, row := range computeDiff(L, R, false) {
			if row.l >= 0 {
				if row.l != nl {
					t.Fatalf("izquierda fuera de orden: %d, esperaba %d", row.l, nl)
				}
				nl++
			}
			if row.r >= 0 {
				if row.r != nr {
					t.Fatalf("derecha fuera de orden: %d, esperaba %d", row.r, nr)
				}
				nr++
			}
			if row.kind == dSame && string(L[row.l]) != string(R[row.r]) {
				t.Fatalf("fila igual con textos distintos")
			}
		}
		if nl != len(L) || nr != len(R) {
			t.Fatalf("faltan líneas: %d/%d y %d/%d", nl, len(L), nr, len(R))
		}
	}
}

func TestModifiedLineAlignsWithItsOldVersion(t *testing.T) {
	L := toLines("uno\nfunc suma(a, b int) int {\ntres")
	R := toLines("uno\n// nueva línea sin relación\nfunc suma(a, b, c int) int {\ntres")
	rows := computeDiff(L, R, false)
	if len(rows) != 4 || rows[1].kind != dAdd || rows[2].kind != dChg || rows[2].l != 1 || rows[2].r != 2 {
		t.Fatalf("alineación inesperada: %+v", rows)
	}
	var marked strings.Builder
	for i, on := range rows[2].rm {
		if on {
			marked.WriteRune(R[2][i])
		}
	}
	if got := marked.String(); got != "c," {
		// el agregado es «, c» o «c,» según dónde caiga el empate
		if got != ", c" && got != ",c" {
			t.Fatalf("resaltó %q", got)
		}
	}
	for i, on := range rows[2].lm {
		if on && L[1][i] != ' ' && L[1][i] != ',' {
			t.Fatalf("marcó de más en la izquierda: %q", string(L[1][i]))
		}
	}
}

func TestIgnoreWhitespace(t *testing.T) {
	L, R := toLines("if x {\n  y()\n}"), toLines("if x {\n\t\ty()\n}")
	if rows := computeDiff(L, R, false); rows[1].kind != dChg {
		t.Fatalf("debía ver el cambio: %+v", rows)
	}
	for _, r := range computeDiff(L, R, true) {
		if r.kind != dSame {
			t.Fatalf("debía ignorar espacios: %+v", r)
		}
	}
}

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for n, c := range files {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func numbered(n int, change map[int]string) string {
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		if c, ok := change[i]; ok {
			sb.WriteString(c)
		} else {
			sb.WriteString("linea igual ")
			sb.WriteString(strings.Repeat("x", i%7))
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func TestDiffViewInEveryThemeAndSize(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"a.py": numbered(80, map[int]string{10: "total = precio * 2", 60: "print('viejo')"}),
		"b.py": numbered(80, map[int]string{10: "total = precio * 3", 60: "print('nuevo')"}),
	})
	a, s := newTestApp(t, "")
	a.compareFiles(filepath.Join(dir, "a.py"), filepath.Join(dir, "b.py"))
	d, ok := a.dlg.(*DiffView)
	if !ok {
		t.Fatalf("no abrió la comparación: %T", a.dlg)
	}
	a.draw()
	out := screenText(s)
	for _, want := range []string{"precio * 2", "precio * 3", "líneas iguales", "+2", "−2", "cambio 1 de 2"} {
		if !strings.Contains(out, want) {
			t.Fatalf("falta %q en:\n%s", want, out)
		}
	}
	key(a, tcell.KeyRune, 'n', 0)
	if out = screenText(s); !strings.Contains(out, "'nuevo'") || !strings.Contains(out, "cambio 2 de 2") {
		t.Fatalf("n no saltó:\n%s", out)
	}
	key(a, tcell.KeyRune, 'n', 0) // ya en el último: se queda
	key(a, tcell.KeyRune, 'p', 0)
	if !strings.Contains(screenText(s), "cambio 1 de 2") {
		t.Fatalf("p no volvió:\n%s", screenText(s))
	}
	key(a, tcell.KeyRune, 'z', 0)
	if strings.Contains(screenText(s), "líneas iguales") {
		t.Fatal("z no desplegó")
	}
	key(a, tcell.KeyEnd, 0, 0)
	key(a, tcell.KeyHome, 0, 0)
	key(a, tcell.KeyRune, 'z', 0)
	key(a, tcell.KeyTab, 0, 0)
	if len(d.view) <= d.page && !d.uni {
		t.Fatal("Tab no cambió a la unificada")
	}
	for i := range a.themes {
		a.ti = i
		for _, sz := range [][2]int{{160, 40}, {79, 24}, {40, 10}} {
			s.SetSize(sz[0], sz[1])
			a.draw()
			key(a, tcell.KeyRight, 0, 0)
			key(a, tcell.KeyPgDn, 0, 0)
			key(a, tcell.KeyTab, 0, 0)
		}
	}
	s.SetSize(160, 40)
	key(a, tcell.KeyEsc, 0, 0)
	if a.dlg != nil {
		t.Fatal("Esc no cerró")
	}
}

func TestNarrowWindowUsesUnified(t *testing.T) {
	dir := writeFiles(t, map[string]string{"a.txt": "uno\ndos\n", "b.txt": "uno\ndos más\n"})
	a, s := newTestApp(t, "")
	a.compareFiles(filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt"))
	s.SetSize(70, 24)
	a.draw()
	if d := a.dlg.(*DiffView); !d.uni {
		t.Fatal("debía usar la vista unificada")
	}
	out := screenText(s)
	if !strings.Contains(out, "A.TXT → B.TXT") || !strings.Contains(out, "dos más") {
		t.Fatalf("unificada rara:\n%s", out)
	}
}

func TestCompareUsesUnsavedTextAndKeepsBuffer(t *testing.T) {
	dir := writeFiles(t, map[string]string{"a.txt": "uno\n", "otro.txt": "uno\n"})
	p := filepath.Join(dir, "a.txt")
	a, s := newTestApp(t, p)
	typeStr(a, "cambio ")
	buf := a.buf
	a.compareWith(filepath.Join(dir, "otro.txt"))
	a.draw()
	out := screenText(s)
	if !strings.Contains(out, "cambio uno") || !strings.Contains(out, "(SIN GUARDAR)") {
		t.Fatalf("no usó el texto sin guardar:\n%s", out)
	}
	key(a, tcell.KeyEsc, 0, 0)
	a.compareSaved()
	if _, ok := a.dlg.(*DiffView); !ok {
		t.Fatalf("versión guardada: %T", a.dlg)
	}
	key(a, tcell.KeyEsc, 0, 0)
	if a.buf != buf || !a.buf.Dirty {
		t.Fatal("la comparación tocó el buffer")
	}
}

func TestCompareMessages(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"a.txt": "uno\ndos\n", "igual.txt": "uno\ndos\n", "crlf.txt": "uno\r\ndos\r\n", "bin": "a\x00b",
	})
	f := func(n string) string { return filepath.Join(dir, n) }
	cases := []struct{ p, q, want string }{
		{"a.txt", "igual.txt", "No hay diferencias"},
		{"a.txt", "crlf.txt", "finales de línea"},
		{"a.txt", "bin", "binario"},
		{"a.txt", "falta.txt", "no existe"},
		{"a.txt", ".", "es una carpeta"},
	}
	for _, c := range cases {
		a, s := newTestApp(t, "")
		a.compareFiles(f(c.p), f(c.q))
		a.draw()
		if _, isDiff := a.dlg.(*DiffView); isDiff || !strings.Contains(screenText(s), c.want) {
			t.Fatalf("%s vs %s: esperaba %q:\n%s", c.p, c.q, c.want, screenText(s))
		}
	}
}

func TestPickDialogDoesNotOpenTheFile(t *testing.T) {
	dir := writeFiles(t, map[string]string{"a.txt": "uno\n", "b.txt": "dos\n"})
	a, s := newTestApp(t, filepath.Join(dir, "a.txt"))
	typeStr(a, "sucio ")
	buf := a.buf
	a.compareOther()
	a.draw()
	if !strings.Contains(screenText(s), "Comparar con...") {
		t.Fatalf("sin título:\n%s", screenText(s))
	}
	typeStr(a, "b.txt\n")
	if _, ok := a.dlg.(*DiffView); !ok || a.buf != buf {
		t.Fatalf("elegir archivo: dlg=%T, buffer cambiado=%v", a.dlg, a.buf != buf)
	}
}

func TestMenuTreeAndMouse(t *testing.T) {
	dir := writeFiles(t, map[string]string{"a.txt": numbered(200, map[int]string{150: "distinta"}), "b.txt": numbered(200, nil)})
	a, s := newTestApp(t, filepath.Join(dir, "a.txt"))
	a.draw()
	if !strings.Contains(screenText(s), "Comparar") {
		t.Fatalf("falta el menú:\n%s", screenText(s))
	}
	a.focus = focTree
	for i, n := range a.tree {
		if n.name == "b.txt" {
			a.treeSel = i
		}
	}
	key(a, tcell.KeyRune, 'c', 0)
	d, ok := a.dlg.(*DiffView)
	if !ok {
		t.Fatalf("c en el árbol: %T", a.dlg)
	}
	key(a, tcell.KeyRune, 'z', 0) // sin plegar, para que haya que desplazarse
	before := d.off
	a.handle(tcell.NewEventMouse(10, 10, tcell.WheelDown, 0))
	if d.off != before+3 {
		t.Fatalf("rueda: %d → %d", before, d.off)
	}
	a.handle(tcell.NewEventMouse(d.barX, d.barY, tcell.Button1, 0))
	a.handle(tcell.NewEventMouse(d.barX, d.barY, tcell.ButtonNone, 0))
	if d.off != 0 {
		t.Fatalf("clic en el mapa: off=%d", d.off)
	}
	a.draw()
}

func TestIgnoreWhitespaceKey(t *testing.T) {
	dir := writeFiles(t, map[string]string{"a.txt": "if x {\n  y()\n}\n", "b.txt": "if x {\n\t\ty()\n}\n"})
	a, s := newTestApp(t, "")
	a.compareFiles(filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt"))
	a.draw()
	key(a, tcell.KeyRune, 'w', 0)
	if !strings.Contains(screenText(s), "sin diferencias (ignorando espacios)") {
		t.Fatalf("w no ignoró los espacios:\n%s", screenText(s))
	}
	key(a, tcell.KeyRune, 'n', 0) // sin cambios a donde saltar: no debe romperse
	key(a, tcell.KeyRune, 'w', 0)
	if !strings.Contains(screenText(s), "cambio 1 de 1") {
		t.Fatalf("w no volvió:\n%s", screenText(s))
	}
}
