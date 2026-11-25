package main

// Fórmulas matemáticas: un subconjunto razonable de LaTeX (lo que se usa en
// Markdown con $…$ y $$…$$) dibujado con caracteres Unicode. En modo
// «display» las fracciones, raíces, límites de sumatorias y matrices se arman
// en dos dimensiones; en línea todo queda en un solo renglón.

import (
	"strings"
	"unicode"
)

// ---------- cajas ----------

// mbox es un rectángulo de celdas. Cada celda es un carácter (con sus marcas
// combinantes); "" es la segunda mitad de un carácter ancho.
type mbox struct {
	c    [][]string
	w    int
	base int // fila de la línea base, para alinear con lo que va al lado
}

func (b mbox) h() int { return len(b.c) }

func blank(w, h int) mbox {
	c := make([][]string, h)
	for i := range c {
		c[i] = make([]string, w)
		for j := range c[i] {
			c[i][j] = " "
		}
	}
	return mbox{c: c, w: w}
}

func textBox(s string) mbox {
	row := []string{}
	for _, r := range s {
		if unicode.Is(unicode.Mn, r) && len(row) > 0 {
			k := len(row) - 1
			for k > 0 && row[k] == "" {
				k--
			}
			row[k] += string(r)
			continue
		}
		row = append(row, string(r))
		if runeW(r) == 2 {
			row = append(row, "")
		}
	}
	return mbox{c: [][]string{row}, w: len(row)}
}

func (b mbox) String() string {
	var sb strings.Builder
	for i, row := range b.c {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(strings.TrimRight(strings.Join(row, ""), " "))
	}
	return sb.String()
}

func blit(dst *mbox, src mbox, x, y int) {
	for j, row := range src.c {
		for i, cell := range row {
			if y+j >= 0 && y+j < dst.h() && x+i >= 0 && x+i < dst.w {
				dst.c[y+j][x+i] = cell
			}
		}
	}
}

// hcat pone cajas una al lado de la otra, alineadas por la línea base.
func hcat(bs ...mbox) mbox {
	above, below, w := 0, 0, 0
	for _, b := range bs {
		above = max(above, b.base)
		below = max(below, b.h()-1-b.base)
		w += b.w
	}
	out := blank(w, above+below+1)
	out.base = above
	x := 0
	for _, b := range bs {
		blit(&out, b, x, above-b.base)
		x += b.w
	}
	return out
}

// vstack apila cajas centradas; la línea base es la de bs[baseOf].
func vstack(bs []mbox, baseOf int) mbox {
	w, h := 0, 0
	for _, b := range bs {
		w = max(w, b.w)
		h += b.h()
	}
	out := blank(w, h)
	y := 0
	for i, b := range bs {
		blit(&out, b, (w-b.w)/2, y)
		if i == baseOf {
			out.base = y + b.base
		}
		y += b.h()
	}
	return out
}

// ---------- árbol ----------

const (
	mAtom = iota
	mRow
	mFrac
	mBinom
	mSqrt
	mScripts
	mDelim
	mEnv
	mAccent
	mOver
	mUnder
	mBoxed
	mSpace
)

// clases para el espaciado (como en TeX, simplificado)
const (
	cOrd = iota
	cOp
	cBin
	cRel
	cOpen
	cClose
	cPunct
	cSpace
)

type mnode struct {
	kind     int
	cls      int
	s        string // texto del átomo, nombre del entorno o del acento
	big      bool   // operador grande (∑, ∫…)
	limits   bool   // en display, los índices van arriba y abajo
	kids     []*mnode
	a, b     *mnode
	sup, sub *mnode
	rows     [][]*mnode
	l, r     string
}

// ---------- tablas de símbolos ----------

var mathSyms = map[string]struct {
	s   string
	cls int
}{}

func init() {
	add := func(cls int, pairs ...string) {
		for i := 0; i+1 < len(pairs); i += 2 {
			mathSyms[pairs[i]] = struct {
				s   string
				cls int
			}{pairs[i+1], cls}
		}
	}
	add(cOrd,
		"alpha", "α", "beta", "β", "gamma", "γ", "delta", "δ", "epsilon", "ϵ", "varepsilon", "ε", "zeta", "ζ",
		"eta", "η", "theta", "θ", "vartheta", "ϑ", "iota", "ι", "kappa", "κ", "lambda", "λ", "mu", "μ", "nu", "ν",
		"xi", "ξ", "pi", "π", "varpi", "ϖ", "rho", "ρ", "varrho", "ϱ", "sigma", "σ", "varsigma", "ς", "tau", "τ",
		"upsilon", "υ", "phi", "ϕ", "varphi", "φ", "chi", "χ", "psi", "ψ", "omega", "ω",
		"Gamma", "Γ", "Delta", "Δ", "Theta", "Θ", "Lambda", "Λ", "Xi", "Ξ", "Pi", "Π", "Sigma", "Σ",
		"Upsilon", "Υ", "Phi", "Φ", "Psi", "Ψ", "Omega", "Ω",
		"infty", "∞", "partial", "∂", "nabla", "∇", "forall", "∀", "exists", "∃", "nexists", "∄",
		"emptyset", "∅", "varnothing", "∅", "neg", "¬", "lnot", "¬", "hbar", "ℏ", "ell", "ℓ", "Re", "ℜ",
		"Im", "ℑ", "aleph", "ℵ", "angle", "∠", "triangle", "△", "prime", "′", "dots", "…", "ldots", "…",
		"cdots", "⋯", "vdots", "⋮", "ddots", "⋱", "top", "⊤", "bot", "⊥", "degree", "°", "checkmark", "✓",
		"%", "%", "$", "$", "#", "#", "&", "&", "_", "_", "|", "‖", "Vert", "‖", "vert", "|", "backslash", "\\",
		"square", "□", "Box", "□", "blacksquare", "■", "diamond", "◇", "clubsuit", "♣", "heartsuit", "♡",
		"wp", "℘", "imath", "ı", "jmath", "ȷ", "surd", "√", "flat", "♭", "sharp", "♯", "natural", "♮",
	)
	add(cBin,
		"pm", "±", "mp", "∓", "times", "×", "cdot", "·", "div", "÷", "ast", "∗", "star", "⋆", "circ", "∘",
		"bullet", "•", "cup", "∪", "cap", "∩", "setminus", "∖", "wedge", "∧", "land", "∧", "vee", "∨",
		"lor", "∨", "oplus", "⊕", "ominus", "⊖", "otimes", "⊗", "odot", "⊙", "oslash", "⊘", "sqcup", "⊔",
		"sqcap", "⊓", "uplus", "⊎", "amalg", "⨿", "dagger", "†", "ddagger", "‡", "wr", "≀",
	)
	add(cRel,
		"leq", "≤", "le", "≤", "geq", "≥", "ge", "≥", "neq", "≠", "ne", "≠", "approx", "≈", "equiv", "≡",
		"sim", "∼", "simeq", "≃", "cong", "≅", "propto", "∝", "in", "∈", "notin", "∉", "ni", "∋",
		"subset", "⊂", "subseteq", "⊆", "supset", "⊃", "supseteq", "⊇", "subsetneq", "⊊", "supsetneq", "⊋",
		"to", "→", "rightarrow", "→", "leftarrow", "←", "gets", "←", "Rightarrow", "⇒", "Leftarrow", "⇐",
		"implies", "⇒", "impliedby", "⇐", "iff", "⇔", "Leftrightarrow", "⇔", "leftrightarrow", "↔",
		"mapsto", "↦", "longrightarrow", "⟶", "longleftarrow", "⟵", "Longrightarrow", "⟹", "Longleftarrow", "⟸",
		"longleftrightarrow", "⟷", "Longleftrightarrow", "⟺", "longmapsto", "⟼", "uparrow", "↑",
		"downarrow", "↓", "updownarrow", "↕", "Uparrow", "⇑", "Downarrow", "⇓", "nearrow", "↗", "searrow", "↘",
		"hookrightarrow", "↪", "hookleftarrow", "↩", "rightleftharpoons", "⇌", "leadsto", "⇝",
		"ll", "≪", "gg", "≫", "perp", "⊥", "parallel", "∥", "mid", "∣", "nmid", "∤", "models", "⊨",
		"vdash", "⊢", "dashv", "⊣", "prec", "≺", "succ", "≻", "preceq", "⪯", "succeq", "⪰", "asymp", "≍",
		"doteq", "≐", "triangleq", "≜", "coloneqq", "≔", "lesssim", "≲", "gtrsim", "≳", "leqslant", "⩽",
		"geqslant", "⩾", "sqsubseteq", "⊑", "sqsupseteq", "⊒", "bowtie", "⋈", "smile", "⌣", "frown", "⌢",
	)
	add(cOpen, "{", "{", "lbrace", "{", "langle", "⟨", "lfloor", "⌊", "lceil", "⌈", "lvert", "|", "lVert", "‖",
		"lbrack", "[", "lparen", "(")
	add(cClose, "}", "}", "rbrace", "}", "rangle", "⟩", "rfloor", "⌋", "rceil", "⌉", "rvert", "|", "rVert", "‖",
		"rbrack", "]", "rparen", ")")
	add(cPunct, "colon", ":", "ldotp", ".", "cdotp", "·")
	add(cSpace, ",", " ", ":", " ", ">", " ", ";", " ", " ", " ", "quad", "  ", "qquad", "    ",
		"enspace", " ", "thinspace", " ", "medspace", " ", "thickspace", " ", "space", " ", "!", "",
		"negthinspace", "", "hfill", " ", "hspace", "")
}

// operadores grandes: los que llevan límites arriba y abajo en display
var bigOps = map[string]string{
	"sum": "∑", "prod": "∏", "coprod": "∐", "bigcup": "⋃", "bigcap": "⋂", "bigvee": "⋁", "bigwedge": "⋀",
	"bigoplus": "⊕", "bigotimes": "⊗", "bigodot": "⊙", "bigsqcup": "⊔", "biguplus": "⊎",
}

var integrals = map[string]string{"int": "∫", "iint": "∬", "iiint": "∭", "oint": "∮", "oiint": "∯", "intop": "∫"}

var funcNames = map[string]bool{}
var limFuncs = map[string]bool{"lim": true, "max": true, "min": true, "sup": true, "inf": true, "det": true,
	"Pr": true, "gcd": true, "liminf": true, "limsup": true, "argmax": true, "argmin": true}

func init() {
	for _, f := range strings.Fields(`sin cos tan cot sec csc arcsin arccos arctan sinh cosh tanh coth log ln lg exp
		lim max min sup inf det arg deg dim gcd hom ker Pr liminf limsup argmax argmin mod bmod sgn tr rank diag`) {
		funcNames[f] = true
	}
}

var blackboard = map[rune]string{'C': "ℂ", 'H': "ℍ", 'N': "ℕ", 'P': "ℙ", 'Q': "ℚ", 'R': "ℝ", 'Z': "ℤ", '1': "𝟙",
	'E': "𝔼", 'A': "𝔸", 'F': "𝔽", 'K': "𝕂"}
var calligraphic = map[rune]string{'B': "ℬ", 'E': "ℰ", 'F': "ℱ", 'H': "ℋ", 'I': "ℐ", 'L': "ℒ", 'M': "ℳ",
	'R': "ℛ", 'e': "ℯ", 'g': "ℊ", 'o': "ℴ"}
var fraktur = map[rune]string{'C': "ℭ", 'H': "ℌ", 'I': "ℑ", 'R': "ℜ", 'Z': "ℨ"}

var supMap = map[rune]rune{}
var subMap = map[rune]rune{}

func init() {
	pairs := func(m map[rune]rune, from, to string) {
		f, t := []rune(from), []rune(to)
		for i := range f {
			m[f[i]] = t[i]
		}
	}
	pairs(supMap, "0123456789+-−=()niabcdefghjklmoprstuvwxyzABDEGHIJKLMNOPRTUVWαβγδεθφχ′·",
		"⁰¹²³⁴⁵⁶⁷⁸⁹⁺⁻⁻⁼⁽⁾ⁿⁱᵃᵇᶜᵈᵉᶠᵍʰʲᵏˡᵐᵒᵖʳˢᵗᵘᵛʷˣʸᶻᴬᴮᴰᴱᴳᴴᴵᴶᴷᴸᴹᴺᴼᴾᴿᵀᵁⱽᵂᵅᵝᵞᵟᵋᶿᵠᵡ′˙")
	pairs(subMap, "0123456789+-−=()aehijklmnoprstuvxβγρφχ",
		"₀₁₂₃₄₅₆₇₈₉₊₋₋₌₍₎ₐₑₕᵢⱼₖₗₘₙₒₚᵣₛₜᵤᵥₓᵦᵧᵨᵩᵪ")
}

var vulgar = map[string]string{"1/2": "½", "1/3": "⅓", "2/3": "⅔", "1/4": "¼", "3/4": "¾", "1/5": "⅕",
	"2/5": "⅖", "3/5": "⅗", "4/5": "⅘", "1/6": "⅙", "5/6": "⅚", "1/8": "⅛", "3/8": "⅜", "5/8": "⅝", "7/8": "⅞"}

// ---------- lectura del LaTeX ----------

type mparser struct {
	src []rune
	i   int
}

func isASCIILetter(r rune) bool { return r < 128 && unicode.IsLetter(r) }

func (p *mparser) skipSpace() {
	for p.i < len(p.src) {
		switch r := p.src[p.i]; {
		case unicode.IsSpace(r):
			p.i++
		case r == '%':
			for p.i < len(p.src) && p.src[p.i] != '\n' {
				p.i++
			}
		default:
			return
		}
	}
}

func (p *mparser) next() string {
	p.skipSpace()
	if p.i >= len(p.src) {
		return ""
	}
	r := p.src[p.i]
	p.i++
	if r != '\\' || p.i >= len(p.src) {
		return string(r)
	}
	j := p.i
	if isASCIILetter(p.src[j]) {
		for j < len(p.src) && isASCIILetter(p.src[j]) {
			j++
		}
	} else {
		j++
	}
	t := "\\" + string(p.src[p.i:j])
	p.i = j
	return t
}

func (p *mparser) peek() string {
	save := p.i
	t := p.next()
	p.i = save
	return t
}

// raw lee un argumento entre llaves tal cual (para \text, \begin…).
func (p *mparser) raw() string {
	p.skipSpace()
	if p.i >= len(p.src) {
		return ""
	}
	if p.src[p.i] != '{' {
		t := p.next()
		return t
	}
	depth, start := 0, p.i+1
	for j := p.i; j < len(p.src); j++ {
		switch p.src[j] {
		case '\\':
			j++
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				p.i = j + 1
				return string(p.src[start:j])
			}
		}
	}
	p.i = len(p.src)
	return string(p.src[start:])
}

func stopAt(ts ...string) func(string) bool {
	return func(t string) bool {
		for _, s := range ts {
			if t == s {
				return true
			}
		}
		return false
	}
}

func atom(s string, cls int) *mnode { return &mnode{kind: mAtom, s: s, cls: cls} }

func (p *mparser) row(stop func(string) bool) *mnode {
	n := &mnode{kind: mRow}
	for {
		t := p.peek()
		if t == "" || stop(t) {
			return n
		}
		switch t {
		case "^", "_":
			p.next()
			arg := p.arg()
			k := len(n.kids)
			var last *mnode
			if k > 0 && n.kids[k-1].kind == mScripts && (t == "^" && n.kids[k-1].sup == nil || t == "_" && n.kids[k-1].sub == nil) {
				last = n.kids[k-1]
			} else {
				base := atom("", cOrd)
				if k > 0 {
					base = n.kids[k-1]
					n.kids = n.kids[:k-1]
				}
				last = &mnode{kind: mScripts, a: base}
				n.kids = append(n.kids, last)
			}
			if t == "^" {
				last.sup = arg
			} else {
				last.sub = arg
			}
		case "'":
			c := 0
			for p.peek() == "'" {
				p.next()
				c++
			}
			n.kids = append(n.kids, atom([]string{"′", "″", "‴", "⁗"}[min(c, 4)-1], cOrd))
		default:
			if nd := p.atom(); nd != nil {
				n.kids = append(n.kids, nd)
			}
		}
	}
}

func (p *mparser) arg() *mnode {
	switch p.peek() {
	case "{":
		p.next()
		r := p.row(stopAt("}"))
		p.next()
		return r
	case "", "}", "&", "\\\\":
		return &mnode{kind: mRow}
	}
	if n := p.atom(); n != nil {
		return n
	}
	return &mnode{kind: mRow}
}

func (p *mparser) delimTok() string {
	t := p.next()
	switch t {
	case "<", "\\langle":
		return "⟨"
	case ">", "\\rangle":
		return "⟩"
	case "\\{", "\\lbrace":
		return "{"
	case "\\}", "\\rbrace":
		return "}"
	case "\\|", "\\Vert", "\\lVert", "\\rVert":
		return "‖"
	case "\\vert", "\\lvert", "\\rvert":
		return "|"
	case "\\lfloor":
		return "⌊"
	case "\\rfloor":
		return "⌋"
	case "\\lceil":
		return "⌈"
	case "\\rceil":
		return "⌉"
	case "\\backslash":
		return "\\"
	}
	return t
}

func (p *mparser) atom() *mnode {
	t := p.next()
	if t == "" {
		return nil
	}
	if t == "{" {
		r := p.row(stopAt("}"))
		p.next()
		return r
	}
	if len(t) > 1 && t[0] == '\\' {
		return p.command(t[1:])
	}
	r := []rune(t)[0]
	switch r {
	case '+':
		return atom("+", cBin)
	case '-':
		return atom("−", cBin)
	case '*':
		return atom("∗", cBin)
	case '=', '<', '>', ':':
		return atom(t, cRel)
	case ',', ';':
		return atom(t, cPunct)
	case '(', '[':
		return atom(t, cOpen)
	case ')', ']':
		return atom(t, cClose)
	case '~':
		return atom(" ", cSpace)
	case '}', '&', '#':
		return nil
	}
	return atom(t, cOrd)
}

func (p *mparser) env(name string) *mnode {
	if name == "array" || name == "alignat" || name == "alignat*" || name == "tabular" {
		p.raw() // especificación de columnas
	}
	n := &mnode{kind: mEnv, s: strings.TrimSuffix(name, "*")}
	cur := []*mnode{}
	for {
		cell := p.row(stopAt("&", "\\\\", "\\cr", "\\end"))
		cur = append(cur, cell)
		switch p.next() {
		case "&":
			continue
		case "\\\\", "\\cr":
			if p.peek() == "[" { // \\[2pt]
				for t := p.next(); t != "]" && t != ""; t = p.next() {
				}
			}
			n.rows = append(n.rows, cur)
			cur = []*mnode{}
			continue
		case "\\end":
			p.raw()
		}
		break
	}
	if len(cur) > 1 || len(cur[0].kids) > 0 {
		n.rows = append(n.rows, cur)
	}
	return n
}

func mapLetters(s string, m map[rune]string) string {
	var sb strings.Builder
	for _, r := range s {
		if v, ok := m[r]; ok {
			sb.WriteString(v)
		} else if !unicode.IsSpace(r) {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

var negated = map[string]string{"=": "≠", "\\in": "∉", "<": "≮", ">": "≯", "\\equiv": "≢", "\\subset": "⊄",
	"\\supset": "⊅", "\\subseteq": "⊈", "\\supseteq": "⊉", "\\leq": "≰", "\\geq": "≱", "\\sim": "≁",
	"\\approx": "≉", "\\exists": "∄", "\\mid": "∤", "\\parallel": "∦", "\\cong": "≇"}

func (p *mparser) command(name string) *mnode {
	if s, ok := mathSyms[name]; ok {
		if s.cls == cSpace {
			if name == "hspace" {
				p.raw()
				return atom(" ", cSpace)
			}
			if s.s == "" {
				return nil
			}
		}
		return atom(s.s, s.cls)
	}
	if s, ok := bigOps[name]; ok {
		return &mnode{kind: mAtom, s: s, cls: cOp, big: true, limits: true}
	}
	if s, ok := integrals[name]; ok {
		return &mnode{kind: mAtom, s: s, cls: cOp, big: true}
	}
	if funcNames[name] {
		s := name
		switch name {
		case "liminf":
			s = "lim inf"
		case "limsup":
			s = "lim sup"
		case "argmax":
			s = "arg max"
		case "argmin":
			s = "arg min"
		case "bmod":
			s = "mod"
		}
		return &mnode{kind: mAtom, s: s, cls: cOp, limits: limFuncs[name]}
	}
	switch name {
	case "frac", "dfrac", "tfrac", "cfrac":
		return &mnode{kind: mFrac, a: p.arg(), b: p.arg()}
	case "binom", "dbinom", "tbinom":
		return &mnode{kind: mBinom, a: p.arg(), b: p.arg()}
	case "pmod":
		return &mnode{kind: mRow, kids: []*mnode{atom(" (mod ", cOrd), p.arg(), atom(")", cClose)}}
	case "sqrt":
		n := &mnode{kind: mSqrt}
		if p.peek() == "[" {
			p.next()
			n.b = p.row(stopAt("]"))
			p.next()
		}
		n.a = p.arg()
		return n
	case "left":
		n := &mnode{kind: mDelim, l: p.delimTok()}
		n.a = p.row(stopAt("\\right"))
		if p.next() == "\\right" {
			n.r = p.delimTok()
		}
		return n
	case "right", "middle":
		p.delimTok()
		return nil
	case "big", "Big", "bigg", "Bigg", "bigl", "bigr", "Bigl", "Bigr", "biggl", "biggr", "Biggl", "Biggr",
		"bigm", "Bigm", "displaystyle", "textstyle", "scriptstyle", "scriptscriptstyle", "limits", "nolimits",
		"nonumber", "notag", "nonscript", "centering", "rm", "bf", "it", "cal", "mathstrut", "strut", "allowbreak":
		return nil
	case "begin":
		return p.env(p.raw())
	case "end":
		p.raw()
		return nil
	case "text", "textrm", "textnormal", "mbox", "hbox", "textit", "textbf", "texttt", "textsf", "textup", "emph", "fbox":
		s := p.raw()
		if name == "fbox" {
			return &mnode{kind: mBoxed, a: atom(s, cOrd)}
		}
		return atom(s, cOrd)
	case "operatorname", "operatorname*":
		lim := p.peek() == "*"
		if lim {
			p.next()
		}
		return &mnode{kind: mAtom, s: p.raw(), cls: cOp, limits: lim || name == "operatorname*"}
	case "mathrm", "mathit", "mathbf", "boldsymbol", "bm", "mathsf", "mathtt", "mathnormal", "pmb", "mathord", "hphantom", "ensuremath":
		return p.arg()
	case "mathbin":
		n := p.arg()
		return &mnode{kind: mRow, cls: cBin, kids: []*mnode{n}}
	case "mathrel":
		n := p.arg()
		return &mnode{kind: mRow, cls: cRel, kids: []*mnode{n}}
	case "mathop":
		return &mnode{kind: mAtom, s: p.raw(), cls: cOp}
	case "mathbb", "Bbb", "mathds", "symbb":
		return atom(mapLetters(p.raw(), blackboard), cOrd)
	case "mathcal", "mathscr":
		return atom(mapLetters(p.raw(), calligraphic), cOrd)
	case "mathfrak":
		return atom(mapLetters(p.raw(), fraktur), cOrd)
	case "hat", "widehat", "bar", "overline", "vec", "overrightarrow", "overleftarrow", "dot", "ddot",
		"tilde", "widetilde", "check", "breve", "acute", "grave", "underline", "mathring":
		return &mnode{kind: mAccent, s: name, a: p.arg()}
	case "overset", "stackrel":
		top := p.arg()
		return &mnode{kind: mOver, a: top, b: p.arg()}
	case "underset":
		bot := p.arg()
		return &mnode{kind: mUnder, a: bot, b: p.arg()}
	case "overbrace", "underbrace":
		return p.arg()
	case "boxed":
		return &mnode{kind: mBoxed, a: p.arg()}
	case "color":
		p.raw()
		return nil
	case "textcolor", "colorbox":
		p.raw()
		return p.arg()
	case "label", "vphantom", "phantom", "ref", "eqref":
		s := p.raw()
		if name == "ref" || name == "eqref" {
			return atom("("+s+")", cOrd)
		}
		return nil
	case "tag", "tag*":
		return atom("    ("+p.raw()+")", cOrd)
	case "not":
		t := p.peek()
		if s, ok := negated[t]; ok {
			p.next()
			return atom(s, cRel)
		}
		if n := p.atom(); n != nil {
			if n.kind == mAtom {
				n.s += "̸"
			}
			return n
		}
		return nil
	case "\\":
		return nil // salto de línea fuera de un entorno: se ignora
	}
	return atom("\\"+name, cOrd) // desconocido: se muestra tal cual
}

// parseMath lee una fórmula completa. Si tiene & o \\ sueltos (pasa mucho en
// textos escritos a mano o por modelos) se trata como un entorno aligned.
func parseMath(src string) *mnode {
	p := &mparser{src: []rune(src)}
	env := p.env("")
	if len(env.rows) == 1 && len(env.rows[0]) == 1 {
		return env.rows[0][0]
	}
	if len(env.rows) == 0 {
		return &mnode{kind: mRow}
	}
	env.s = "gather"
	for _, r := range env.rows {
		if len(r) > 1 {
			env.s = "aligned"
		}
	}
	return env
}

// ---------- dibujo ----------

type mctx struct{ display, tight bool }

// renderMath dibuja una fórmula. En línea siempre da un solo renglón.
func renderMath(src string, display bool) mbox {
	b := mctx{display: display}.render(parseMath(src))
	if !display && b.h() != 1 {
		return textBox(strings.Join(strings.Fields(src), " "))
	}
	return b
}

func boxText(b mbox) string {
	if b.h() == 0 {
		return ""
	}
	return strings.Join(b.c[b.base], "")
}

func (c mctx) script() mctx { return mctx{display: c.display, tight: true} }

func (c mctx) inline() mctx { return mctx{tight: c.tight} }

func clsOf(n *mnode) int {
	switch n.kind {
	case mAtom:
		return n.cls
	case mScripts:
		if n.a.kind == mAtom && n.a.cls == cOp {
			return cOp
		}
		if n.a.kind == mAtom && n.a.cls == cRel {
			return cRel
		}
		return cOrd
	case mRow:
		if n.cls == cBin || n.cls == cRel {
			return n.cls
		}
	case mOver, mUnder:
		return clsOf(n.b)
	}
	return cOrd
}

func (c mctx) render(n *mnode) mbox {
	if n == nil {
		return textBox("")
	}
	switch n.kind {
	case mAtom:
		return textBox(n.s)
	case mRow:
		return c.renderRow(n.kids)
	case mFrac:
		return c.frac(n)
	case mBinom:
		top, bot := c.script().render(n.a), c.script().render(n.b)
		if !c.display {
			return textBox("C(" + boxText(top) + ", " + boxText(bot) + ")")
		}
		inner := vstack([]mbox{top, bot}, 0)
		inner.base = top.h() - 1
		if inner.h() == 2 {
			inner.base = 1
		}
		return hcat(delimBox("(", inner), inner, delimBox(")", inner))
	case mSqrt:
		return c.sqrt(n)
	case mScripts:
		return c.scripts(n)
	case mDelim:
		inner := c.render(n.a)
		if !c.display {
			return hcat(textBox(strings.TrimPrefix(n.l, ".")), inner, textBox(strings.TrimPrefix(n.r, ".")))
		}
		return hcat(delimBox(n.l, inner), inner, delimBox(n.r, inner))
	case mEnv:
		return c.env(n)
	case mAccent:
		return c.accent(n)
	case mOver, mUnder:
		main := c.render(n.b)
		ann := c.script().render(n.a)
		if !c.display {
			s := &mnode{kind: mScripts, a: n.b}
			if n.kind == mOver {
				s.sup = n.a
			} else {
				s.sub = n.a
			}
			return c.render(s)
		}
		if n.kind == mOver {
			return vstack([]mbox{ann, main}, 1)
		}
		return vstack([]mbox{main, ann}, 0)
	case mBoxed:
		in := c.render(n.a)
		if !c.display {
			return hcat(textBox("["), in, textBox("]"))
		}
		out := blank(in.w+4, in.h()+2)
		out.base = in.base + 1
		blit(&out, in, 2, 1)
		for x := 1; x < out.w-1; x++ {
			out.c[0][x], out.c[out.h()-1][x] = "─", "─"
		}
		for y := 1; y < out.h()-1; y++ {
			out.c[y][0], out.c[y][out.w-1] = "│", "│"
		}
		out.c[0][0], out.c[0][out.w-1], out.c[out.h()-1][0], out.c[out.h()-1][out.w-1] = "┌", "┐", "└", "┘"
		return out
	}
	return textBox("")
}

// simple: no hace falta paréntesis al escribirlo en línea (a/b, x^n…)
func simple(s string) bool {
	rs := []rune(s)
	if len(rs) <= 1 {
		return true
	}
	if rs[0] == '(' && rs[len(rs)-1] == ')' && strings.Count(s, "(") == 1 {
		return true
	}
	for _, r := range rs {
		if !(unicode.IsLetter(r) || unicode.IsNumber(r) || r == '.' || r == '!' || r == '√' || r == '′' || unicode.Is(unicode.Mn, r)) {
			return false
		}
	}
	return true
}

func paren(s string) string {
	if simple(s) {
		return s
	}
	return "(" + s + ")"
}

// sparen: un índice de más de un carácter va entre paréntesis (e^(iπ)).
func sparen(s string) string {
	if len([]rune(s)) <= 1 || strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") && strings.Count(s, "(") == 1 {
		return s
	}
	return "(" + s + ")"
}

func (c mctx) frac(n *mnode) mbox {
	if !c.display {
		num, den := boxText(c.render(n.a)), boxText(c.render(n.b))
		if v, ok := vulgar[num+"/"+den]; ok {
			return textBox(v)
		}
		return textBox(paren(num) + "/" + paren(den))
	}
	num, den := mctx{display: true}.render(n.a), mctx{display: true}.render(n.b)
	w := max(num.w, den.w) + 2
	return vstack([]mbox{num, textBox(strings.Repeat("─", w)), den}, 1)
}

func (c mctx) sqrt(n *mnode) mbox {
	body := c.render(n.a)
	sym := "√"
	if n.b != nil {
		idx := boxText(c.script().render(n.b))
		switch idx {
		case "3":
			sym = "∛"
		case "4":
			sym = "∜"
		default:
			if s, ok := toScript(textBox(idx), supMap); ok {
				sym = s + "√"
			} else {
				sym = idx + "√"
			}
		}
	}
	if !c.display {
		return textBox(sym + paren(boxText(body)))
	}
	sw := strW(sym)
	out := blank(sw+body.w, body.h()+1)
	out.base = body.base + 1
	blit(&out, body, sw, 1)
	for x := sw; x < out.w; x++ {
		out.c[0][x] = "_"
	}
	last := out.h() - 1
	for y := 1; y < last; y++ {
		out.c[y][sw-1] = "│"
	}
	blit(&out, textBox(sym), 0, last)
	return out
}

func toScript(b mbox, m map[rune]rune) (string, bool) {
	if b.h() != 1 || b.w == 0 {
		return "", false
	}
	var sb strings.Builder
	for _, cell := range b.c[0] {
		rs := []rune(cell)
		if len(rs) != 1 {
			return "", false
		}
		v, ok := m[rs[0]]
		if !ok {
			return "", false
		}
		sb.WriteRune(v)
	}
	return sb.String(), true
}

func (c mctx) scripts(n *mnode) mbox {
	base := c.render(n.a)
	var sup, sub *mbox
	sc := c.script()
	if n.sup != nil {
		b := sc.render(n.sup)
		sup = &b
	}
	if n.sub != nil {
		b := sc.render(n.sub)
		sub = &b
	}
	if c.display && n.a.kind == mAtom && n.a.limits {
		parts := []mbox{}
		baseOf := 0
		if sup != nil {
			parts = append(parts, *sup)
			baseOf = 1
		}
		parts = append(parts, base)
		if sub != nil {
			parts = append(parts, *sub)
		}
		return vstack(parts, baseOf)
	}
	// índices con caracteres Unicode (x², aᵢ) cuando se puede
	if base.h() == 1 {
		us, ok1 := "", true
		ub, ok2 := "", true
		if sup != nil {
			us, ok1 = toScript(*sup, supMap)
		}
		if sub != nil {
			ub, ok2 = toScript(*sub, subMap)
		}
		if ok1 && ok2 {
			return hcat(base, textBox(ub+us))
		}
	}
	if !c.display {
		s := boxText(base)
		if sub != nil {
			if u, ok := toScript(*sub, subMap); ok {
				s += u
			} else {
				s += "_" + sparen(boxText(*sub))
			}
		}
		if sup != nil {
			if u, ok := toScript(*sup, supMap); ok {
				s += u
			} else {
				s += "^" + sparen(boxText(*sup))
			}
		}
		return textBox(s)
	}
	ov := 0
	if base.h() > 1 {
		ov = 1
	}
	top, sw := 0, 0
	if sup != nil {
		top = max(0, sup.h()-ov)
		sw = sup.w
	}
	H := top + base.h()
	subY := top + base.h() - ov
	if sub != nil {
		H = max(H, subY+sub.h())
		sw = max(sw, sub.w)
	}
	out := blank(base.w+sw, H)
	blit(&out, base, 0, top)
	if sup != nil {
		blit(&out, *sup, base.w, top+ov-sup.h())
	}
	if sub != nil {
		blit(&out, *sub, base.w, subY)
	}
	out.base = top + base.base
	return out
}

// delimBox arma un paréntesis (o corchete, llave…) de la altura de ref.
func delimBox(d string, ref mbox) mbox {
	h := ref.h()
	if d == "." || d == "" {
		b := blank(0, h)
		b.base = ref.base
		return b
	}
	b := blank(strW(d), h)
	b.base = ref.base
	if h == 1 {
		return hcat(textBox(d))
	}
	var top, mid, bot, center string
	switch d {
	case "(":
		top, mid, bot = "⎛", "⎜", "⎝"
	case ")":
		top, mid, bot = "⎞", "⎟", "⎠"
	case "[":
		top, mid, bot = "⎡", "⎢", "⎣"
	case "]":
		top, mid, bot = "⎤", "⎥", "⎦"
	case "{":
		top, mid, bot, center = "⎧", "⎪", "⎩", "⎨"
	case "}":
		top, mid, bot, center = "⎫", "⎪", "⎭", "⎬"
	case "|":
		top, mid, bot = "│", "│", "│"
	case "‖":
		top, mid, bot = "‖", "‖", "‖"
	case "⌊":
		top, mid, bot = "│", "│", "⌊"
	case "⌋":
		top, mid, bot = "│", "│", "⌋"
	case "⌈":
		top, mid, bot = "⌈", "│", "│"
	case "⌉":
		top, mid, bot = "⌉", "│", "│"
	case "⟨", "⟩":
		up, down := "╱", "╲"
		if d == "⟩" {
			up, down = down, up
		}
		for y := 0; y < h; y++ {
			switch {
			case h%2 == 1 && y == h/2:
				b.c[y][0] = d
			case y < h/2:
				b.c[y][0] = up
			default:
				b.c[y][0] = down
			}
		}
		return b
	default:
		blit(&b, textBox(d), 0, b.base)
		return b
	}
	for y := 0; y < h; y++ {
		switch {
		case y == 0:
			b.c[y][0] = top
		case y == h-1:
			b.c[y][0] = bot
		case center != "" && y == (h-1)/2:
			b.c[y][0] = center
		default:
			b.c[y][0] = mid
		}
	}
	return b
}

var stretchy = map[string]string{"(": ")", "[": "]", "{": "}", "⟨": "⟩", "⌊": "⌋", "⌈": "⌉"}

func (c mctx) renderRow(kids []*mnode) mbox {
	type item struct {
		b   mbox
		cls int
	}
	items := []item{}
	for i := 0; i < len(kids); i++ {
		k := kids[i]
		// en display, un paréntesis común que encierra algo alto (una
		// fracción) crece como si fuera \left( … \right)
		if c.display && k.kind == mAtom && k.cls == cOpen {
			if closer, ok := stretchy[k.s]; ok {
				depth, j := 0, i+1
				for ; j < len(kids); j++ {
					kj := kids[j]
					if kj.kind != mAtom {
						continue
					}
					if kj.cls == cOpen && kj.s == k.s {
						depth++
					} else if kj.cls == cClose && kj.s == closer {
						if depth == 0 {
							break
						}
						depth--
					}
				}
				if j < len(kids) {
					inner := c.renderRow(kids[i+1 : j])
					b := hcat(delimBox(k.s, inner), inner, delimBox(closer, inner))
					items = append(items, item{b, cOrd})
					i = j
					continue
				}
			}
		}
		items = append(items, item{c.render(k), clsOf(k)})
	}
	// un + o − al principio, o después de otro operador, es unario
	for i := range items {
		if items[i].cls != cBin {
			continue
		}
		prev := -1
		if i > 0 {
			prev = items[i-1].cls
		}
		if prev == -1 || prev == cBin || prev == cRel || prev == cOpen || prev == cPunct || prev == cOp || i == len(items)-1 {
			items[i].cls = cOrd
		}
	}
	out := make([]mbox, 0, 2*len(items))
	for i, it := range items {
		if i > 0 && gap(items[i-1].cls, it.cls, c.tight) {
			out = append(out, textBox(" "))
		}
		out = append(out, it.b)
	}
	if len(out) == 0 {
		return textBox("")
	}
	return hcat(out...)
}

func gap(prev, cur int, tight bool) bool {
	switch {
	case prev == cSpace || cur == cSpace:
		return false
	case prev == cOp:
		return cur == cOrd || cur == cOp
	case tight:
		return false
	case prev == cRel && cur == cRel:
		return false
	case cur == cRel:
		return prev != cOpen
	case prev == cRel:
		return cur != cClose && cur != cPunct
	case cur == cBin || prev == cBin:
		return true
	case prev == cPunct:
		return true
	}
	return false
}

func (c mctx) env(n *mnode) mbox {
	name := n.s
	l, r := "", ""
	switch name {
	case "pmatrix", "smallmatrix":
		l, r = "(", ")"
	case "bmatrix":
		l, r = "[", "]"
	case "Bmatrix":
		l, r = "{", "}"
	case "vmatrix":
		l, r = "|", "|"
	case "Vmatrix":
		l, r = "‖", "‖"
	case "cases", "dcases":
		l = "{"
	case "rcases":
		r = "}"
	}
	align := strings.HasPrefix(name, "align") || name == "split" || name == "eqnarray" || name == "flalign"
	cases := strings.HasSuffix(name, "cases")
	cell := func(i, j int) *mnode {
		if j >= len(n.rows[i]) {
			return nil
		}
		nd := n.rows[i][j]
		if align && j%2 == 1 && len(nd.kids) > 0 {
			// "&=" : el = necesita algo a la izquierda para espaciarse
			nd = &mnode{kind: mRow, kids: append([]*mnode{atom("", cOrd)}, nd.kids...)}
		}
		return nd
	}
	if !c.display {
		rows := []string{}
		for i, row := range n.rows {
			cells := []string{}
			for j := range row {
				cells = append(cells, boxText(c.render(cell(i, j))))
			}
			sep := ", "
			if align || name == "gather" || name == "" {
				sep = ""
			} else if cases {
				sep = " "
			}
			rows = append(rows, strings.Join(cells, sep))
		}
		s := strings.Join(rows, "; ")
		if l != "" || r != "" {
			s = l + s + r
			if cases {
				s = l + " " + strings.Join(rows, "; ")
			}
		}
		return textBox(s)
	}
	cols := 0
	for _, row := range n.rows {
		cols = max(cols, len(row))
	}
	cells := make([][]mbox, len(n.rows))
	colW := make([]int, cols)
	tall := false
	for i := range n.rows {
		cells[i] = make([]mbox, cols)
		for j := 0; j < cols; j++ {
			cells[i][j] = c.render(cell(i, j))
			colW[j] = max(colW[j], cells[i][j].w)
			tall = tall || cells[i][j].h() > 1
		}
	}
	gapAfter := func(j int) int {
		switch {
		case j == cols-1:
			return 0
		case align:
			if j%2 == 0 {
				return 0
			}
			return 3
		}
		return 2
	}
	W := 0
	for j := range colW {
		W += colW[j] + gapAfter(j)
	}
	var rowsB []mbox
	for i := range cells {
		above, below := 0, 0
		for _, b := range cells[i] {
			above = max(above, b.base)
			below = max(below, b.h()-1-b.base)
		}
		rb := blank(W, above+below+1)
		x := 0
		for j, b := range cells[i] {
			dx := (colW[j] - b.w) / 2
			switch {
			case cases || (align && j%2 == 1):
				dx = 0
			case align:
				dx = colW[j] - b.w
			}
			blit(&rb, b, x+dx, above-b.base)
			x += colW[j] + gapAfter(j)
		}
		if tall && i > 0 {
			rowsB = append(rowsB, blank(W, 1))
		}
		rowsB = append(rowsB, rb)
	}
	if len(rowsB) == 0 {
		return textBox(l + r)
	}
	grid := vstack(rowsB, 0)
	grid.base = (grid.h() - 1) / 2
	if l != "" || r != "" {
		parts := []mbox{}
		if l != "" {
			parts = append(parts, delimBox(l, grid))
			if cases {
				parts = append(parts, textBox(" "))
			}
		}
		parts = append(parts, grid)
		if r != "" {
			parts = append(parts, delimBox(r, grid))
		}
		return hcat(parts...)
	}
	return grid
}

var accentMarks = map[string]struct {
	comb  string
	above string
}{
	"hat": {"̂", "^"}, "widehat": {"̂", "^"}, "bar": {"̄", "_"}, "overline": {"̅", "_"},
	"vec": {"⃗", "→"}, "overrightarrow": {"⃗", "→"}, "overleftarrow": {"⃖", "←"},
	"dot": {"̇", "·"}, "ddot": {"̈", "¨"}, "tilde": {"̃", "~"}, "widetilde": {"̃", "~"},
	"check": {"̌", "ˇ"}, "breve": {"̆", "˘"}, "acute": {"́", "´"}, "grave": {"̀", "`"},
	"mathring": {"̊", "°"}, "underline": {"̲", "‾"},
}

func (c mctx) accent(n *mnode) mbox {
	body := c.render(n.a)
	m := accentMarks[n.s]
	if body.h() == 1 && body.w == 1 {
		body.c[0][0] += m.comb
		return body
	}
	if !c.display || body.h() == 1 && (n.s == "bar" || n.s == "overline" || n.s == "underline") {
		// en línea: la marca va sobre cada carácter (rayas) o sobre el último
		out := mbox{c: make([][]string, body.h()), w: body.w, base: body.base}
		for y := range body.c {
			out.c[y] = append([]string(nil), body.c[y]...)
		}
		row := out.c[out.base]
		for x := range row {
			if n.s == "bar" || n.s == "overline" || n.s == "underline" {
				if row[x] != "" && row[x] != " " {
					row[x] += m.comb
				}
			} else if x == len(row)-1 {
				row[x] += m.comb
			}
		}
		return out
	}
	mark := blank(body.w, 1)
	switch m.above {
	case "_", "~", "‾":
		for x := range mark.c[0] {
			mark.c[0][x] = m.above
		}
	case "→", "←":
		for x := range mark.c[0] {
			mark.c[0][x] = "─"
		}
		if m.above == "→" {
			mark.c[0][body.w-1] = "→"
		} else {
			mark.c[0][0] = "←"
		}
	default:
		mark.c[0][body.w/2] = m.above
	}
	if n.s == "underline" {
		return vstack([]mbox{body, mark}, 0)
	}
	return vstack([]mbox{mark, body}, 1)
}
