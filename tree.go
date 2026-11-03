package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
)

// Árbol de archivos de la columna izquierda.

type treeNode struct {
	name, path string
	dir        bool
	depth      int
}

func readEntries(dir string) []treeNode {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var dirs, files []treeNode
	for _, e := range ents {
		n := e.Name()
		if strings.HasPrefix(n, ".") {
			continue
		}
		p := filepath.Join(dir, n)
		isDir := e.IsDir()
		if e.Type()&os.ModeSymlink != 0 {
			if fi, err := os.Stat(p); err == nil {
				isDir = fi.IsDir()
			}
		}
		if isDir {
			dirs = append(dirs, treeNode{name: n, path: p, dir: true})
		} else {
			files = append(files, treeNode{name: n, path: p})
		}
	}
	less := func(l []treeNode) func(i, j int) bool {
		return func(i, j int) bool { return strings.ToLower(l[i].name) < strings.ToLower(l[j].name) }
	}
	sort.Slice(dirs, less(dirs))
	sort.Slice(files, less(files))
	return append(dirs, files...)
}

func (a *App) refreshTree() {
	var sel string
	if a.treeSel >= 0 && a.treeSel < len(a.tree) {
		sel = a.tree[a.treeSel].path
	}
	items := []treeNode{{name: "..", path: filepath.Dir(a.root), dir: true}}
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		for _, n := range readEntries(dir) {
			n.depth = depth
			items = append(items, n)
			if n.dir && a.treeOpen[n.path] && depth < 12 {
				walk(n.path, depth+1)
			}
		}
	}
	walk(a.root, 0)
	a.tree = items
	a.treeAt = time.Now()
	a.treeSel = max(0, min(a.treeSel, len(items)-1))
	if sel != "" {
		for i, n := range items {
			if n.path == sel && n.name != ".." {
				a.treeSel = i
				break
			}
		}
	}
}

// setRoot cambia la carpeta de trabajo: árbol, agente y (si está libre) la terminal.
func (a *App) setRoot(dir string) {
	abs, err := filepath.Abs(expandHome(dir))
	if err != nil {
		a.msgBox("Carpeta", err.Error())
		return
	}
	if fi, err := os.Stat(abs); err != nil || !fi.IsDir() {
		a.msgBox("Carpeta", "No es una carpeta: "+abs)
		return
	}
	if abs == a.root {
		return
	}
	a.root = abs
	a.treeSel, a.treeOff = 0, 0
	a.claudeSID = "" // las sesiones de Claude son por carpeta
	a.refreshTree()
	if a.term != nil && a.term.idle() {
		a.term.write([]byte("\x15cd " + shellQuote(abs) + "\r"))
	}
	a.flash("Carpeta: " + prettyPath(abs))
}

func prettyPath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && (p == home || strings.HasPrefix(p, home+"/")) {
		return "~" + p[len(home):]
	}
	return p
}

// relPath devuelve p relativo a la carpeta raíz si está adentro.
func (a *App) relPath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if rel, err := filepath.Rel(a.root, abs); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return abs
}

func (a *App) chooseFolder() {
	a.dlg = NewInput("Abrir carpeta", "Carpeta de trabajo:", prettyPath(a.root)+"/", func(a *App, p string) {
		if p = strings.TrimSpace(p); p != "" {
			a.setRoot(p)
		}
	})
}

func (a *App) treeActivate(i int, keepFocus bool) {
	if i < 0 || i >= len(a.tree) {
		return
	}
	n := a.tree[i]
	switch {
	case n.name == "..":
		a.setRoot(n.path)
	case n.dir:
		a.treeOpen[n.path] = !a.treeOpen[n.path]
		a.refreshTree()
	default:
		if abs, _ := filepath.Abs(a.buf.Path); abs == n.path {
			if !keepFocus {
				a.focus = focEditor
			}
			return
		}
		a.confirmDiscard(func() {
			a.openPath(n.path)
			if !keepFocus {
				a.focus = focEditor
			}
		})
	}
}

func (a *App) treeKey(ev *tcell.EventKey) {
	n := len(a.tree)
	if n == 0 {
		return
	}
	page := max(1, a.rTree.h-3)
	cur := a.tree[a.treeSel]
	switch ev.Key() {
	case tcell.KeyUp:
		a.treeSel = max(0, a.treeSel-1)
	case tcell.KeyDown:
		a.treeSel = min(n-1, a.treeSel+1)
	case tcell.KeyPgUp:
		a.treeSel = max(0, a.treeSel-page)
	case tcell.KeyPgDn:
		a.treeSel = min(n-1, a.treeSel+page)
	case tcell.KeyHome:
		a.treeSel = 0
	case tcell.KeyEnd:
		a.treeSel = n - 1
	case tcell.KeyEnter:
		a.treeActivate(a.treeSel, false)
	case tcell.KeyRight:
		if cur.dir && cur.name != ".." && !a.treeOpen[cur.path] {
			a.treeOpen[cur.path] = true
			a.refreshTree()
		} else if cur.dir && a.treeSel+1 < n {
			a.treeSel++
		}
	case tcell.KeyLeft:
		if cur.dir && a.treeOpen[cur.path] {
			a.treeOpen[cur.path] = false
			a.refreshTree()
			return
		}
		for i := a.treeSel - 1; i >= 0; i-- { // ir a la carpeta padre
			if a.tree[i].dir && a.tree[i].depth < cur.depth {
				a.treeSel = i
				return
			}
		}
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		a.setRoot(filepath.Dir(a.root))
	case tcell.KeyEsc:
		a.focus = focEditor
	case tcell.KeyRune:
		switch ev.Rune() {
		case 'r', 'R':
			if cur.dir {
				a.setRoot(cur.path)
			}
		case 'q':
			a.focus = focEditor
		}
	}
}

func (a *App) drawTree() {
	T := a.T()
	r := a.rTree
	focused := a.focus == focTree
	if time.Since(a.treeAt) > 2*time.Second {
		a.refreshTree()
	}
	a.fill(r.x, r.y, r.w, r.h, ' ', T.Normal)
	a.frame(r, focused, T.Frame)
	title := " " + filepath.Base(a.root) + " "
	if a.root == "/" {
		title = " / "
	}
	if T.Upper {
		title = strings.ToUpper(title)
	}
	ts := T.Frame
	if focused {
		ts = T.Title
	}
	tw := min(strW(title), r.w-4)
	a.putw(r.x+(r.w-tw)/2, r.y, title, ts, r.w-4)

	inner := r.h - 2
	if a.treeSel < a.treeOff {
		a.treeOff = a.treeSel
	}
	if a.treeSel >= a.treeOff+inner {
		a.treeOff = a.treeSel - inner + 1
	}
	a.treeOff = max(0, min(a.treeOff, len(a.tree)-inner))
	open := ""
	if a.buf.Path != "" {
		open, _ = filepath.Abs(a.buf.Path)
	}
	w := r.w - 2
	for i := 0; i < inner && a.treeOff+i < len(a.tree); i++ {
		idx := a.treeOff + i
		n := a.tree[idx]
		y := r.y + 1 + i
		icon := "  "
		if n.dir {
			icon = "▸ "
			if a.treeOpen[n.path] {
				icon = "▾ "
			}
		}
		label := strings.Repeat("  ", n.depth) + icon + n.name
		if n.dir && n.name != ".." {
			label += "/"
		}
		s := T.Normal
		if n.dir {
			s = T.Keyword
		}
		if n.path == open {
			s = T.String
		}
		if idx == a.treeSel {
			if focused {
				s = T.ListSel
			} else {
				s = T.Selection
			}
			a.fill(r.x+1, y, w, 1, ' ', s)
		}
		a.putw(r.x+1, y, label, s, w)
	}
}

func (a *App) treeMouse(y int) {
	r := a.rTree
	i := a.treeOff + y - r.y - 1
	if y <= r.y || y >= r.y+r.h-1 || i < 0 || i >= len(a.tree) {
		a.focus = focTree
		return
	}
	a.focus = focTree
	a.treeSel = i
	a.treeActivate(i, a.tree[i].dir)
}
