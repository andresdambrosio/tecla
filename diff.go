package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v2"
)

// Comparación de dos códigos: primero el algoritmo, después la vista a pantalla completa.

const (
	diffCtx      = 3     // líneas iguales que quedan visibles alrededor de cada cambio
	diffSimilar  = 0.4   // parecido mínimo para considerar que una línea es la otra modificada
	diffBlockMax = 40000 // tope de (borradas × agregadas) para buscar parejas dentro de un bloque
)

// diffMaxD limita el trabajo de Myers: pasado este número de ediciones el resultado
// sigue siendo correcto, pero puede dejar de ser el mínimo posible.
var diffMaxD = 3000

// ---------- Myers ----------

// diffMatch devuelve, para cada elemento de a, su pareja en b (o -1). Las parejas son crecientes.
func diffMatch(a, b []int) []int {
	m := make([]int, len(a))
	for i := range m {
		m[i] = -1
	}
	matchInto(a, b, 0, 0, m)
	return m
}

func matchInto(a, b []int, ao, bo int, m []int) {
	for len(a) > 0 && len(b) > 0 && a[0] == b[0] {
		m[ao] = bo
		a, b = a[1:], b[1:]
		ao++
		bo++
	}
	for len(a) > 0 && len(b) > 0 && a[len(a)-1] == b[len(b)-1] {
		m[ao+len(a)-1] = bo + len(b) - 1
		a, b = a[:len(a)-1], b[:len(b)-1]
	}
	n, l := len(a), len(b)
	if n == 0 || l == 0 {
		return
	}
	maxD := min(n+l, diffMaxD)
	off := maxD + 1
	v := make([]int32, 2*maxD+3)
	trace := make([][]int32, 0, 64)
	endX, endY, endD := -1, -1, 0
search:
	for d := 0; d <= maxD; d++ {
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[off+k-1] < v[off+k+1]) {
				x = int(v[off+k+1])
			} else {
				x = int(v[off+k-1]) + 1
			}
			y := x - k
			for x < n && y < l && a[x] == b[y] {
				x++
				y++
			}
			v[off+k] = int32(x)
			if x == n && y == l {
				endX, endY, endD = x, y, d
				break search
			}
		}
		trace = append(trace, append([]int32(nil), v[off-d-1:off+d+2]...))
	}
	if endX < 0 { // demasiadas ediciones: seguimos desde el punto que más avanzó
		endD = maxD
		best := -1
		for k := -maxD; k <= maxD; k += 2 {
			x := int(v[off+k])
			y := x - k
			if x <= n && y >= 0 && y <= l && x+y > best {
				best, endX, endY = x+y, x, y
			}
		}
	}
	x, y := endX, endY
	for d := endD; d > 0; d-- {
		prev := trace[d-1]
		at := func(k int) int32 { return prev[k+d] }
		k := x - y
		var px, py, sx, sy int // punto anterior y comienzo de la diagonal
		if k == -d || (k != d && at(k-1) < at(k+1)) {
			px = int(at(k + 1))
			py = px - (k + 1)
			sx, sy = px, py+1
		} else {
			px = int(at(k - 1))
			py = px - (k - 1)
			sx, sy = px+1, py
		}
		for x > sx && y > sy {
			x--
			y--
			m[ao+x] = bo + y
		}
		x, y = px, py
	}
	for x > 0 && y > 0 {
		x--
		y--
		m[ao+x] = bo + y
	}
	if endX < n || endY < l {
		matchInto(a[endX:], b[endY:], ao+endX, bo+endY, m)
	}
}

// ---------- filas de la comparación ----------

type diffKind uint8

const (
	dSame diffKind = iota
	dDel
	dAdd
	dChg
)

// diffRow es una fila alineada: la línea l de la izquierda frente a la r de la derecha (-1 = nada).
type diffRow struct {
	l, r   int
	kind   diffKind
	lm, rm []bool // runas que cambiaron dentro de la línea; nil = no se pudo afinar
}

func lineKey(l []rune, ignoreWS bool) string {
	if ignoreWS {
		return strings.Join(strings.Fields(string(l)), " ")
	}
	return string(l)
}

func computeDiff(L, R [][]rune, ignoreWS bool) []diffRow {
	ids := map[string]int{}
	intern := func(ls [][]rune) []int {
		out := make([]int, len(ls))
		for i, l := range ls {
			k := lineKey(l, ignoreWS)
			id, ok := ids[k]
			if !ok {
				id = len(ids)
				ids[k] = id
			}
			out[i] = id
		}
		return out
	}
	il, ir := intern(L), intern(R)
	m := diffMatch(il, ir)
	rows := make([]diffRow, 0, max(len(L), len(R)))
	i, j := 0, 0
	for i < len(L) || j < len(R) {
		i0 := i
		for i < len(L) && m[i] < 0 {
			i++
		}
		end := len(R)
		if i < len(L) {
			end = m[i]
		}
		rows = alignBlock(L[i0:i], R[j:end], i0, j, ignoreWS, rows)
		j = end
		if i < len(L) {
			rows = append(rows, diffRow{l: i, r: j})
			i++
			j++
		}
	}
	return rows
}

// alignBlock ordena un bloque de cambios: cada línea modificada queda frente a su versión anterior.
func alignBlock(L, R [][]rune, l0, r0 int, ignoreWS bool, rows []diffRow) []diffRow {
	i, j := 0, 0
	gap := func(ie, je int) {
		for i < ie || j < je {
			row := diffRow{l: -1, r: -1, kind: dChg}
			if i < ie {
				row.l = l0 + i
				i++
			} else {
				row.kind = dAdd
			}
			if j < je {
				row.r = r0 + j
				j++
			} else {
				row.kind = dDel
			}
			rows = append(rows, row)
		}
	}
	for _, p := range similarPairs(L, R) {
		gap(p[0], p[1])
		lm, rm := wordDiff(L[i], R[j], ignoreWS)
		rows = append(rows, diffRow{l: l0 + i, r: r0 + j, kind: dChg, lm: lm, rm: rm})
		i++
		j++
	}
	gap(len(L), len(R))
	return rows
}

// similarPairs busca, respetando el orden, las parejas de líneas más parecidas.
func similarPairs(L, R [][]rune) [][2]int {
	n, m := len(L), len(R)
	if n == 0 || m == 0 || n*m > diffBlockMax {
		return nil
	}
	bags := func(ls [][]rune) []map[string]int {
		out := make([]map[string]int, len(ls))
		for i, l := range ls {
			out[i] = map[string]int{}
			for _, t := range lineTokens(l) {
				if !t.space {
					out[i][string(l[t.s:t.e])]++
				}
			}
		}
		return out
	}
	bl, br := bags(L), bags(R)
	sim := make([][]float32, n)
	score := make([][]float32, n+1)
	for i := range score {
		score[i] = make([]float32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		sim[i] = make([]float32, m)
		for j := m - 1; j >= 0; j-- {
			sim[i][j] = bagSimilarity(bl[i], br[j])
			best := max(score[i+1][j], score[i][j+1])
			if sim[i][j] >= diffSimilar {
				best = max(best, sim[i][j]+score[i+1][j+1])
			}
			score[i][j] = best
		}
	}
	var pairs [][2]int
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case sim[i][j] >= diffSimilar && score[i][j] == sim[i][j]+score[i+1][j+1]:
			pairs = append(pairs, [2]int{i, j})
			i++
			j++
		case score[i][j] == score[i+1][j]:
			i++
		default:
			j++
		}
	}
	return pairs
}

func bagSimilarity(a, b map[string]int) float32 {
	na, nb, common := 0, 0, 0
	for t, c := range a {
		na += c
		common += min(c, b[t])
	}
	for _, c := range b {
		nb += c
	}
	if na+nb == 0 {
		return 0
	}
	return 2 * float32(common) / float32(na+nb)
}

type diffToken struct {
	s, e  int
	space bool
}

// lineTokens parte una línea en palabras, tramos de espacios y signos sueltos.
func lineTokens(l []rune) []diffToken {
	var out []diffToken
	for i := 0; i < len(l); {
		j := i + 1
		space := unicode.IsSpace(l[i])
		switch {
		case space:
			for j < len(l) && unicode.IsSpace(l[j]) {
				j++
			}
		case isIdent(l[i]):
			for j < len(l) && isIdent(l[j]) {
				j++
			}
		}
		out = append(out, diffToken{i, j, space})
		i = j
	}
	return out
}

// wordDiff marca qué partes de dos líneas parecidas cambiaron.
func wordDiff(l, r []rune, ignoreWS bool) ([]bool, []bool) {
	tl, tr := lineTokens(l), lineTokens(r)
	ids := map[string]int{}
	intern := func(line []rune, ts []diffToken) []int {
		out := make([]int, len(ts))
		for i, t := range ts {
			k := string(line[t.s:t.e])
			id, ok := ids[k]
			if !ok {
				id = len(ids)
				ids[k] = id
			}
			out[i] = id
		}
		return out
	}
	il, ir := intern(l, tl), intern(r, tr)
	okL, okR := make([]bool, len(tl)), make([]bool, len(tr))
	for i, j := range diffMatch(il, ir) {
		if j >= 0 {
			okL[i], okR[j] = true, true
		}
	}
	return tokenMask(len(l), tl, okL, ignoreWS), tokenMask(len(r), tr, okR, ignoreWS)
}

func tokenMask(n int, ts []diffToken, ok []bool, ignoreWS bool) []bool {
	mask := make([]bool, n)
	for i, t := range ts {
		changed := !ok[i]
		// un espacio entre dos palabras cambiadas se pinta con ellas: se lee mejor
		if t.space && i > 0 && i+1 < len(ts) && !ok[i-1] && !ok[i+1] {
			changed = true
		}
		if changed && !(ignoreWS && t.space) {
			for k := t.s; k < t.e; k++ {
				mask[k] = true
			}
		}
	}
	return mask
}

// ---------- vista ----------

// diffSide es uno de los dos códigos que se comparan.
type diffSide struct {
	label  string
	lines  [][]rune
	lang   *Lang
	crlf   bool
	states []bool // comentario de bloque abierto al empezar cada línea
}

// diffLine es lo que ocupa un renglón de pantalla.
type diffLine struct {
	row  int
	part int8 // en vista unificada: 0 fila entera, 1 sólo izquierda, 2 sólo derecha
	fold int  // > 0: renglón que resume esa cantidad de líneas iguales
}

type DiffView struct {
	L, R             diffSide
	rows             []diffRow
	view             []diffLine
	hunks            []int // renglones donde empieza cada cambio
	added, removed   int
	ignoreWS         bool
	unified, uni     bool // uni: lo que se ve de verdad (una ventana angosta fuerza la unificada)
	fold             bool
	cur, off, hoff   int
	page, maxV       int
	barX, barY, barH int
}

func newDiffView(l, r diffSide) *DiffView {
	d := &DiffView{L: l, R: r, fold: true, page: 1}
	d.L.states = blockStatesOf(l.lines, l.lang)
	d.R.states = blockStatesOf(r.lines, r.lang)
	for _, s := range []*diffSide{&d.L, &d.R} {
		for _, line := range s.lines {
			d.maxV = max(d.maxV, vcol(line, len(line)))
		}
	}
	d.recompute()
	return d
}

func blockStatesOf(lines [][]rune, l *Lang) []bool {
	st := make([]bool, len(lines))
	if l != nil && l.BS != "" {
		in := false
		for i, line := range lines {
			st[i] = in
			in = highlight(line, l, in, nil, nil)
		}
	}
	return st
}

func (d *DiffView) recompute() {
	d.rows = computeDiff(d.L.lines, d.R.lines, d.ignoreWS)
	d.added, d.removed = 0, 0
	for _, r := range d.rows {
		if r.kind == dAdd || r.kind == dChg {
			d.added++
		}
		if r.kind == dDel || r.kind == dChg {
			d.removed++
		}
	}
	d.view = nil
	d.off = 0
	d.rebuild()
	d.jump(0)
}

// rebuild arma los renglones visibles (pliegues, vista unificada) sin perder el lugar.
func (d *DiffView) rebuild() {
	anchor := -1
	if d.off < len(d.view) {
		anchor = d.view[d.off].row
	}
	n := len(d.rows)
	keep := make([]bool, n)
	for i, r := range d.rows {
		if r.kind != dSame {
			for k := max(0, i-diffCtx); k <= min(n-1, i+diffCtx); k++ {
				keep[k] = true
			}
		}
	}
	d.view, d.hunks = d.view[:0], nil
	inHunk := false
	for i := 0; i < n; {
		if d.fold && !keep[i] {
			j := i
			for j < n && !keep[j] {
				j++
			}
			if j-i > 1 {
				d.view = append(d.view, diffLine{row: i, fold: j - i})
				i = j
				inHunk = false
				continue
			}
		}
		k := d.rows[i].kind
		if k != dSame && !inHunk {
			d.hunks = append(d.hunks, len(d.view))
		}
		inHunk = k != dSame
		if d.uni && k == dChg {
			d.view = append(d.view, diffLine{row: i, part: 1}, diffLine{row: i, part: 2})
		} else {
			d.view = append(d.view, diffLine{row: i})
		}
		i++
	}
	if anchor >= 0 {
		d.off = max(0, sort.Search(len(d.view), func(i int) bool { return d.view[i].row > anchor })-1)
	}
	d.clamp()
	d.syncCur()
}

func (d *DiffView) clamp() {
	d.off = max(0, min(d.off, len(d.view)-d.page))
	d.hoff = max(0, min(d.hoff, d.maxV))
}

// syncCur recalcula en qué cambio estamos después de desplazarse a mano.
func (d *DiffView) syncCur() {
	i := sort.SearchInts(d.hunks, d.off+diffCtx+1) - 1
	d.cur = max(0, i)
}

func (d *DiffView) scroll(n int) {
	d.off += n
	d.clamp()
	d.syncCur()
}

func (d *DiffView) jump(i int) {
	if len(d.hunks) == 0 {
		return
	}
	d.cur = max(0, min(i, len(d.hunks)-1))
	d.off = d.hunks[d.cur] - diffCtx
	d.clamp()
}

func (d *DiffView) Key(a *App, ev *tcell.EventKey) {
	shift := ev.Modifiers()&tcell.ModShift != 0
	switch ev.Key() {
	case tcell.KeyEsc:
		a.dlg = nil
	case tcell.KeyUp:
		d.scroll(-1)
	case tcell.KeyDown:
		d.scroll(1)
	case tcell.KeyPgUp:
		d.scroll(-max(1, d.page-1))
	case tcell.KeyPgDn:
		d.scroll(max(1, d.page-1))
	case tcell.KeyHome:
		d.scroll(-len(d.view))
	case tcell.KeyEnd:
		d.scroll(len(d.view))
	case tcell.KeyLeft:
		d.hoff -= 8
		d.clamp()
	case tcell.KeyRight:
		d.hoff += 8
		d.clamp()
	case tcell.KeyTab:
		d.unified = !d.unified
	case tcell.KeyF3:
		if shift {
			d.jump(d.cur - 1)
		} else {
			d.jump(d.cur + 1)
		}
	case tcell.KeyF9:
		a.nextTheme()
	case tcell.KeyRune:
		switch unicode.ToLower(ev.Rune()) {
		case 'q':
			a.dlg = nil
		case 'n':
			d.jump(d.cur + 1)
		case 'p':
			d.jump(d.cur - 1)
		case 'z':
			d.fold = !d.fold
			d.rebuild()
		case 'w':
			d.ignoreWS = !d.ignoreWS
			d.recompute()
		}
	}
}

func (d *DiffView) Wheel(n int) { d.scroll(n) }

// Click en la barra de la derecha: salta a ese punto de la comparación.
func (d *DiffView) Click(x, y int) {
	if x == d.barX && y >= d.barY && y < d.barY+d.barH {
		d.off = (y-d.barY)*len(d.view)/d.barH - d.page/2
		d.clamp()
		d.syncCur()
	}
}

func (d *DiffView) Draw(a *App) {
	T := a.T()
	w, h := a.s.Size()
	if uni := d.unified || w < 80; uni != d.uni {
		d.uni = uni
		d.rebuild()
	}
	a.fill(0, 0, w, h, ' ', T.Normal)
	d.page = max(1, h-4)
	d.clamp()
	d.drawHeader(a, w)
	gw := len(strconv.Itoa(max(len(d.L.lines), len(d.R.lines))))
	top := 2
	if d.uni {
		r := rect{0, 1, w, h - 2}
		a.frame(r, true, T.Frame)
		d.boxTitle(a, r, d.L.label+" → "+d.R.label)
		for i := 0; i < d.page && d.off+i < len(d.view); i++ {
			d.drawUnified(a, 1, top+i, w-2, gw, d.view[d.off+i])
		}
	} else {
		lw := w / 2
		rl, rr := rect{0, 1, lw, h - 2}, rect{lw, 1, w - lw, h - 2}
		a.frame(rl, true, T.Frame)
		a.frame(rr, true, T.Frame)
		d.boxTitle(a, rl, d.L.label)
		d.boxTitle(a, rr, d.R.label)
		for i := 0; i < d.page && d.off+i < len(d.view); i++ {
			v := d.view[d.off+i]
			if v.fold > 0 {
				d.drawFold(a, 1, top+i, lw-2, v.fold)
				d.drawFold(a, lw+1, top+i, w-lw-2, v.fold)
				continue
			}
			row := d.rows[v.row]
			d.drawCell(a, 1, top+i, lw-2, gw, &d.L, row.l, row, true)
			d.drawCell(a, lw+1, top+i, w-lw-2, gw, &d.R, row.r, row, false)
		}
	}
	if len(d.hunks) > 0 {
		if y := top + d.hunks[d.cur] - d.off; y >= top && y < top+d.page {
			a.s.SetContent(0, y, '▸', nil, T.Title)
		}
	}
	d.barX, d.barY, d.barH = w-1, top, d.page
	d.drawMinimap(a)
	left := " n/p cambio · Tab vista · z plegar · w espacios · ←→ desplazar · Esc cerrar"
	a.fill(0, h-1, w, 1, ' ', T.Status)
	a.putw(0, h-1, left, T.Status, w)
}

func (d *DiffView) boxTitle(a *App, r rect, s string) {
	T := a.T()
	if T.Upper {
		s = strings.ToUpper(s)
	}
	s = " " + s + " "
	tw := min(strW(s), r.w-4)
	a.putw(r.x+(r.w-tw)/2, r.y, s, T.Title, r.w-4)
}

func (d *DiffView) drawHeader(a *App, w int) {
	T := a.T()
	a.fill(0, 0, w, 1, ' ', T.MenuBar)
	x := a.put(1, 0, " Comparar ", T.MenuBar.Bold(true))
	if len(d.hunks) == 0 {
		msg := "sin diferencias"
		if d.ignoreWS {
			msg += " (ignorando espacios)"
		}
		a.put(x+1, 0, msg, T.MenuBar)
		return
	}
	x = a.put(x+1, 0, fmt.Sprintf(" +%d ", d.added), T.DiffAddHi.StrikeThrough(false))
	x = a.put(x+1, 0, fmt.Sprintf(" −%d ", d.removed), T.DiffDelHi.StrikeThrough(false))
	if d.ignoreWS {
		a.put(x+2, 0, "ignorando espacios", T.MenuBar)
	}
	right := fmt.Sprintf("cambio %d de %d ", d.cur+1, len(d.hunks))
	a.put(w-strW(right)-1, 0, right, T.MenuBar)
}

func (d *DiffView) drawFold(a *App, x, y, w, n int) {
	s := fmt.Sprintf("··· %d líneas iguales ···", n)
	a.putw(x+max(0, (w-strW(s))/2), y, s, a.T().Comment, w)
}

// drawCell dibuja una mitad de la vista lado a lado.
func (d *DiffView) drawCell(a *App, x, y, w, gw int, s *diffSide, ly int, row diffRow, left bool) {
	T := a.T()
	if ly < 0 {
		a.fill(x, y, w, 1, '░', T.DiffFill)
		return
	}
	base, hi, mark, mask := T.Normal, T.DiffAddHi, ' ', row.rm
	if left {
		hi, mask = T.DiffDelHi, row.lm
	}
	if row.kind != dSame {
		if left {
			base, mark = T.DiffDel, '-'
		} else {
			base, mark = T.DiffAdd, '+'
		}
	}
	a.fill(x, y, w, 1, ' ', base)
	gs := base
	if row.kind == dSame {
		gs = T.Gutter
	}
	a.put(x, y, fmt.Sprintf("%*d ", gw, ly+1), gs)
	a.s.SetContent(x+gw+1, y, mark, nil, gs.Bold(true))
	d.drawText(a, x+gw+3, y, w-gw-3, s, ly, base, hi, mask)
}

func (d *DiffView) drawUnified(a *App, x, y, w, gw int, v diffLine) {
	T := a.T()
	if v.fold > 0 {
		d.drawFold(a, x, y, w, v.fold)
		return
	}
	row := d.rows[v.row]
	l, r := row.l, row.r
	switch v.part {
	case 1:
		r = -1
	case 2:
		l = -1
	}
	num := func(n int) string {
		if n < 0 {
			return strings.Repeat(" ", gw)
		}
		return fmt.Sprintf("%*d", gw, n+1)
	}
	gutter := num(l) + " " + num(r) + " "
	tx := x + 2*gw + 4
	switch {
	case row.kind == dSame:
		a.put(x, y, gutter, T.Gutter)
		d.drawText(a, tx, y, x+w-tx, &d.L, l, T.Normal, T.Normal, nil)
	case r < 0:
		a.fill(x, y, w, 1, ' ', T.DiffDel)
		a.put(x, y, gutter+"-", T.DiffDel)
		d.drawText(a, tx, y, x+w-tx, &d.L, l, T.DiffDel, T.DiffDelHi, row.lm)
	default:
		a.fill(x, y, w, 1, ' ', T.DiffAdd)
		a.put(x, y, gutter+"+", T.DiffAdd)
		d.drawText(a, tx, y, x+w-tx, &d.R, r, T.DiffAdd, T.DiffAddHi, row.rm)
	}
}

// drawText pinta una línea con su sintaxis sobre el fondo del cambio.
func (d *DiffView) drawText(a *App, x, y, w int, s *diffSide, ly int, base, hi tcell.Style, mask []bool) {
	T := a.T()
	line := s.lines[ly]
	styles := make([]tcell.Style, len(line))
	highlight(line, s.lang, s.states[ly], T, styles)
	_, bg, _ := base.Decompose()
	vc := 0
	for i, ch := range line {
		cw := runeW(ch)
		if ch == '\t' {
			cw = tabW - vc%tabW
		}
		st := styles[i].Background(bg)
		if mask != nil && mask[i] {
			st = hi
		}
		sx := vc - d.hoff
		if ch == '\t' {
			for k := 0; k < cw; k++ {
				if sx+k >= 0 && sx+k < w {
					a.s.SetContent(x+sx+k, y, ' ', nil, st)
				}
			}
		} else if sx >= 0 && sx+cw <= w {
			a.s.SetContent(x+sx, y, ch, nil, st)
		}
		vc += cw
		if vc-d.hoff >= w {
			break
		}
	}
}

// drawMinimap convierte la barra de desplazamiento en un mapa de todos los cambios.
func (d *DiffView) drawMinimap(a *App) {
	T := a.T()
	n := len(d.view)
	if n == 0 || d.barH < 1 {
		return
	}
	del, add := make([]bool, d.barH), make([]bool, d.barH)
	for i, v := range d.view {
		if v.fold > 0 {
			continue
		}
		c := i * d.barH / n
		k := d.rows[v.row].kind
		del[c] = del[c] || k == dDel || (k == dChg && v.part != 2)
		add[c] = add[c] || k == dAdd || (k == dChg && v.part != 1)
	}
	_, delBg, _ := T.DiffDelHi.Decompose()
	_, addBg, _ := T.DiffAddHi.Decompose()
	first := d.off * d.barH / n
	last := (min(d.off+d.page, n) - 1) * d.barH / n
	for i := 0; i < d.barH; i++ {
		y := d.barY + i
		switch {
		case del[i] && add[i]:
			a.s.SetContent(d.barX, y, '▌', nil, tcell.StyleDefault.Foreground(delBg).Background(addBg))
		case del[i]:
			a.s.SetContent(d.barX, y, ' ', nil, tcell.StyleDefault.Background(delBg))
		case add[i]:
			a.s.SetContent(d.barX, y, ' ', nil, tcell.StyleDefault.Background(addBg))
		case i >= first && i <= last:
			a.s.SetContent(d.barX, y, '▒', nil, T.Thumb)
		default:
			a.s.SetContent(d.barX, y, '░', nil, T.Scroll)
		}
	}
}

// ---------- abrir una comparación ----------

func (a *App) bufferSide() diffSide {
	b := a.buf
	label := "Sin título"
	if b.Path != "" {
		label = filepath.Base(b.Path)
	}
	if b.Dirty {
		label += " (sin guardar)"
	}
	return diffSide{label: label, lines: b.Lines, lang: langFor(b.Path), crlf: b.CRLF}
}

func loadSide(p string) (diffSide, error) {
	fi, err := os.Stat(p)
	if os.IsNotExist(err) {
		return diffSide{}, fmt.Errorf("no existe %s", p)
	}
	if err != nil {
		return diffSide{}, err
	}
	if fi.IsDir() {
		return diffSide{}, fmt.Errorf("%s es una carpeta", p)
	}
	b, err := LoadBuffer(p)
	if err != nil {
		return diffSide{}, err
	}
	return diffSide{label: filepath.Base(p), lines: b.Lines, lang: langFor(p), crlf: b.CRLF}, nil
}

func (a *App) showDiff(l, r diffSide) {
	d := newDiffView(l, r)
	if len(d.hunks) > 0 {
		a.dlg = d
		return
	}
	lines := []string{"No hay diferencias entre", l.label + " y " + r.label + "."}
	if l.crlf != r.crlf {
		lines = append(lines, "Sólo cambian los finales de línea (CRLF / LF).")
	}
	a.msgBox("Comparar", lines...)
}

// compareWith compara lo que hay en el editor, con sus cambios sin guardar, contra un archivo.
func (a *App) compareWith(p string) {
	r, err := loadSide(p)
	if err != nil {
		a.msgBox("Comparar", err.Error())
		return
	}
	a.showDiff(a.bufferSide(), r)
}

func (a *App) compareFiles(p, q string) {
	l, err := loadSide(p)
	if err == nil {
		var r diffSide
		if r, err = loadSide(q); err == nil {
			if l.label == r.label {
				l.label, r.label = prettyPath(p), prettyPath(q)
			}
			a.showDiff(l, r)
			return
		}
	}
	a.msgBox("Comparar", err.Error())
}

func (a *App) compareOther() {
	a.dlg = NewPickDialog(a, "Comparar con...", (*App).compareWith)
}

func (a *App) compareTwo() {
	a.dlg = NewPickDialog(a, "Primer archivo", func(a *App, p string) {
		a.dlg = NewPickDialog(a, "Segundo archivo", func(a *App, q string) { a.compareFiles(p, q) })
	})
}

// compareSaved muestra qué cambió en el editor desde la última vez que se guardó.
func (a *App) compareSaved() {
	if a.buf.Path == "" {
		a.flash("Este archivo todavía no está guardado")
		return
	}
	l, err := loadSide(a.buf.Path)
	if err != nil {
		a.msgBox("Comparar", err.Error())
		return
	}
	l.label += " (en disco)"
	a.showDiff(l, a.bufferSide())
}

func (a *App) compareClipboard() {
	t := a.getClip()
	if t == "" {
		a.flash("El portapapeles está vacío")
		return
	}
	b := NewBuffer("")
	b.setText(t)
	a.showDiff(a.bufferSide(), diffSide{label: "Portapapeles", lines: b.Lines, lang: langFor(a.buf.Path), crlf: b.CRLF})
}
