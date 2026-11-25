package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestMathInline(t *testing.T) {
	cases := map[string]string{
		`x^2 + y_i`:                            "x² + yᵢ",
		`a \neq 0`:                             "a ≠ 0",
		`\frac{1}{2}`:                          "½",
		`\frac{a+b}{c}`:                        "(a + b)/c",
		`e^{i\pi}`:                             "e^(iπ)",
		`\sqrt{x+1}`:                           "√(x + 1)",
		`\sqrt[3]{x}`:                          "∛x",
		`\alpha \to \infty`:                    "α → ∞",
		`-x`:                                   "−x",
		`\mathbb{R}^n`:                         "ℝⁿ",
		`\sin x`:                               "sin x",
		`f'(x)`:                                "f′(x)",
		`\left( a \right)`:                     "(a)",
		`\text{si } x`:                         "si x",
		`\begin{pmatrix}a&b\\c&d\end{pmatrix}`: "(a, b; c, d)",
		`\foo`:                                 `\foo`,
		`x \not\in A`:                          "x ∉ A",
		`A^T`:                                  "Aᵀ",
	}
	for src, want := range cases {
		if got := renderMath(src, false).String(); got != want {
			t.Errorf("%s: %q, quería %q", src, got, want)
		}
	}
}

func TestMathDisplay(t *testing.T) {
	cases := map[string]string{
		`\frac{a}{b}`:                          " a\n───\n b",
		`\sum_{i=1}^n i`:                       " n\n ∑  i\ni=1",
		`\sqrt{x}`:                             " _\n√x",
		`\begin{pmatrix}a&b\\c&d\end{pmatrix}`: "⎛a  b⎞\n⎝c  d⎠",
		`\begin{aligned} a &= b \\ &= c \end{aligned}`: "a = b\n  = c",
		`\left(\frac{1}{2}\right)`:                     "⎛ 1 ⎞\n⎜───⎟\n⎝ 2 ⎠",
		`(\frac{1}{2})`:                                "⎛ 1 ⎞\n⎜───⎟\n⎝ 2 ⎠",
	}
	for src, want := range cases {
		if got := renderMath(src, true).String(); got != want {
			t.Errorf("%s:\n%s\nquería:\n%s", src, got, want)
		}
	}
	// nada de lo que se dibuja debe ocupar dos columnas: las cajas suponen una por celda
	for _, s := range mathSyms {
		for _, r := range s.s {
			if runeW(r) != 1 {
				t.Errorf("%q ocupa %d columnas", r, runeW(r))
			}
		}
	}
	for _, m := range []map[string]string{bigOps, integrals} {
		for _, s := range m {
			if runeW([]rune(s)[0]) != 1 {
				t.Errorf("%q es ancho", s)
			}
		}
	}
	for _, src := range []string{`}`, `{`, `\frac`, `x^`, `\left(`, `\begin{matrix}`, `&&\\\\`, `\sqrt[`, `_`, `\\`, `\not`} {
		renderMath(src, true) // no debe colgarse ni entrar en pánico
		renderMath(src, false)
	}
}

func mdText(lines []mdLine) string {
	var sb strings.Builder
	for _, l := range lines {
		for _, c := range l.cells {
			sb.WriteString(c.s)
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func TestRenderMarkdown(t *testing.T) {
	src := strings.Split(`# Título

Texto con **negrita**, $x^2$ y precios de $5 y $10.
Sigue el *mismo* párrafo.

$$
\frac{a}{b}
$$

- uno
- [x] hecho
  - adentro

> cita $\pi$

| a | b |
|---|--:|
| 1 | 2 |

`+"```"+`
código *sin* formato
`+"```", "\n")
	out := renderMarkdown(src, 80)
	got := mdText(out)
	for _, want := range []string{"Título\n══════", "negrita, x² y precios de $5 y $10. Sigue el mismo párrafo.",
		" a \n", "───", "• uno", "☒ hecho", "  ◦ adentro", "│ cita π", "│ 1 │ 2 │", "código *sin* formato"} {
		if !strings.Contains(got, want) {
			t.Errorf("falta %q en:\n%s", want, got)
		}
	}
	for _, l := range out {
		for _, c := range l.cells {
			if c.s == "x²" || c.s == "²" {
				if c.k != kMath {
					t.Errorf("la fórmula no tiene estilo de fórmula")
				}
			}
		}
	}
	// cortar renglones: nada más ancho que el ancho pedido (salvo fórmulas en display)
	long := []string{strings.Repeat("palabra ", 40) + "$a+b+c+d$ " + strings.Repeat("x", 100)}
	for _, l := range renderMarkdown(long, 30) {
		if len(l.cells) > 30 {
			t.Fatalf("renglón de %d columnas: %q", len(l.cells), mdText([]mdLine{l}))
		}
	}
	// la fórmula en línea no se corta por la mitad
	if got := mdText(renderMarkdown([]string{"aaaa $a + b$"}, 8)); !strings.Contains(got, "a + b") {
		t.Fatalf("cortó la fórmula:\n%s", got)
	}
}

func TestPreviewInEveryThemeAndSize(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "notas.md")
	var sb strings.Builder
	sb.WriteString("# Notas\n\n$$x = \\frac{-b \\pm \\sqrt{b^2-4ac}}{2a}$$\n\n")
	for i := 0; i < 80; i++ {
		sb.WriteString("Línea con $\\alpha_" + string(rune('0'+i%10)) + "$\n\n")
	}
	if err := os.WriteFile(p, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	a, s := newTestApp(t, p)
	a.draw()
	key(a, tcell.KeyCtrlP, 0, tcell.ModCtrl)
	v, ok := a.dlg.(*MarkdownView)
	if !ok {
		t.Fatalf("Ctrl+P no abrió la vista previa: %T", a.dlg)
	}
	out := screenText(s)
	for _, want := range []string{"VISTA PREVIA · NOTAS.MD", "√b² − 4ac", "Línea con α₀", "Ctrl+P o Esc"} {
		if !strings.Contains(out, want) {
			t.Fatalf("falta %q en:\n%s", want, out)
		}
	}
	key(a, tcell.KeyPgDn, 0, 0)
	if v.off == 0 {
		t.Fatal("PgDn no desplazó")
	}
	for i := range a.themes {
		a.ti = i
		for _, sz := range [][2]int{{160, 40}, {79, 24}, {44, 12}} {
			s.SetSize(sz[0], sz[1])
			a.draw()
			key(a, tcell.KeyEnd, 0, 0)
			key(a, tcell.KeyRight, 0, 0)
		}
	}
	s.SetSize(160, 40)
	key(a, tcell.KeyCtrlP, 0, tcell.ModCtrl)
	if a.dlg != nil {
		t.Fatal("Ctrl+P no cerró")
	}
	if a.buf.Cur.Y == 0 {
		t.Fatal("al volver, el cursor debía quedar donde se estaba leyendo")
	}
	// en un archivo que no es Markdown no se abre
	a.buf.Path = filepath.Join(dir, "x.go")
	key(a, tcell.KeyCtrlP, 0, tcell.ModCtrl)
	if a.dlg != nil {
		t.Fatal("no debía abrir la vista previa de un .go")
	}
}
