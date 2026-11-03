package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gdamore/tcell/v2"
)

// AIJob es una consulta en curso (o terminada) a `claude` o `codex`.
type AIJob struct {
	Engine, Prompt string
	Agent          bool
	Start, End     time.Time
	sid            string // sesión de Claude
	ok             bool

	mu      sync.Mutex
	out     []byte
	running bool
	cancel  func()
	post    func()
	handled bool

	buf     *Buffer
	path    string
	hadSel  bool
	selS    Pos
	selE    Pos
	selText string
}

func (j *AIJob) Write(p []byte) (int, error) {
	j.mu.Lock()
	j.out = append(j.out, p...)
	j.mu.Unlock()
	j.post()
	return len(p), nil
}

func (j *AIJob) Text() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return string(j.out)
}

func (j *AIJob) Running() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.running
}

func itoa(n int) string { return strconv.Itoa(n) }

func (a *App) quickAI(prompt string) {
	if len(a.have) == 0 {
		a.askAI()
		return
	}
	a.startAI(prompt, false)
}

func (a *App) startAI(prompt string, agent bool) {
	if a.job != nil && a.job.Running() {
		a.flash("Ya hay una consulta en curso (F8 la cancela)")
		return
	}
	eng := a.engine
	if !a.have[eng] {
		a.msgBox("Falta "+engineName(eng), "No encontré `"+eng+"` en el PATH.", "Cambia de motor con F2.")
		return
	}
	b := a.buf
	if agent && b.Dirty {
		if b.Path == "" {
			a.msgBox("Modo agente", "Guarda el archivo primero:", "el agente trabaja sobre el disco.")
			return
		}
		if err := b.Save(b.Path); err != nil {
			a.msgBox("Error al guardar", err.Error())
			return
		}
	}

	j := &AIJob{Engine: eng, Prompt: prompt, Agent: agent, Start: time.Now(), running: true, buf: b, path: b.Path}
	j.post = func() { _ = a.s.PostEvent(tcell.NewEventInterrupt(nil)) }
	if b.HasSel() {
		s, e := b.SelRange()
		j.hadSel, j.selS, j.selE, j.selText = true, s, e, b.TextRange(s, e)
	}
	full := a.buildPrompt(j)

	dir := a.root

	ctx, cancel := context.WithCancel(context.Background())
	j.cancel = cancel
	var cmd *exec.Cmd
	lastFile := ""
	switch eng {
	case "claude":
		args := []string{"-p", "--output-format", "text"}
		if a.claudeSID != "" {
			j.sid = a.claudeSID
			args = append(args, "--resume", j.sid)
		} else {
			j.sid = newUUID()
			args = append(args, "--session-id", j.sid)
		}
		if agent {
			args = append(args, "--permission-mode", "acceptEdits")
		}
		cmd = exec.CommandContext(ctx, "claude", args...)
	default:
		if f, err := os.CreateTemp("", "tecla-codex-*.txt"); err == nil {
			lastFile = f.Name()
			f.Close()
		}
		sandbox := "read-only"
		if agent {
			sandbox = "workspace-write"
		}
		args := []string{"exec", "--skip-git-repo-check", "--color", "never", "-s", sandbox}
		if lastFile != "" {
			args = append(args, "-o", lastFile)
		}
		args = append(args, "-")
		cmd = exec.CommandContext(ctx, "codex", args...)
	}
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(full)
	cmd.Stdout = j
	cmd.Stderr = j
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
	cmd.WaitDelay = 3 * time.Second

	a.job = j
	a.jobs = append(a.jobs, j)
	if len(a.jobs) > 30 {
		a.jobs = a.jobs[len(a.jobs)-30:]
	}
	a.pane = true
	a.paneFollow = true

	if err := cmd.Start(); err != nil {
		j.running = false
		j.End = time.Now()
		j.out = []byte("[no se pudo ejecutar " + eng + ": " + err.Error() + "]")
		j.handled = true
		return
	}
	go func() {
		t := time.NewTicker(150 * time.Millisecond)
		defer t.Stop()
		for range t.C {
			if !j.Running() {
				return
			}
			j.post()
		}
	}()
	go func() {
		err := cmd.Wait()
		j.mu.Lock()
		if lastFile != "" {
			if data, e := os.ReadFile(lastFile); e == nil && strings.TrimSpace(string(data)) != "" {
				j.out = data
			}
			os.Remove(lastFile)
		}
		j.ok = err == nil && ctx.Err() == nil
		if ctx.Err() != nil {
			j.out = append(j.out, "\n[cancelado]"...)
		} else if err != nil {
			j.out = append(j.out, fmt.Sprintf("\n[error: %v]", err)...)
		}
		j.running = false
		j.End = time.Now()
		j.mu.Unlock()
		_ = a.s.PostEvent(tcell.NewEventInterrupt(j))
	}()
}

const maxCtxLines = 2000

func (a *App) buildPrompt(j *AIJob) string {
	b := a.buf
	var sb strings.Builder
	name := b.Path
	if name == "" {
		name = "(sin guardar)"
	}
	lang := langFor(b.Path)
	selBlock := func() {
		if j.hadSel {
			fmt.Fprintf(&sb, "\nSelección actual (líneas %d-%d):\n<seleccion>\n%s\n</seleccion>\n", j.selS.Y+1, j.selE.Y+1, j.selText)
		}
	}

	if j.Agent {
		fmt.Fprintf(&sb, "Trabajas desde tecla, un editor de texto minimalista, en la carpeta %s. ", a.root)
		if b.Path != "" {
			fmt.Fprintf(&sb, "El usuario tiene abierto %s (cursor en la línea %d).\n", a.relPath(b.Path), b.Cur.Y+1)
		} else {
			sb.WriteString("El usuario no tiene ningún archivo abierto.\n")
		}
		selBlock()
		sb.WriteString("\nPuedes leer y editar los archivos de esta carpeta para cumplir el pedido. ")
		sb.WriteString("Al terminar, resume en pocas líneas, en texto plano, qué cambiaste.\n\nPedido: ")
		sb.WriteString(j.Prompt)
		return sb.String()
	}

	sb.WriteString("Estás integrado en tecla, un editor de texto minimalista que corre en una terminal. ")
	sb.WriteString("Tu respuesta se muestra en una columna angosta de texto plano: sé breve y directo, sin tablas ni markdown decorativo. ")
	sb.WriteString("Responde en el idioma del pedido. No modifiques archivos: sólo responde.\n")
	if j.hadSel {
		sb.WriteString("Si el pedido implica escribir o cambiar código, incluye el resultado en UN único bloque ``` que reemplazará exactamente la selección (sin números de línea, conservando la indentación).\n")
	} else {
		sb.WriteString("Si el pedido implica escribir código, incluye el código en UN único bloque ``` que se insertará donde está la marca <<CURSOR>> (sin números de línea, con la indentación correcta).\n")
	}
	fmt.Fprintf(&sb, "\nArchivo: %s", name)
	if lang != nil {
		fmt.Fprintf(&sb, " (%s)", lang.Name)
	}
	fmt.Fprintf(&sb, ", %d líneas, cursor en la línea %d.\n", len(b.Lines), b.Cur.Y+1)
	selBlock()

	from, to := 0, len(b.Lines)
	if to > maxCtxLines {
		from = max(0, b.Cur.Y-maxCtxLines/2)
		to = min(len(b.Lines), from+maxCtxLines)
		fmt.Fprintf(&sb, "(Se muestra un fragmento: líneas %d-%d)\n", from+1, to)
	}
	sb.WriteString("<archivo>\n")
	for y := from; y < to; y++ {
		l := b.Lines[y]
		if !j.hadSel && y == b.Cur.Y {
			sb.WriteString(string(l[:b.Cur.X]) + "<<CURSOR>>" + string(l[b.Cur.X:]))
		} else {
			sb.WriteString(string(l))
		}
		sb.WriteByte('\n')
	}
	sb.WriteString("</archivo>\n\nPedido: ")
	sb.WriteString(j.Prompt)
	return sb.String()
}

func (a *App) aiDone(j *AIJob) {
	dur := j.End.Sub(j.Start).Round(100 * time.Millisecond).String()
	if j.Engine == "claude" && j.ok && j.sid != "" {
		a.claudeSID = j.sid
	}
	a.refreshTree()
	if j.Agent && j.buf == a.buf && a.buf.Path == j.path {
		b := a.buf
		data, err := os.ReadFile(b.Path)
		switch {
		case err != nil:
			a.flash("El agente terminó (" + dur + ")")
		case string(data) == b.Text():
			a.flash("El agente terminó en " + dur + " · este archivo no cambió")
		case b.Dirty:
			a.flash("El agente cambió el archivo, pero tienes cambios sin guardar (Archivo › Recargar)")
		default:
			b.ReloadText(string(data))
			a.flash("Cambios del agente cargados (" + dur + ") · Ctrl+Z deshace")
		}
		return
	}
	if _, ok := extractCode(j.Text()); ok && !j.Agent {
		a.flash("Respuesta lista (" + dur + ") · F5 aplica el código")
	} else {
		a.flash("Respuesta lista (" + dur + ")")
	}
}

func (a *App) cancelAI() {
	if a.job == nil || !a.job.Running() {
		a.flash("No hay ninguna consulta en curso")
		return
	}
	a.job.cancel()
	a.flash("Cancelando...")
}

func (a *App) copyAnswer() {
	if a.job == nil {
		return
	}
	a.setClip(a.job.Text())
	a.flash("Respuesta copiada al portapapeles")
}

// extractCode devuelve el contenido del primer bloque ``` de la respuesta.
func extractCode(s string) (string, bool) {
	lines := strings.Split(s, "\n")
	start := -1
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			if start < 0 {
				start = i
				continue
			}
			return strings.Join(lines[start+1:i], "\n") + "\n", true
		}
	}
	return "", false
}

func (a *App) applyAI() {
	j := a.job
	switch {
	case j == nil:
		a.flash("Todavía no hay respuesta de la IA (Ctrl+K)")
		return
	case j.Running():
		a.flash("Espera a que termine la respuesta")
		return
	case j.Agent:
		a.flash("En modo agente los cambios ya se escribieron en disco")
		return
	}
	code, ok := extractCode(j.Text())
	if !ok {
		a.flash("La respuesta no trae un bloque de código para aplicar")
		return
	}
	b := a.buf
	target := ""
	if j.hadSel && j.buf == b && b.validPos(j.selS) && b.validPos(j.selE) && b.TextRange(j.selS, j.selE) == j.selText {
		b.SetSel(j.selS, j.selE)
		target = j.selText
	} else if b.HasSel() {
		s, e := b.SelRange()
		target = b.TextRange(s, e)
	} else if b.Cur.X > 0 {
		target = "x" // insertar a mitad de línea: sin salto final
	}
	if target != "" && !strings.HasSuffix(target, "\n") {
		code = strings.TrimSuffix(code, "\n")
	}
	b.InsertText(code, "ai")
	b.lastOp = ""
	a.focus = focEditor
	a.follow = true
	a.flash("Código aplicado · Ctrl+Z deshace")
}
