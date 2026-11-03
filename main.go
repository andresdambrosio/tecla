// tecla: un editor de texto minimalista con estética de MS-DOS / Mac clásico,
// que puede pedirle cosas a Claude Code o a Codex.
package main

import (
	"fmt"
	"os"
)

func main() {
	path := ""
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-h", "--help":
			fmt.Println("uso: tecla [archivo]\n\nDentro del editor, F1 muestra la ayuda.")
			return
		}
		path = os.Args[1]
	}
	a, err := NewApp(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tecla:", err)
		os.Exit(1)
	}
	a.Run()
}
