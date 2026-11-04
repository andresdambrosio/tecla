// tecla: un editor de texto minimalista con estética de MS-DOS / Mac clásico,
// que puede pedirle cosas a Claude Code o a Codex.
package main

import (
	"fmt"
	"os"
)

func main() {
	path := ""
	var compare []string
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-h", "--help":
			fmt.Println("uso: tecla [archivo | carpeta]\n     tecla --diff a b\n\nDentro del editor, F1 muestra la ayuda.")
			return
		case "-d", "--diff":
			if len(os.Args) != 4 {
				fmt.Fprintln(os.Stderr, "uso: tecla --diff a b")
				os.Exit(2)
			}
			compare = os.Args[2:]
			path = compare[0]
		default:
			path = os.Args[1]
		}
	}
	a, err := NewApp(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tecla:", err)
		os.Exit(1)
	}
	if compare != nil {
		a.compareFiles(compare[0], compare[1])
	}
	a.Run()
}
