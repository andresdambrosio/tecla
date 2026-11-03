package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
)

const tabW = 4

// Pos es una posición en el buffer: línea Y, índice de rune X.
type Pos struct{ Y, X int }

func (p Pos) Less(q Pos) bool { return p.Y < q.Y || (p.Y == q.Y && p.X < q.X) }

type snapshot struct {
	lines [][]rune
	cur   Pos
}

type Buffer struct {
	Lines            [][]rune
	Cur              Pos
	Anchor           *Pos // ancla de la selección; nil = sin selección
	WantV            int  // columna visual deseada al moverse verticalmente
	Path             string
	Dirty            bool
	CRLF             bool
	IndentTab        bool
	ScrollY, ScrollX int
	Version          int
	undo, redo       []snapshot
	lastOp           string
}

func NewBuffer(path string) *Buffer {
	return &Buffer{Lines: [][]rune{{}}, Path: path, IndentTab: defaultTabs(path)}
}

func defaultTabs(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return strings.HasSuffix(base, ".go") || base == "makefile" || strings.HasSuffix(base, ".mk")
}

func LoadBuffer(path string) (*Buffer, error) {
	b := NewBuffer(path)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return b, nil
		}
		return nil, err
	}
	if strings.IndexByte(string(data[:min(len(data), 8000)]), 0) >= 0 {
		return nil, fmt.Errorf("%s parece un archivo binario", filepath.Base(path))
	}
	b.setText(string(data))
	b.detectIndent()
	return b, nil
}

func (b *Buffer) setText(s string) {
	b.CRLF = strings.Contains(s, "\r\n")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	parts := strings.Split(s, "\n")
	b.Lines = make([][]rune, len(parts))
	for i, p := range parts {
		b.Lines[i] = []rune(p)
	}
	b.Version++
}

func (b *Buffer) detectIndent() {
	tabs, spaces := 0, 0
	for _, l := range b.Lines {
		if len(l) > 1 {
			if l[0] == '\t' {
				tabs++
			} else if l[0] == ' ' && l[1] == ' ' {
				spaces++
			}
		}
	}
	if tabs > spaces {
		b.IndentTab = true
	} else if spaces > tabs {
		b.IndentTab = false
	}
}

func (b *Buffer) Text() string {
	parts := make([]string, len(b.Lines))
	for i, l := range b.Lines {
		parts[i] = string(l)
	}
	sep := "\n"
	if b.CRLF {
		sep = "\r\n"
	}
	return strings.Join(parts, sep)
}

func (b *Buffer) Save(path string) error {
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	if err := os.WriteFile(path, []byte(b.Text()), mode); err != nil {
		return err
	}
	b.Path = path
	b.Dirty = false
	return nil
}

// ReloadText reemplaza el contenido (por ejemplo, tras una edición del agente) de forma deshacible.
func (b *Buffer) ReloadText(s string) {
	b.push("reload")
	b.setText(s)
	b.Anchor = nil
	b.clamp()
	b.Dirty = false
	b.lastOp = ""
}

// ---------- deshacer ----------

func (b *Buffer) snap() snapshot {
	ls := make([][]rune, len(b.Lines))
	for i, l := range b.Lines {
		ls[i] = append([]rune(nil), l...)
	}
	return snapshot{ls, b.Cur}
}

func (b *Buffer) push(op string) {
	if op == b.lastOp && (op == "type" || op == "bs" || op == "del") {
		return
	}
	b.undo = append(b.undo, b.snap())
	if len(b.undo) > 500 {
		b.undo = b.undo[1:]
	}
	b.redo = nil
	b.lastOp = op
}

func (b *Buffer) restore(s snapshot) {
	b.Lines = s.lines
	b.Cur = s.cur
	b.Anchor = nil
	b.Version++
	b.Dirty = true
	b.clamp()
	b.WantV = b.vcur()
	b.lastOp = ""
}

func (b *Buffer) Undo() bool {
	if len(b.undo) == 0 {
		return false
	}
	b.redo = append(b.redo, b.snap())
	s := b.undo[len(b.undo)-1]
	b.undo = b.undo[:len(b.undo)-1]
	b.restore(s)
	return true
}

func (b *Buffer) Redo() bool {
	if len(b.redo) == 0 {
		return false
	}
	b.undo = append(b.undo, b.snap())
	s := b.redo[len(b.redo)-1]
	b.redo = b.redo[:len(b.redo)-1]
	b.restore(s)
	return true
}

// ---------- selección ----------

func (b *Buffer) HasSel() bool { return b.Anchor != nil && *b.Anchor != b.Cur }

func (b *Buffer) SelRange() (Pos, Pos) {
	a, c := *b.Anchor, b.Cur
	if c.Less(a) {
		return c, a
	}
	return a, c
}

func (b *Buffer) InSel(y, x int) bool {
	if !b.HasSel() {
		return false
	}
	s, e := b.SelRange()
	p := Pos{y, x}
	return !p.Less(s) && p.Less(e)
}

func (b *Buffer) validPos(p Pos) bool {
	return p.Y >= 0 && p.Y < len(b.Lines) && p.X >= 0 && p.X <= len(b.Lines[p.Y])
}

func (b *Buffer) TextRange(s, e Pos) string {
	if s.Y == e.Y {
		return string(b.Lines[s.Y][s.X:e.X])
	}
	var sb strings.Builder
	sb.WriteString(string(b.Lines[s.Y][s.X:]))
	for y := s.Y + 1; y < e.Y; y++ {
		sb.WriteByte('\n')
		sb.WriteString(string(b.Lines[y]))
	}
	sb.WriteByte('\n')
	sb.WriteString(string(b.Lines[e.Y][:e.X]))
	return sb.String()
}

func (b *Buffer) SelectAll() {
	p := Pos{0, 0}
	b.Anchor = &p
	last := len(b.Lines) - 1
	b.Cur = Pos{last, len(b.Lines[last])}
	b.lastOp = ""
}

func (b *Buffer) SetSel(s, e Pos) {
	b.Anchor = &s
	b.Cur = e
	b.WantV = b.vcur()
	b.lastOp = ""
}

// ---------- primitivas de edición (sin deshacer) ----------

func (b *Buffer) insertAt(p Pos, text string) Pos {
	parts := strings.Split(text, "\n")
	line := b.Lines[p.Y]
	before := append([]rune(nil), line[:p.X]...)
	after := append([]rune(nil), line[p.X:]...)
	b.Version++
	b.Dirty = true
	if len(parts) == 1 {
		r := []rune(parts[0])
		b.Lines[p.Y] = append(append(before, r...), after...)
		return Pos{p.Y, p.X + len(r)}
	}
	nl := make([][]rune, 0, len(parts))
	nl = append(nl, append(before, []rune(parts[0])...))
	for _, m := range parts[1 : len(parts)-1] {
		nl = append(nl, []rune(m))
	}
	last := []rune(parts[len(parts)-1])
	end := Pos{p.Y + len(parts) - 1, len(last)}
	nl = append(nl, append(last, after...))
	rest := append([][]rune(nil), b.Lines[p.Y+1:]...)
	b.Lines = append(append(b.Lines[:p.Y], nl...), rest...)
	return end
}

func (b *Buffer) deleteRange(s, e Pos) {
	b.Version++
	b.Dirty = true
	if s.Y == e.Y {
		l := b.Lines[s.Y]
		b.Lines[s.Y] = append(append([]rune(nil), l[:s.X]...), l[e.X:]...)
		return
	}
	nl := append(append([]rune(nil), b.Lines[s.Y][:s.X]...), b.Lines[e.Y][e.X:]...)
	rest := append([][]rune(nil), b.Lines[e.Y+1:]...)
	b.Lines = append(append(b.Lines[:s.Y], nl), rest...)
}

func (b *Buffer) unit() string {
	if b.IndentTab {
		return "\t"
	}
	return strings.Repeat(" ", tabW)
}

// ---------- edición ----------

func normalizeNL(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

func (b *Buffer) InsertText(text, op string) {
	text = normalizeNL(text)
	b.push(op)
	if b.HasSel() {
		s, e := b.SelRange()
		b.deleteRange(s, e)
		b.Cur = s
	}
	b.Anchor = nil
	b.Cur = b.insertAt(b.Cur, text)
	b.WantV = b.vcur()
}

func (b *Buffer) DeleteSel() bool {
	if !b.HasSel() {
		return false
	}
	b.push("cut")
	s, e := b.SelRange()
	b.deleteRange(s, e)
	b.Cur = s
	b.Anchor = nil
	b.WantV = b.vcur()
	b.lastOp = ""
	return true
}

func allSpace(r []rune) bool {
	for _, c := range r {
		if c != ' ' && c != '\t' {
			return false
		}
	}
	return true
}

func (b *Buffer) TypeRune(r rune) {
	b.push("type")
	if b.HasSel() {
		s, e := b.SelRange()
		b.deleteRange(s, e)
		b.Cur = s
	}
	b.Anchor = nil
	// des-indentar al cerrar un bloque en una línea vacía
	if r == '}' || r == ']' || r == ')' {
		l := b.Lines[b.Cur.Y]
		u := []rune(b.unit())
		if b.Cur.X == len(l) && len(l) >= len(u) && allSpace(l) {
			b.Lines[b.Cur.Y] = append([]rune(nil), l[:len(l)-len(u)]...)
			b.Cur.X -= len(u)
			b.Version++
		}
	}
	b.Cur = b.insertAt(b.Cur, string(r))
	b.WantV = b.vcur()
	if r == ' ' {
		b.lastOp = ""
	}
}

func lastNonSpace(r []rune) rune {
	for i := len(r) - 1; i >= 0; i-- {
		if r[i] != ' ' && r[i] != '\t' {
			return r[i]
		}
	}
	return 0
}

func (b *Buffer) Newline() {
	l := b.Lines[b.Cur.Y]
	i := 0
	for i < len(l) && i < b.Cur.X && (l[i] == ' ' || l[i] == '\t') {
		i++
	}
	indent := string(l[:i])
	switch lastNonSpace(l[:b.Cur.X]) {
	case '{', '(', '[', ':':
		indent += b.unit()
	}
	b.InsertText("\n"+indent, "newline")
	b.lastOp = ""
}

func (b *Buffer) Backspace() {
	if b.DeleteSel() {
		return
	}
	if b.Cur.X == 0 && b.Cur.Y == 0 {
		return
	}
	b.push("bs")
	if b.Cur.X > 0 {
		l := b.Lines[b.Cur.Y]
		n := 1
		if !b.IndentTab && b.Cur.X > 0 && strings.Trim(string(l[:b.Cur.X]), " ") == "" {
			n = (b.Cur.X-1)%tabW + 1
		}
		s := Pos{b.Cur.Y, b.Cur.X - n}
		b.deleteRange(s, b.Cur)
		b.Cur = s
	} else {
		py := b.Cur.Y - 1
		s := Pos{py, len(b.Lines[py])}
		b.deleteRange(s, b.Cur)
		b.Cur = s
	}
	b.WantV = b.vcur()
}

func (b *Buffer) DeleteFwd() {
	if b.DeleteSel() {
		return
	}
	l := b.Lines[b.Cur.Y]
	var e Pos
	if b.Cur.X < len(l) {
		e = Pos{b.Cur.Y, b.Cur.X + 1}
	} else if b.Cur.Y < len(b.Lines)-1 {
		e = Pos{b.Cur.Y + 1, 0}
	} else {
		return
	}
	b.push("del")
	b.deleteRange(b.Cur, e)
}

func (b *Buffer) DeleteWordBack() {
	if b.DeleteSel() {
		return
	}
	start := b.Cur
	b.WordLeft(false)
	if b.Cur == start {
		return
	}
	b.push("cut")
	b.deleteRange(b.Cur, start)
	b.lastOp = ""
	b.WantV = b.vcur()
}

func (b *Buffer) selLines() (int, int) {
	if !b.HasSel() {
		return b.Cur.Y, b.Cur.Y
	}
	s, e := b.SelRange()
	ey := e.Y
	if e.X == 0 && e.Y > s.Y {
		ey--
	}
	return s.Y, ey
}

func (b *Buffer) selectLines(y0, y1 int) {
	s := Pos{y0, 0}
	b.Anchor = &s
	b.Cur = Pos{y1, len(b.Lines[y1])}
	b.WantV = b.vcur()
}

func (b *Buffer) Tab() {
	if b.HasSel() {
		s, e := b.SelRange()
		if s.Y != e.Y {
			b.Indent()
			return
		}
	}
	b.InsertText(b.unit(), "type")
}

func (b *Buffer) Indent() {
	b.push("indent")
	y0, y1 := b.selLines()
	u := []rune(b.unit())
	for y := y0; y <= y1; y++ {
		if len(b.Lines[y]) > 0 {
			b.Lines[y] = append(append([]rune(nil), u...), b.Lines[y]...)
		}
	}
	b.Version++
	b.Dirty = true
	b.selectLines(y0, y1)
	b.lastOp = ""
}

func (b *Buffer) Unindent() {
	b.push("indent")
	y0, y1 := b.selLines()
	hadSel := b.HasSel()
	for y := y0; y <= y1; y++ {
		l := b.Lines[y]
		n := 0
		if len(l) > 0 && l[0] == '\t' {
			n = 1
		} else {
			for n < tabW && n < len(l) && l[n] == ' ' {
				n++
			}
		}
		b.Lines[y] = append([]rune(nil), l[n:]...)
		if y == b.Cur.Y && !hadSel {
			b.Cur.X = max(0, b.Cur.X-n)
		}
	}
	b.Version++
	b.Dirty = true
	if hadSel {
		b.selectLines(y0, y1)
	}
	b.lastOp = ""
}

func (b *Buffer) ToggleComment(prefix string) {
	if prefix == "" {
		return
	}
	y0, y1 := b.selLines()
	p := []rune(prefix + " ")
	all := true
	minInd := -1
	for y := y0; y <= y1; y++ {
		l := b.Lines[y]
		if allSpace(l) {
			continue
		}
		i := 0
		for i < len(l) && (l[i] == ' ' || l[i] == '\t') {
			i++
		}
		if minInd < 0 || i < minInd {
			minInd = i
		}
		if !strings.HasPrefix(string(l[i:]), prefix) {
			all = false
		}
	}
	if minInd < 0 {
		return
	}
	b.push("comment")
	for y := y0; y <= y1; y++ {
		l := b.Lines[y]
		if allSpace(l) {
			continue
		}
		if all {
			i := 0
			for i < len(l) && (l[i] == ' ' || l[i] == '\t') {
				i++
			}
			rest := strings.TrimPrefix(string(l[i:]), prefix)
			rest = strings.TrimPrefix(rest, " ")
			b.Lines[y] = append(append([]rune(nil), l[:i]...), []rune(rest)...)
		} else {
			nl := append([]rune(nil), l[:minInd]...)
			nl = append(nl, p...)
			b.Lines[y] = append(nl, l[minInd:]...)
		}
	}
	b.Version++
	b.Dirty = true
	if b.HasSel() {
		b.selectLines(y0, y1)
	}
	b.clamp()
	b.lastOp = ""
}

func (b *Buffer) DuplicateLine() {
	b.push("dup")
	y := b.Cur.Y
	cp := append([]rune(nil), b.Lines[y]...)
	rest := append([][]rune(nil), b.Lines[y+1:]...)
	b.Lines = append(append(b.Lines[:y+1], cp), rest...)
	b.Cur.Y++
	b.Anchor = nil
	b.Version++
	b.Dirty = true
	b.lastOp = ""
}

// ---------- movimiento ----------

func runeW(r rune) int {
	w := runewidth.RuneWidth(r)
	if w < 1 {
		return 1
	}
	return w
}

func vcol(line []rune, x int) int {
	c := 0
	for i := 0; i < x && i < len(line); i++ {
		if line[i] == '\t' {
			c += tabW - c%tabW
		} else {
			c += runeW(line[i])
		}
	}
	return c
}

func xFromV(line []rune, v int) int {
	c := 0
	for i, r := range line {
		w := runeW(r)
		if r == '\t' {
			w = tabW - c%tabW
		}
		if c+w > v {
			return i
		}
		c += w
	}
	return len(line)
}

func (b *Buffer) vcur() int { return vcol(b.Lines[b.Cur.Y], b.Cur.X) }

func (b *Buffer) clamp() {
	if len(b.Lines) == 0 {
		b.Lines = [][]rune{{}}
	}
	b.Cur.Y = max(0, min(b.Cur.Y, len(b.Lines)-1))
	b.Cur.X = max(0, min(b.Cur.X, len(b.Lines[b.Cur.Y])))
}

func (b *Buffer) sel(shift bool) {
	b.lastOp = ""
	if shift {
		if b.Anchor == nil {
			p := b.Cur
			b.Anchor = &p
		}
	} else {
		b.Anchor = nil
	}
}

func (b *Buffer) Left(shift bool) {
	if !shift && b.HasSel() {
		s, _ := b.SelRange()
		b.Anchor = nil
		b.Cur = s
	} else {
		b.sel(shift)
		if b.Cur.X > 0 {
			b.Cur.X--
		} else if b.Cur.Y > 0 {
			b.Cur.Y--
			b.Cur.X = len(b.Lines[b.Cur.Y])
		}
	}
	b.WantV = b.vcur()
}

func (b *Buffer) Right(shift bool) {
	if !shift && b.HasSel() {
		_, e := b.SelRange()
		b.Anchor = nil
		b.Cur = e
	} else {
		b.sel(shift)
		if b.Cur.X < len(b.Lines[b.Cur.Y]) {
			b.Cur.X++
		} else if b.Cur.Y < len(b.Lines)-1 {
			b.Cur.Y++
			b.Cur.X = 0
		}
	}
	b.WantV = b.vcur()
}

func (b *Buffer) Up(shift bool, n int) {
	b.sel(shift)
	if b.Cur.Y-n < 0 {
		b.Cur = Pos{0, 0}
		b.WantV = 0
		return
	}
	b.Cur.Y -= n
	b.Cur.X = xFromV(b.Lines[b.Cur.Y], b.WantV)
}

func (b *Buffer) Down(shift bool, n int) {
	b.sel(shift)
	last := len(b.Lines) - 1
	if b.Cur.Y+n > last {
		b.Cur = Pos{last, len(b.Lines[last])}
		b.WantV = b.vcur()
		return
	}
	b.Cur.Y += n
	b.Cur.X = xFromV(b.Lines[b.Cur.Y], b.WantV)
}

func (b *Buffer) Home(shift bool) {
	b.sel(shift)
	l := b.Lines[b.Cur.Y]
	i := 0
	for i < len(l) && (l[i] == ' ' || l[i] == '\t') {
		i++
	}
	if b.Cur.X == i {
		i = 0
	}
	b.Cur.X = i
	b.WantV = b.vcur()
}

func (b *Buffer) End(shift bool) {
	b.sel(shift)
	b.Cur.X = len(b.Lines[b.Cur.Y])
	b.WantV = b.vcur()
}

func (b *Buffer) DocStart(shift bool) {
	b.sel(shift)
	b.Cur = Pos{0, 0}
	b.WantV = 0
}

func (b *Buffer) DocEnd(shift bool) {
	b.sel(shift)
	last := len(b.Lines) - 1
	b.Cur = Pos{last, len(b.Lines[last])}
	b.WantV = b.vcur()
}

func isIdent(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }

func (b *Buffer) WordLeft(shift bool) {
	b.sel(shift)
	if b.Cur.X == 0 {
		if b.Cur.Y > 0 {
			b.Cur.Y--
			b.Cur.X = len(b.Lines[b.Cur.Y])
		}
		b.WantV = b.vcur()
		return
	}
	l := b.Lines[b.Cur.Y]
	x := b.Cur.X
	for x > 0 && !isIdent(l[x-1]) {
		x--
	}
	for x > 0 && isIdent(l[x-1]) {
		x--
	}
	b.Cur.X = x
	b.WantV = b.vcur()
}

func (b *Buffer) WordRight(shift bool) {
	b.sel(shift)
	l := b.Lines[b.Cur.Y]
	if b.Cur.X >= len(l) {
		if b.Cur.Y < len(b.Lines)-1 {
			b.Cur.Y++
			b.Cur.X = 0
		}
		b.WantV = b.vcur()
		return
	}
	x := b.Cur.X
	for x < len(l) && !isIdent(l[x]) {
		x++
	}
	for x < len(l) && isIdent(l[x]) {
		x++
	}
	b.Cur.X = x
	b.WantV = b.vcur()
}

func (b *Buffer) WordAt(p Pos) (Pos, Pos) {
	l := b.Lines[p.Y]
	s, e := p.X, p.X
	for s > 0 && isIdent(l[s-1]) {
		s--
	}
	for e < len(l) && isIdent(l[e]) {
		e++
	}
	return Pos{p.Y, s}, Pos{p.Y, e}
}

func (b *Buffer) GotoLine(n int) {
	b.Anchor = nil
	b.Cur = Pos{n - 1, 0}
	b.clamp()
	b.WantV = 0
	b.lastOp = ""
}

// ---------- búsqueda ----------

func lowerRunes(r []rune) []rune {
	o := make([]rune, len(r))
	for i, c := range r {
		o[i] = unicode.ToLower(c)
	}
	return o
}

func indexRunes(hay, needle []rune, from int) int {
	for i := from; i+len(needle) <= len(hay); i++ {
		ok := true
		for j := range needle {
			if hay[i+j] != needle[j] {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

// matcher: si la búsqueda está en minúsculas, ignora mayúsculas.
func matcher(q string) ([]rune, func([]rune) []rune) {
	nq := []rune(q)
	if strings.ToLower(q) == q {
		return nq, lowerRunes
	}
	return nq, func(r []rune) []rune { return r }
}

func (b *Buffer) Find(q string, from Pos) (Pos, Pos, bool) {
	if q == "" {
		return Pos{}, Pos{}, false
	}
	nq, norm := matcher(q)
	n := len(b.Lines)
	for k := 0; k <= n; k++ {
		y := (from.Y + k) % n
		start := 0
		if k == 0 {
			start = from.X
		}
		if i := indexRunes(norm(b.Lines[y]), nq, start); i >= 0 {
			if k == n && i >= from.X {
				break
			}
			return Pos{y, i}, Pos{y, i + len(nq)}, true
		}
	}
	return Pos{}, Pos{}, false
}

func (b *Buffer) ReplaceAll(q, r string) int {
	if q == "" {
		return 0
	}
	nq, norm := matcher(q)
	rr := []rune(r)
	count := 0
	pushed := false
	for y, l := range b.Lines {
		h := norm(l)
		var out []rune
		last := 0
		for i := indexRunes(h, nq, 0); i >= 0; i = indexRunes(h, nq, i+len(nq)) {
			if !pushed {
				b.push("replace")
				pushed = true
			}
			out = append(out, l[last:i]...)
			out = append(out, rr...)
			last = i + len(nq)
			count++
		}
		if last > 0 {
			out = append(out, l[last:]...)
			b.Lines[y] = out
		}
	}
	if count > 0 {
		b.Version++
		b.Dirty = true
		b.Anchor = nil
		b.clamp()
		b.lastOp = ""
	}
	return count
}
