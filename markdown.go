package main

// Vista previa de Markdown: títulos, párrafos, énfasis, listas, citas,
// tablas, código y fórmulas ($…$, $$…$$, \(…\), \[…\], ```math).

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v2"
)

type mdKind uint8

const (
	kText mdKind = iota
	kH1
	kH2
	kH3
	kCode
	kMath
	kQuote
	kLink
	kDim
	kRule
)

type mdAttr uint8

const (
	aBold mdAttr = 1 << iota
	aItalic
	aStrike
	aUnder
)

// mdCell es una columna de pantalla. s == "" es la mitad derecha de un carácter ancho.
type mdCell struct {
	s  string
	k  mdKind
	at mdAttr
	nb bool // espacio que no corta el renglón (dentro de una fórmula)
}

type mdLine struct {
	cells []mdCell
	src   int // línea del archivo de donde sale
}

func (k mdKind) style(T *Theme, at mdAttr) tcell.Style {
	var s tcell.Style
	switch k {
	case kH1, kH2:
		s = T.Keyword.Bold(true)
	case kH3:
		s = T.Keyword.Bold(true)
	case kCode:
		s = T.String
	case kMath:
		s = T.Number
	case kQuote, kDim:
		s = T.Comment
	case kLink:
		s = T.Normal.Underline(true)
	case kRule:
		s = T.Frame
	default:
		s = T.Normal
	}
	if at&aBold != 0 {
		s = s.Bold(true)
	}
	if at&aItalic != 0 {
		s = s.Italic(true)
	}
	if at&aStrike != 0 {
		s = s.StrikeThrough(true)
	}
	if at&aUnder != 0 {
		s = s.Underline(true)
	}
	return s
}

func cellsOf(s string, k mdKind, at mdAttr, out []mdCell) []mdCell {
	for _, r := range s {
		if r == '\t' {
			r = ' '
		}
		if unicode.Is(unicode.Mn, r) && len(out) > 0 {
			j := len(out) - 1
			for j > 0 && out[j].s == "" {
				j--
			}
			out[j].s += string(r)
			continue
		}
		out = append(out, mdCell{s: string(r), k: k, at: at})
		if runeW(r) == 2 {
			out = append(out, mdCell{k: k, at: at})
		}
	}
	return out
}

func mathCells(b mbox, row int, out []mdCell) []mdCell {
	for _, c := range b.c[row] {
		out = append(out, mdCell{s: c, k: kMath, nb: true})
	}
	return out
}

// ---------- en línea ----------

func isAlnum(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

func findRun(s []rune, from int, run string) int {
	n := len([]rune(run))
	for j := from; j+n <= len(s); j++ {
		if s[j] == '\\' {
			j++
			continue
		}
		if string(s[j:j+n]) == run {
			return j
		}
	}
	return -1
}

func countRun(s []rune, i int) int {
	n := 0
	for i+n < len(s) && s[i+n] == s[i] {
		n++
	}
	return n
}

// inlineMD convierte el texto de un párrafo en celdas con estilo.
func inlineMD(s []rune, k mdKind, at mdAttr, out []mdCell) []mdCell {
	lit := func(r rune) { out = cellsOf(string(r), k, at, out) }
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '\n':
			out = append(out, mdCell{s: "\n"})
			i++
			continue
		case c == '\\' && i+1 < len(s) && (s[i+1] == '(' || s[i+1] == '['):
			end := `\)`
			if s[i+1] == '[' {
				end = `\]`
			}
			if j := strings.Index(string(s[i+2:]), end); j >= 0 {
				src := []rune(string(s[i+2:])[:j])
				out = mathCells(renderMath(string(src), false), 0, out)
				i += 2 + len(src) + 2
				continue
			}
		case c == '\\' && i+1 < len(s) && strings.ContainsRune("\\`*_{}[]()#+-.!|~$<>\"'=:", s[i+1]):
			lit(s[i+1])
			i += 2
			continue
		case c == '`':
			n := countRun(s, i)
			run := strings.Repeat("`", n)
			if j := strings.Index(string(s[i+n:]), run); j >= 0 {
				code := []rune(string(s[i+n:])[:j])
				t := string(code)
				if len(t) > 1 && t[0] == ' ' && t[len(t)-1] == ' ' {
					t = t[1 : len(t)-1]
				}
				out = cellsOf(t, kCode, at&^aUnder, out)
				i += n + len(code) + n
				continue
			}
			out = cellsOf(run, k, at, out)
			i += n
			continue
		case c == '$':
			if i+1 < len(s) && s[i+1] == '$' {
				if j := findRun(s, i+2, "$$"); j > i+2 {
					out = mathCells(renderMath(string(s[i+2:j]), false), 0, out)
					i = j + 2
					continue
				}
			} else if i+1 < len(s) && s[i+1] != ' ' {
				// como en pandoc: el $ que cierra no va después de un espacio ni antes de un dígito
				for j := i + 1; j < len(s); j++ {
					if s[j] == '\\' {
						j++
						continue
					}
					if s[j] == '$' {
						if s[j-1] != ' ' && (j+1 >= len(s) || !unicode.IsDigit(s[j+1])) {
							out = mathCells(renderMath(string(s[i+1:j]), false), 0, out)
							i = j + 1
							goto next
						}
						break
					}
				}
			}
		case c == '*' || c == '_' || c == '~':
			n := countRun(s, i)
			if c == '~' && n != 2 || n > 3 {
				break
			}
			if c == '_' && i > 0 && isAlnum(s[i-1]) {
				break
			}
			if i+n >= len(s) || s[i+n] == ' ' {
				break
			}
			run := string(s[i : i+n])
			for j := i + n; ; {
				j = findRun(s, j, run)
				if j < 0 {
					break
				}
				after := j + n
				if s[j-1] != ' ' && (after >= len(s) || s[after] != c) && !(c == '_' && after < len(s) && isAlnum(s[after])) {
					var add mdAttr
					switch {
					case c == '~':
						add = aStrike
					case n == 1:
						add = aItalic
					case n == 2:
						add = aBold
					default:
						add = aBold | aItalic
					}
					out = inlineMD(s[i+n:j], k, at|add, out)
					i = after
					goto next
				}
				j++
			}
		case c == '!' && i+1 < len(s) && s[i+1] == '[':
			if text, j, ok := linkAt(s, i+1); ok {
				out = cellsOf("[imagen: "+string(text)+"]", kDim, at, out)
				i = j
				continue
			}
		case c == '[':
			if text, j, ok := linkAt(s, i); ok {
				out = inlineMD(text, kLink, at, out)
				i = j
				continue
			}
		case c == '<':
			j := i + 1
			for j < len(s) && s[j] != '>' && s[j] != ' ' {
				j++
			}
			if j < len(s) && s[j] == '>' {
				tag := strings.ToLower(string(s[i+1 : j]))
				switch {
				case strings.Contains(tag, "://") || strings.Contains(tag, "@") && !strings.HasPrefix(tag, "/"):
					out = cellsOf(string(s[i+1:j]), kLink, at, out)
				case tag == "br" || tag == "br/":
					out = append(out, mdCell{s: "\n"})
				case reTag.MatchString(tag):
					// etiqueta HTML simple (<b>, </sup>…): se omite
				default:
					goto plain
				}
				i = j + 1
				continue
			}
		}
	plain:
		lit(c)
		i++
	next:
	}
	return out
}

// linkAt reconoce [texto](destino) o [texto][ref] empezando en s[i] == '['.
func linkAt(s []rune, i int) ([]rune, int, bool) {
	depth := 0
	for j := i; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case '[':
			depth++
		case ']':
			depth--
			if depth > 0 {
				continue
			}
			text := s[i+1 : j]
			if j+1 < len(s) && (s[j+1] == '(' || s[j+1] == '[') {
				closer := ')'
				if s[j+1] == '[' {
					closer = ']'
				}
				for e := j + 2; e < len(s); e++ {
					if s[e] == closer {
						return text, e + 1, true
					}
				}
			}
			return nil, 0, false
		}
	}
	return nil, 0, false
}

// ---------- cortar en renglones ----------

// wrapCells corta en renglones de width columnas; first y rest son lo que va
// al principio del primer renglón y de los siguientes (viñetas, sangría).
func wrapCells(cs []mdCell, width int, first, rest []mdCell) [][]mdCell {
	var lines [][]mdCell
	line := append([]mdCell(nil), first...)
	pre := len(first)
	pending := false
	flush := func() {
		lines = append(lines, line)
		line = append([]mdCell(nil), rest...)
		pre = len(rest)
		pending = false
	}
	for i := 0; i < len(cs); {
		c := cs[i]
		if c.s == "\n" {
			flush()
			i++
			continue
		}
		if c.s == " " && !c.nb {
			pending = len(line) > pre
			i++
			continue
		}
		j := i
		for j < len(cs) && cs[j].s != "\n" && !(cs[j].s == " " && !cs[j].nb) {
			j++
		}
		word := cs[i:j]
		need := len(word)
		if pending {
			need++
		}
		if len(line)+need > width && len(line) > pre {
			flush()
		}
		if pending {
			sp := cs[i-1]
			line = append(line, sp)
		}
		for len(line)+len(word) > width && width-len(line) > 0 && len(word) > width-pre {
			cut := width - len(line)
			if cut < len(word) && word[cut].s == "" {
				cut--
			}
			if cut <= 0 {
				break
			}
			line = append(line, word[:cut]...)
			word = word[cut:]
			flush()
		}
		line = append(line, word...)
		pending = false
		i = j
	}
	if len(line) > pre || len(lines) == 0 {
		lines = append(lines, line)
	}
	return lines
}

// ---------- bloques ----------

var (
	reHeading = regexp.MustCompile(`^ {0,3}(#{1,6})(?:\s+(.*?))?(?:\s+#+)?\s*$`)
	reRule    = regexp.MustCompile(`^ {0,3}((\*\s*){3,}|(-\s*){3,}|(_\s*){3,})$`)
	reList    = regexp.MustCompile(`^(\s*)([-*+]|\d{1,9}[.)])(\s+|$)(.*)$`)
	reFence   = regexp.MustCompile("^(\\s*)(```+|~~~+)\\s*([^`\\s]*)")
	reTblSep  = regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`)
	reSetext1 = regexp.MustCompile(`^ {0,3}=+\s*$`)
	reSetext2 = regexp.MustCompile(`^ {0,3}-+\s*$`)
	reTag     = regexp.MustCompile(`^/?[a-z][a-z0-9]*/?$`)
	reEnv     = regexp.MustCompile(`^\s*\\begin\{(equation|align|gather|multline|eqnarray|displaymath|math)\*?\}`)
)

type mdRenderer struct {
	width int
	out   []mdLine
}

func (r *mdRenderer) emit(cells []mdCell, src int) {
	r.out = append(r.out, mdLine{cells: cells, src: src})
}

func (r *mdRenderer) blankLine(src int) {
	if n := len(r.out); n > 0 && len(r.out[n-1].cells) > 0 {
		r.emit(nil, src)
	}
}

// renderMarkdown arma las líneas de la vista previa para un ancho dado.
func renderMarkdown(lines []string, width int) []mdLine {
	r := &mdRenderer{width: max(10, width)}
	r.blocks(lines, 0)
	for len(r.out) > 0 && len(r.out[len(r.out)-1].cells) == 0 {
		r.out = r.out[:len(r.out)-1]
	}
	return r.out
}

func isBlockStart(lines []string, i int) bool {
	l := lines[i]
	t := strings.TrimSpace(l)
	return reHeading.MatchString(l) || reRule.MatchString(l) || reFence.MatchString(l) ||
		strings.HasPrefix(t, "$$") || strings.HasPrefix(t, `\[`) || strings.HasPrefix(t, ">") ||
		reList.MatchString(l) && strings.TrimSpace(reList.FindStringSubmatch(l)[4]) != "" ||
		reEnv.MatchString(l) || isTable(lines, i)
}

func isTable(lines []string, i int) bool {
	return strings.Contains(lines[i], "|") && i+1 < len(lines) && reTblSep.MatchString(lines[i+1]) && strings.Contains(lines[i+1], "-")
}

func (r *mdRenderer) blocks(lines []string, src0 int) {
	i := 0
	if src0 == 0 && len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		// encabezado YAML
		for j := 1; j < len(lines); j++ {
			if t := strings.TrimSpace(lines[j]); t == "---" || t == "..." {
				for k := 1; k < j; k++ {
					r.emit(cellsOf(lines[k], kDim, 0, nil), k)
				}
				r.emit(cellsOf(strings.Repeat("─", r.width), kRule, 0, nil), j)
				i = j + 1
				break
			}
		}
	}
	for i < len(lines) {
		line := lines[i]
		t := strings.TrimSpace(line)
		src := src0 + i
		switch {
		case t == "":
			r.blankLine(src)
			i++
		case strings.HasPrefix(t, "<!--"):
			for i < len(lines) && !strings.Contains(lines[i], "-->") {
				i++
			}
			i++
		case reFence.MatchString(line):
			m := reFence.FindStringSubmatch(line)
			indent, fence, lang := len(m[1]), m[2], strings.ToLower(m[3])
			var body []string
			j := i + 1
			for ; j < len(lines); j++ {
				if strings.HasPrefix(strings.TrimSpace(lines[j]), fence) {
					break
				}
				l := lines[j]
				k := 0
				for k < indent && k < len(l) && l[k] == ' ' {
					k++
				}
				body = append(body, l[k:])
			}
			if lang == "math" || lang == "latex" || lang == "tex" && looksLikeMath(body) {
				r.displayMath(strings.Join(body, "\n"), src)
			} else {
				r.code(body, lang, src+1)
			}
			i = j + 1
		case strings.HasPrefix(t, "$$") || strings.HasPrefix(t, `\[`):
			open, close := "$$", "$$"
			if strings.HasPrefix(t, `\[`) {
				open, close = `\[`, `\]`
			}
			rest := strings.TrimPrefix(t, open)
			var sb strings.Builder
			j := i
			for {
				if k := strings.Index(rest, close); k >= 0 {
					sb.WriteString(rest[:k])
					break
				}
				sb.WriteString(rest + "\n")
				j++
				if j >= len(lines) {
					break
				}
				rest = lines[j]
			}
			r.displayMath(sb.String(), src)
			i = j + 1
		case reEnv.MatchString(line):
			name := reEnv.FindStringSubmatch(line)[1]
			end := regexp.MustCompile(`\\end\{` + name + `\*?\}`)
			j := i
			for j < len(lines)-1 && !end.MatchString(lines[j]) {
				j++
			}
			body := strings.Join(lines[i:j+1], "\n")
			if name == "equation" || name == "displaymath" || name == "math" {
				body = regexp.MustCompile(`\\(begin|end)\{\w+\*?\}`).ReplaceAllString(body, "")
			}
			r.displayMath(body, src)
			i = j + 1
		case reHeading.MatchString(line):
			m := reHeading.FindStringSubmatch(line)
			r.heading(len(m[1]), m[2], src)
			i++
		case reRule.MatchString(line):
			r.emit(cellsOf(strings.Repeat("─", r.width), kRule, 0, nil), src)
			i++
		case strings.HasPrefix(t, ">"):
			var inner []string
			j := i
			for ; j < len(lines); j++ {
				q := strings.TrimLeft(lines[j], " ")
				if !strings.HasPrefix(q, ">") {
					break
				}
				q = strings.TrimPrefix(q, ">")
				q = strings.TrimPrefix(q, " ")
				inner = append(inner, q)
			}
			sub := &mdRenderer{width: max(4, r.width-2)}
			sub.blocks(inner, src)
			for len(sub.out) > 0 && len(sub.out[len(sub.out)-1].cells) == 0 {
				sub.out = sub.out[:len(sub.out)-1]
			}
			for _, l := range sub.out {
				cs := []mdCell{{s: "│", k: kQuote}, {s: " ", k: kQuote}}
				for _, c := range l.cells {
					if c.k == kText {
						c.k = kQuote
					}
					cs = append(cs, c)
				}
				r.emit(cs, l.src)
			}
			i = j
		case isTable(lines, i):
			i = r.table(lines, i, src0)
		case reList.MatchString(line):
			i = r.list(lines, i, src0)
		case strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t"):
			var body []string
			j := i
			for ; j < len(lines); j++ {
				l := lines[j]
				if strings.TrimSpace(l) == "" {
					body = append(body, "")
					continue
				}
				if !strings.HasPrefix(l, "    ") && !strings.HasPrefix(l, "\t") {
					break
				}
				body = append(body, strings.TrimPrefix(strings.TrimPrefix(l, "\t"), "    "))
			}
			for len(body) > 0 && body[len(body)-1] == "" {
				body = body[:len(body)-1]
			}
			r.code(body, "", src)
			i = j
		default:
			i = r.paragraph(lines, i, src0)
		}
	}
}

func looksLikeMath(body []string) bool {
	s := strings.Join(body, "\n")
	return !strings.Contains(s, "\\documentclass") && !strings.Contains(s, "\\usepackage")
}

func (r *mdRenderer) heading(level int, text string, src int) {
	k := kH3
	switch level {
	case 1:
		k = kH1
	case 2:
		k = kH2
	}
	cs := inlineMD([]rune(text), k, aBold, nil)
	ls := wrapCells(cs, r.width, nil, nil)
	for _, l := range ls {
		r.emit(l, src)
	}
	if level <= 2 {
		w := 0
		for _, l := range ls {
			w = max(w, len(l))
		}
		ch := "═"
		if level == 2 {
			ch = "─"
		}
		r.emit(cellsOf(strings.Repeat(ch, max(3, w)), kRule, 0, nil), src)
	}
}

func (r *mdRenderer) code(body []string, lang string, src int) {
	for k, l := range body {
		l = strings.ReplaceAll(l, "\t", "    ")
		r.emit(cellsOf("  "+l, kCode, 0, nil), src+k)
	}
}

func (r *mdRenderer) displayMath(src string, line int) {
	b := renderMath(src, true)
	pad := max(0, (r.width-b.w)/2)
	for y := range b.c {
		cs := cellsOf(strings.Repeat(" ", pad), kText, 0, nil)
		r.emit(mathCells(b, y, cs), line)
	}
}

// paragraph junta líneas seguidas hasta una vacía o el comienzo de otro bloque.
func (r *mdRenderer) paragraph(lines []string, i, src0 int) int {
	var sb strings.Builder
	j := i
	for ; j < len(lines); j++ {
		l := lines[j]
		if strings.TrimSpace(l) == "" || j > i && isBlockStart(lines, j) && !reSetext2.MatchString(l) {
			break
		}
		if j > i && (reSetext1.MatchString(l) || reSetext2.MatchString(l)) {
			level := 1
			if reSetext2.MatchString(l) {
				level = 2
			}
			r.heading(level, strings.TrimSpace(sb.String()), src0+i)
			return j + 1
		}
		if j > i {
			prev := lines[j-1]
			if strings.HasSuffix(prev, "  ") || strings.HasSuffix(prev, "\\") && !strings.HasSuffix(prev, "\\\\") {
				sb.WriteByte('\n')
			} else {
				sb.WriteByte(' ')
			}
		}
		l = strings.TrimSpace(l)
		if j+1 < len(lines) && strings.HasSuffix(l, "\\") && !strings.HasSuffix(l, "\\\\") {
			l = strings.TrimSuffix(l, "\\")
		}
		sb.WriteString(l)
	}
	if j == i { // una línea que parecía otro bloque pero no lo es
		sb.WriteString(strings.TrimSpace(lines[i]))
		j++
	}
	for _, l := range wrapCells(inlineMD([]rune(sb.String()), kText, 0, nil), r.width, nil, nil) {
		r.emit(l, src0+i)
	}
	return j
}

func (r *mdRenderer) list(lines []string, i, src0 int) int {
	var indents []int
	for i < len(lines) {
		m := reList.FindStringSubmatch(lines[i])
		if m == nil {
			break
		}
		ind := len(strings.ReplaceAll(m[1], "\t", "    "))
		for len(indents) > 0 && ind < indents[len(indents)-1] {
			indents = indents[:len(indents)-1]
		}
		if len(indents) == 0 || ind > indents[len(indents)-1] {
			indents = append(indents, ind)
		}
		level := len(indents) - 1
		marker := m[2]
		if marker == "-" || marker == "*" || marker == "+" {
			marker = []string{"•", "◦", "▪"}[level%3]
		}
		text := m[4]
		switch {
		case strings.HasPrefix(text, "[ ] "):
			marker, text = "☐", text[4:]
		case strings.HasPrefix(text, "[x] "), strings.HasPrefix(text, "[X] "):
			marker, text = "☒", text[4:]
		}
		src := src0 + i
		// líneas que siguen al ítem sin ser otro ítem ni otro bloque
		j := i + 1
		for ; j < len(lines); j++ {
			l := lines[j]
			if strings.TrimSpace(l) == "" {
				if j+1 < len(lines) && reList.MatchString(lines[j+1]) {
					j++ // lista «suelta»: el renglón vacío no la corta
				}
				break
			}
			if reList.MatchString(l) || isBlockStart(lines, j) {
				break
			}
			text += " " + strings.TrimSpace(l)
		}
		first := cellsOf(strings.Repeat("  ", level), kText, 0, nil)
		first = cellsOf(marker, kRule, 0, first)
		first = cellsOf(" ", kText, 0, first)
		rest := cellsOf(strings.Repeat(" ", len(first)), kText, 0, nil)
		for _, l := range wrapCells(inlineMD([]rune(strings.TrimSpace(text)), kText, 0, nil), r.width, first, rest) {
			r.emit(l, src)
		}
		i = j
		if i < len(lines) && strings.TrimSpace(lines[i]) == "" {
			break
		}
	}
	return i
}

// splitRow parte una fila de tabla por los | que no están dentro de `código` o $fórmulas$.
func splitRow(l string) []string {
	l = strings.TrimSpace(l)
	l = strings.TrimPrefix(l, "|")
	if strings.HasSuffix(l, "|") && !strings.HasSuffix(l, "\\|") {
		l = l[:len(l)-1]
	}
	var cells []string
	var cur strings.Builder
	code, math := false, false
	rs := []rune(l)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case c == '\\' && i+1 < len(rs) && rs[i+1] == '|':
			cur.WriteString("|")
			i++
			continue
		case c == '`':
			code = !code
		case c == '$' && !code:
			math = !math
		case c == '|' && !code && !math:
			cells = append(cells, strings.TrimSpace(cur.String()))
			cur.Reset()
			continue
		}
		cur.WriteRune(c)
	}
	return append(cells, strings.TrimSpace(cur.String()))
}

func (r *mdRenderer) table(lines []string, i, src0 int) int {
	head := splitRow(lines[i])
	seps := splitRow(lines[i+1])
	n := len(head)
	aligns := make([]byte, n)
	for k := 0; k < n && k < len(seps); k++ {
		s := seps[k]
		switch {
		case strings.HasPrefix(s, ":") && strings.HasSuffix(s, ":"):
			aligns[k] = 'c'
		case strings.HasSuffix(s, ":"):
			aligns[k] = 'r'
		}
	}
	rows := [][]string{head}
	srcs := []int{src0 + i}
	j := i + 2
	for ; j < len(lines) && strings.Contains(lines[j], "|") && strings.TrimSpace(lines[j]) != ""; j++ {
		rows = append(rows, splitRow(lines[j]))
		srcs = append(srcs, src0+j)
	}
	cells := make([][][]mdCell, len(rows))
	colW := make([]int, n)
	for y, row := range rows {
		cells[y] = make([][]mdCell, n)
		for x := 0; x < n; x++ {
			t := ""
			if x < len(row) {
				t = row[x]
			}
			var at mdAttr
			if y == 0 {
				at = aBold
			}
			cells[y][x] = inlineMD([]rune(t), kText, at, nil)
			colW[x] = max(colW[x], len(cells[y][x]))
		}
	}
	// si no entra, se achican las columnas más anchas (y su texto se corta en renglones)
	for {
		total := 1
		big := 0
		for x, w := range colW {
			total += w + 3
			if w > colW[big] {
				big = x
			}
		}
		if total <= r.width || colW[big] <= 4 {
			break
		}
		colW[big]--
	}
	border := func(l, m, rr string) []mdCell {
		var sb strings.Builder
		sb.WriteString(l)
		for x, w := range colW {
			sb.WriteString(strings.Repeat("─", w+2))
			if x < n-1 {
				sb.WriteString(m)
			}
		}
		sb.WriteString(rr)
		return cellsOf(sb.String(), kRule, 0, nil)
	}
	bar := mdCell{s: "│", k: kRule}
	sp := mdCell{s: " "}
	r.emit(border("┌", "┬", "┐"), srcs[0])
	for y := range rows {
		wrapped := make([][][]mdCell, n)
		h := 1
		for x := 0; x < n; x++ {
			wrapped[x] = wrapCells(cells[y][x], max(1, colW[x]), nil, nil)
			h = max(h, len(wrapped[x]))
		}
		for k := 0; k < h; k++ {
			line := []mdCell{bar}
			for x := 0; x < n; x++ {
				var c []mdCell
				if k < len(wrapped[x]) {
					c = wrapped[x][k]
				}
				if len(c) > colW[x] {
					c = c[:colW[x]]
				}
				padL := 0
				switch aligns[x] {
				case 'r':
					padL = colW[x] - len(c)
				case 'c':
					padL = (colW[x] - len(c)) / 2
				}
				line = append(line, sp)
				for p := 0; p < padL; p++ {
					line = append(line, sp)
				}
				line = append(line, c...)
				for p := padL + len(c); p < colW[x]; p++ {
					line = append(line, sp)
				}
				line = append(line, sp, bar)
			}
			r.emit(line, srcs[y])
		}
		if y == 0 && len(rows) > 1 {
			r.emit(border("├", "┼", "┤"), srcs[0])
		}
	}
	r.emit(border("└", "┴", "┘"), srcs[len(srcs)-1])
	return j
}

func isMarkdownPath(p string) bool {
	if p == "" {
		return true
	}
	switch strings.ToLower(filepath.Ext(p)) {
	case ".md", ".markdown", ".mdown", ".mkd", ".mkdn", ".mdx", ".qmd", ".rmd", ".txt":
		return true
	}
	return false
}
