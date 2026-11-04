package main

import (
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v2"
)

type MenuItem struct {
	Label, Key string
	Do         func(a *App)
	Check      func(a *App) bool
}

type Menu struct {
	Title string
	Hot   int // índice de la letra rápida (Alt+letra)
	Items []MenuItem
}

func buildMenus(themes []Theme) []Menu {
	sep := MenuItem{}
	ver := Menu{Title: "Ver", Items: []MenuItem{
		{Label: "Números de línea", Do: func(a *App) { a.lineNums = !a.lineNums; a.saveConfig() }, Check: func(a *App) bool { return a.lineNums }},
		{Label: "Árbol de archivos", Key: "Ctrl+B", Do: (*App).toggleTree, Check: func(a *App) bool { return a.showTree }},
		{Label: "Terminal", Do: (*App).toggleTerm, Check: func(a *App) bool { return a.showTerm }},
		{Label: "Agente (columna derecha)", Key: "F4", Do: (*App).togglePane, Check: func(a *App) bool { return a.pane }},
		{Label: "Siguiente panel", Key: "F6", Do: func(a *App) { a.cycleFocus(false) }},
		{Label: "Tema siguiente", Key: "F9", Do: (*App).nextTheme},
		sep,
	}}
	for i, t := range themes {
		i := i
		ver.Items = append(ver.Items, MenuItem{Label: "Tema " + t.Name, Do: func(a *App) { a.setTheme(i) }, Check: func(a *App) bool { return a.ti == i }})
	}
	return []Menu{
		{Title: "Archivo", Hot: 0, Items: []MenuItem{
			{Label: "Nuevo", Key: "Ctrl+N", Do: (*App).newFile},
			{Label: "Abrir...", Key: "Ctrl+O", Do: (*App).openFile},
			{Label: "Carpeta de trabajo...", Do: (*App).chooseFolder},
			{Label: "Guardar", Key: "Ctrl+S", Do: func(a *App) { a.save(nil) }},
			{Label: "Guardar como...", Do: func(a *App) { a.saveAs(nil) }},
			{Label: "Recargar desde disco", Do: (*App).reload},
			sep,
			{Label: "Salir", Key: "Ctrl+Q", Do: (*App).tryQuit},
		}},
		{Title: "Editar", Hot: 0, Items: []MenuItem{
			{Label: "Deshacer", Key: "Ctrl+Z", Do: func(a *App) { a.buf.Undo() }},
			{Label: "Rehacer", Key: "Ctrl+Y", Do: func(a *App) { a.buf.Redo() }},
			sep,
			{Label: "Cortar", Key: "Ctrl+X", Do: (*App).cut},
			{Label: "Copiar", Key: "Ctrl+C", Do: (*App).copy},
			{Label: "Pegar", Key: "Ctrl+V", Do: (*App).paste},
			{Label: "Seleccionar todo", Key: "Ctrl+A", Do: func(a *App) { a.buf.SelectAll() }},
			sep,
			{Label: "Duplicar línea", Key: "Ctrl+D", Do: func(a *App) { a.buf.DuplicateLine() }},
			{Label: "Comentar / descomentar", Key: "Ctrl+/", Do: (*App).toggleComment},
			{Label: "Indentar", Key: "Tab", Do: func(a *App) { a.buf.Indent() }},
			{Label: "Desindentar", Key: "Shift+Tab", Do: func(a *App) { a.buf.Unindent() }},
		}},
		{Title: "Buscar", Hot: 1, Items: []MenuItem{
			{Label: "Buscar...", Key: "Ctrl+F", Do: (*App).findDialog},
			{Label: "Buscar siguiente", Key: "F3", Do: (*App).findNext},
			{Label: "Reemplazar todo...", Key: "Ctrl+R", Do: (*App).replaceDialog},
			{Label: "Ir a línea...", Key: "Ctrl+G", Do: (*App).gotoDialog},
		}},
		ver,
		{Title: "Comparar", Hot: 0, Items: []MenuItem{
			{Label: "Archivo actual con otro...", Do: (*App).compareOther},
			{Label: "Dos archivos...", Do: (*App).compareTwo},
			{Label: "Versión guardada", Do: (*App).compareSaved},
			{Label: "Portapapeles", Do: (*App).compareClipboard},
		}},
		{Title: "Ejecutar", Hot: 1, Items: []MenuItem{
			{Label: "Ejecutar archivo", Key: "F12", Do: (*App).runFile},
			{Label: "Ir a la terminal", Key: "F7", Do: (*App).toggleTermFocus},
			{Label: "Reiniciar terminal", Do: func(a *App) {
				a.term.close()
				a.term = nil
				a.showTerm = true
				a.focus = focTerm
			}},
		}},
		{Title: "IA", Hot: 0, Items: []MenuItem{
			{Label: "Pedirle algo", Key: "Ctrl+K", Do: (*App).askAI},
			{Label: "Explicar selección", Do: func(a *App) { a.quickAI("Explica brevemente qué hace este código.") }},
			{Label: "Buscar errores", Do: func(a *App) {
				a.quickAI("Revisa este código y señala errores o problemas concretos, del más grave al menos grave. Sé breve.")
			}},
			{Label: "Aplicar código de la respuesta", Key: "F5", Do: (*App).applyAI},
			{Label: "Copiar respuesta", Do: (*App).copyAnswer},
			{Label: "Cancelar consulta", Key: "F8", Do: (*App).cancelAI},
			sep,
			{Label: "Usar Claude", Key: "F2", Do: func(a *App) { a.engine = "claude"; a.saveConfig() }, Check: func(a *App) bool { return a.engine == "claude" }},
			{Label: "Usar Codex", Key: "F2", Do: func(a *App) { a.engine = "codex"; a.saveConfig() }, Check: func(a *App) bool { return a.engine == "codex" }},
			{Label: "Modo agente (edita archivos)", Key: "Tab", Do: func(a *App) { a.agent = !a.agent; a.saveConfig() }, Check: func(a *App) bool { return a.agent }},
			sep,
			{Label: "Nueva conversación", Do: (*App).newConversation},
		}},
		{Title: "Ayuda", Hot: 1, Items: []MenuItem{
			{Label: "Atajos de teclado", Key: "F1", Do: (*App).showHelp},
			{Label: "Acerca de tecla", Do: (*App).showAbout},
		}},
	}
}

func (a *App) openMenu(i int) {
	a.menuOpen = true
	a.mi = i
	a.ii = 0
}

func (a *App) menuAt(x int) int {
	for i, mx := range a.menuX {
		if x >= mx && x < mx+len([]rune(a.menus[i].Title))+2 {
			return i
		}
	}
	return -1
}

func (a *App) menuHot(r rune) bool {
	r = unicode.ToLower(r)
	for i, m := range a.menus {
		if unicode.ToLower([]rune(m.Title)[m.Hot]) == r {
			a.openMenu(i)
			return true
		}
	}
	return false
}

func (a *App) execItem(mi, ii int) {
	it := a.menus[mi].Items[ii]
	if it.Label == "" || it.Do == nil {
		return
	}
	a.menuOpen = false
	it.Do(a)
	a.follow = true
}

func (a *App) menuKey(ev *tcell.EventKey) {
	n := len(a.menus)
	items := a.menus[a.mi].Items
	move := func(d int) {
		for k := 0; k < len(items); k++ {
			a.ii = (a.ii + d + len(items)) % len(items)
			if items[a.ii].Label != "" {
				return
			}
		}
	}
	switch ev.Key() {
	case tcell.KeyEsc, tcell.KeyF10:
		a.menuOpen = false
	case tcell.KeyLeft:
		a.openMenu((a.mi - 1 + n) % n)
	case tcell.KeyRight, tcell.KeyTab:
		a.openMenu((a.mi + 1) % n)
	case tcell.KeyUp:
		move(-1)
	case tcell.KeyDown:
		move(1)
	case tcell.KeyHome:
		a.ii = 0
	case tcell.KeyEnd:
		a.ii = len(items) - 1
	case tcell.KeyEnter:
		a.execItem(a.mi, a.ii)
	case tcell.KeyRune:
		r := unicode.ToLower(ev.Rune())
		if ev.Modifiers()&tcell.ModAlt != 0 {
			a.menuHot(r)
			return
		}
		for i, it := range items {
			if it.Label != "" && unicode.ToLower([]rune(it.Label)[0]) == r {
				a.execItem(a.mi, i)
				return
			}
		}
	}
}

func (a *App) drawDropdown(w int) {
	T := a.T()
	m := a.menus[a.mi]
	lw, kw := 0, 0
	for _, it := range m.Items {
		lw = max(lw, strW(it.Label))
		kw = max(kw, strW(it.Key))
	}
	width := lw + kw + 8
	x := a.menuX[a.mi] - 1
	if x+width+2 > w {
		x = w - width - 2
	}
	x = max(0, x)
	h := len(m.Items) + 2
	a.ddX, a.ddW = x, width
	a.fill(x, 1, width, h, ' ', T.MenuBar)
	a.box(x, 1, width, h, Single, T.MenuBar)
	a.shadow(x, 1, width, h)
	for i, it := range m.Items {
		y := 2 + i
		if it.Label == "" {
			a.s.SetContent(x, y, Single.LT, nil, T.MenuBar)
			a.s.SetContent(x+width-1, y, Single.RT, nil, T.MenuBar)
			a.fill(x+1, y, width-2, 1, Single.H, T.MenuBar)
			continue
		}
		base, hot := T.MenuBar, T.MenuHot
		if i == a.ii {
			base, hot = T.MenuSel, T.MenuSelHot
		}
		a.fill(x+1, y, width-2, 1, ' ', base)
		if it.Check != nil && it.Check(a) {
			a.put(x+2, y, "✓", base)
		}
		lbl := []rune(it.Label)
		a.s.SetContent(x+4, y, lbl[0], nil, hot)
		a.put(x+5, y, string(lbl[1:]), base)
		if it.Key != "" {
			a.put(x+width-2-strW(it.Key), y, it.Key, base)
		}
	}
}

var helpText = strings.Split(`
 PANELES
 F6 / Shift+F6   Pasar al panel siguiente / anterior (o clic con el ratón)
 Ctrl+B          Mostrar / ocultar el árbol de archivos
 F7              Ir a la terminal (otra vez F7 vuelve al editor)
 F4              Mostrar / ocultar el agente      Ctrl+K   Escribirle al agente
 F12 / Ctrl+F9   Ejecutar el archivo actual en la terminal

 ARCHIVOS (columna izquierda)
 Enter    Abrir archivo / plegar carpeta     r          Usar carpeta como raíz
 ← →      Plegar / desplegar                 Retroceso  Subir a la carpeta padre
 c        Comparar el archivo elegido con el que está en el editor

 ARCHIVO                          EDICIÓN
 Ctrl+N   Nuevo                   Ctrl+Z / Ctrl+Y   Deshacer / Rehacer
 Ctrl+O   Abrir                   Ctrl+X / C / V    Cortar / Copiar / Pegar
 Ctrl+S   Guardar                 Ctrl+A            Seleccionar todo
 Ctrl+Q   Salir                   Ctrl+D            Duplicar línea
                                  Ctrl+/            Comentar líneas
 BUSCAR                           Tab / Shift+Tab   Indentar / desindentar
 Ctrl+F   Buscar                  Shift+flechas     Seleccionar
 F3       Buscar siguiente        Alt+←/→           Palabra anterior/siguiente
 Ctrl+R   Reemplazar todo         Doble clic        Seleccionar palabra
 Ctrl+G   Ir a línea

 AGENTE (Claude / Codex, columna derecha)
 Enter         Enviar el pedido. Si hay texto seleccionado, va como contexto.
 Tab           Modo Agente (edita los archivos de la carpeta) / Pregunta.
 F5 / Ctrl+E   Aplicar el bloque de código de la respuesta. Ctrl+Z deshace.
 F2 / Ctrl+T   Cambiar entre Claude y Codex       F8   Cancelar consulta
 ↑ ↓           Pedidos anteriores · PgUp/PgDn desplaza la conversación
 Con Claude la conversación continúa entre pedidos (IA › Nueva conversación).

 TERMINAL
 Es un shell de verdad: todas las teclas van al shell, salvo F1–F12.

 COMPARAR (menú Comparar, o tecla --diff a b)
 n / p    Cambio siguiente / anterior        z    Plegar / mostrar líneas iguales
 Tab      Lado a lado / unificada            w    Ignorar espacios
 ← →      Desplazar                          Esc  Cerrar

 VER
 F9       Cambiar tema: DOS · Turbo · Mac · Ámbar · Fósforo
 Esc/F10  Abrir el menú (también Alt+letra, o con el ratón)
`, "\n")

func (a *App) showHelp() {
	a.dlg = &TextDialog{Title: "Ayuda", Lines: helpText[1:]}
}

func (a *App) showAbout() {
	motores := []string{}
	for _, e := range []string{"claude", "codex"} {
		estado := "no encontrado"
		if a.have[e] {
			estado = "listo"
		}
		motores = append(motores, "   "+engineName(e)+": "+estado)
	}
	lines := []string{
		"",
		"   ╔╦╗╔═╗╔═╗╦  ╔═╗",
		"    ║ ║╣ ║  ║  ╠═╣",
		"    ╩ ╚═╝╚═╝╩═╝╩ ╩   versión 1.0",
		"",
		"   Un editor de texto a la vieja usanza,",
		"   con Claude y Codex adentro.",
		"",
	}
	lines = append(lines, motores...)
	lines = append(lines, "")
	a.dlg = &TextDialog{Title: "Acerca de tecla", Lines: lines}
}
