package main

import (
	"os"
	"testing"
	"time"
)

func TestLiveAI(t *testing.T) {
	if os.Getenv("TECLA_LIVE") == "" {
		t.Skip("TECLA_LIVE no definido")
	}
	home, _ := os.UserHomeDir()
	for _, eng := range []string{"claude", "codex"} {
		a, s := newTestApp(t, "")
		os.Setenv("HOME", home) // los CLIs necesitan su login real
		a.engine = eng
		typeStr(a, "def suma(a, b):\n")
		a.buf.SetSel(Pos{0, 0}, Pos{1, 0})
		a.startAI("Agrega un docstring corto a esta función y devuelve la función completa.", false)
		deadline := time.Now().Add(150 * time.Second)
		for a.job.Running() && time.Now().Before(deadline) {
			time.Sleep(500 * time.Millisecond)
		}
		a.draw()
		t.Logf("[%s] salida:\n%s", eng, a.job.Text())
		a.applyAI()
		a.draw()
		t.Logf("[%s] buffer tras F5:\n%s\n%s", eng, a.buf.Text(), screenText(s))
	}
}
