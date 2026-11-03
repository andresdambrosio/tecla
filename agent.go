package main

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
)

// Columna derecha: conversación con Claude / Codex y caja de entrada.

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// askAI lleva el foco a la caja de entrada del agente (Ctrl+K).
func (a *App) askAI() {
	if len(a.have) == 0 {
		a.msgBox("IA no disponible", "No encontré `claude` ni `codex` en el PATH.", "Instala alguno y vuelve a abrir tecla.")
		return
	}
	a.pane = true
	a.focus = focAgent
	a.paneFollow = true
}

func (a *App) newConversation() {
	if a.job != nil && a.job.Running() {
		a.flash("Espera a que termine (F8 cancela)")
		return
	}
	a.jobs, a.job = nil, nil
	a.claudeSID = ""
	a.paneFollow = true
	a.flash("Conversación nueva")
}

func (a *App) sendChat() {
	p := strings.TrimSpace(a.chat.String())
	if p == "" {
		return
	}
	if a.job != nil && a.job.Running() {
		a.flash("Ya hay una consulta en curso (F8 la cancela)")
		return
	}
	a.history = append(a.history, p)
	a.chatHi = len(a.history)
	a.chat.set("")
	a.startAI(p, a.agent)
}

func (a *App) agentKey(ev *tcell.EventKey) {
	inner := a.rAgent.h - 4
	switch ev.Key() {
	case tcell.KeyEsc:
		a.focus = focEditor
	case tcell.KeyEnter:
		a.sendChat()
	case tcell.KeyTab, tcell.KeyBacktab:
		a.agent = !a.agent
		a.saveConfig()
	case tcell.KeyUp:
		if a.chatHi > 0 {
			a.chatHi--
			a.chat.set(a.history[a.chatHi])
		}
	case tcell.KeyDown:
		if a.chatHi < len(a.history)-1 {
			a.chatHi++
			a.chat.set(a.history[a.chatHi])
		} else {
			a.chatHi = len(a.history)
			a.chat.set("")
		}
	case tcell.KeyPgUp:
		a.scrollPane(-max(1, inner-2))
	case tcell.KeyPgDn:
		a.scrollPane(max(1, inner-2))
	case tcell.KeyCtrlC:
		if a.chat.String() == "" {
			a.copyAnswer()
		} else {
			a.chat.set("")
		}
	default:
		a.chat.key(ev)
	}
}

func (a *App) scrollPane(d int) {
	a.paneFollow = false
	a.paneScroll = max(0, a.paneScroll+d)
}

// ---------- dibujo ----------

type pline struct {
	text string
	st   tcell.Style
	fill bool
}

func wrapText(s string, w int, hard bool) []string {
	if w < 4 {
		w = 4
	}
	r := []rune(s)
	if strW(s) <= w {
		return []string{s}
	}
	var out []string
	for len(r) > 0 {
		cut, width := 0, 0
		for cut < len(r) && width+runeW(r[cut]) <= w {
			width += runeW(r[cut])
			cut++
		}
		if cut >= len(r) {
			out = append(out, string(r))
			break
		}
		if !hard {
			for k := cut; k > cut/2; k-- {
				if r[k-1] == ' ' {
					cut = k
					break
				}
			}
		}
		out = append(out, string(r[:cut]))
		r = r[cut:]
	}
	return out
}

func (a *App) jobLines(j *AIJob, w int) []pline {
	T := a.T()
	var out []pline
	mode := "pregunta"
	if j.Agent {
		mode = "agente"
	}
	head := fmt.Sprintf("%s · %s · %s", j.Start.Format("15:04"), engineName(j.Engine), mode)
	out = append(out, pline{head, T.PaneDim, false})
	for _, wl := range wrapText("> "+j.Prompt, w, false) {
		out = append(out, pline{wl, T.PaneDim.Bold(true), false})
	}
	out = append(out, pline{"", T.Pane, false})
	text := j.Text()
	running := j.Running()
	if running && strings.TrimSpace(text) == "" {
		text = "Pensando..."
	}
	inFence := false
	for _, raw := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		raw = strings.ReplaceAll(raw, "\t", "    ")
		if strings.HasPrefix(strings.TrimSpace(raw), "```") {
			inFence = !inFence
			out = append(out, pline{strings.Repeat("┄", w), T.PaneDim, false})
			continue
		}
		s := T.Pane
		if inFence {
			s = T.PaneCode
		} else if running && j.Engine == "codex" {
			s = T.PaneDim // registro de progreso de codex
		}
		for _, wl := range wrapText(raw, w, inFence) {
			out = append(out, pline{wl, s, inFence})
		}
	}
	return out
}

func (a *App) paneLines(w int) []pline {
	T := a.T()
	if len(a.jobs) == 0 {
		intro := []string{
			"Asistente: " + engineName(a.engine),
			"",
			"Escribe abajo y Enter para enviar.",
			"",
			"Tab     Agente (edita archivos de la",
			"        carpeta) / Pregunta (sólo responde).",
			"F2      cambiar entre Claude y Codex.",
			"F5      aplicar el bloque de código de",
			"        la respuesta (Ctrl+Z deshace).",
			"↑ ↓     pedidos anteriores.",
			"PgUp/Dn desplazar la conversación.",
			"Esc     volver al editor.",
			"",
			"Si seleccionas texto en el editor, va",
			"como contexto del pedido.",
		}
		var out []pline
		for i, l := range intro {
			s := T.Pane
			if i == 0 {
				s = s.Bold(true)
			}
			for _, wl := range wrapText(l, w, false) {
				out = append(out, pline{wl, s, false})
			}
		}
		return out
	}
	var out []pline
	for i, j := range a.jobs {
		if i > 0 {
			out = append(out, pline{"", T.Pane, false}, pline{strings.Repeat("─", w), T.PaneDim, false})
		}
		out = append(out, a.jobLines(j, w)...)
	}
	return out
}

// frame dibuja el marco de un panel: doble si tiene el foco, simple si no.
func (a *App) frame(r rect, focused bool, st tcell.Style) {
	bx := Single
	if focused {
		bx = Double
	}
	a.box(r.x, r.y, r.w, r.h, bx, st)
}

func (a *App) drawPane() {
	T := a.T()
	r := a.rAgent
	focused := a.focus == focAgent
	a.fill(r.x, r.y, r.w, r.h, ' ', T.Pane)
	a.frame(r, focused, T.PaneFrame)

	title := " " + engineName(a.engine) + " "
	if j := a.job; j != nil {
		end := j.End
		if j.Running() {
			end = time.Now()
		}
		state := fmt.Sprintf("%.1fs", end.Sub(j.Start).Seconds())
		if j.Running() {
			state = spinner() + " " + state
		}
		title = fmt.Sprintf(" %s · %s ", engineName(j.Engine), state)
	}
	ts := T.PaneFrame
	if focused {
		ts = T.Title
	}
	a.putw(r.x+2, r.y, title, ts, r.w-4)

	tw := r.w - 4
	// caja de entrada: hasta 4 renglones, abajo de todo
	prompt := "› "
	in := []rune(prompt + a.chat.String())
	inLines := max(1, min(4, (len(in)+tw-1)/max(1, tw)))
	if len(in)%max(1, tw) == 0 && len(in) > 0 && inLines < 4 {
		inLines++ // lugar para el cursor al final
	}
	sepY := r.y + r.h - 2 - inLines
	inner := sepY - (r.y + 1)

	lines := a.paneLines(tw)
	maxScroll := max(0, len(lines)-inner)
	if a.paneFollow || a.paneScroll >= maxScroll {
		a.paneScroll = maxScroll
		a.paneFollow = true
	}
	for i := 0; i < inner; i++ {
		li := a.paneScroll + i
		if li >= len(lines) {
			break
		}
		pl := lines[li]
		if pl.fill {
			a.fill(r.x+2, r.y+1+i, tw, 1, ' ', pl.st)
		}
		a.putw(r.x+2, r.y+1+i, pl.text, pl.st, tw)
	}
	if maxScroll > 0 && !a.paneFollow {
		pct := fmt.Sprintf(" %d%% ", (a.paneScroll*100)/maxScroll)
		a.put(r.x+r.w-2-strW(pct), sepY, pct, T.PaneFrame)
	}

	// separador y entrada
	bx := Single
	if focused {
		bx = Double
	}
	a.s.SetContent(r.x, sepY, bx.LT, nil, T.PaneFrame)
	a.s.SetContent(r.x+r.w-1, sepY, bx.RT, nil, T.PaneFrame)
	a.fill(r.x+1, sepY, r.w-2, 1, '─', T.PaneFrame)
	mode := " Pregunta "
	if a.agent {
		mode = " Agente "
	}
	a.put(r.x+2, sepY, mode, T.PaneCode)
	if b := a.buf; b.HasSel() {
		s, e := b.SelRange()
		a.putw(r.x+2+strW(mode)+1, sepY, fmt.Sprintf(" selección %d–%d ", s.Y+1, e.Y+1), T.PaneDim, r.w-6-strW(mode))
	}
	fs := T.Pane
	if focused {
		fs = T.Pane.Bold(true)
	}
	off := 0
	if len(in) > inLines*tw {
		off = (len(in)/tw - inLines + 1) * tw
	}
	for k := 0; k < inLines; k++ {
		from := off + k*tw
		if from >= len(in) {
			break
		}
		to := min(len(in), from+tw)
		a.put(r.x+2, sepY+1+k, string(in[from:to]), fs)
	}
	if !focused && a.chat.String() == "" {
		a.putw(r.x+2+len([]rune(prompt)), sepY+1, "Ctrl+K para escribir", T.PaneDim, tw-2)
	}
	if focused && a.dlg == nil && !a.menuOpen {
		c := len([]rune(prompt)) + a.chat.c - off
		a.cursorAt(r.x+2+c%tw, sepY+1+c/tw)
	}
	hint := " Tab modo · F2 motor "
	if r.w-4 > strW(hint)+2 {
		a.put(r.x+r.w-2-strW(hint), r.y+r.h-1, hint, T.PaneFrame)
	}
}
