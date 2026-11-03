# tecla

Editor de texto minimalista con estética de MS-DOS EDIT / Turbo C / Mac clásico,
con **Claude Code** (`claude`) o **Codex** (`codex`) adentro.

    go build -o tecla . && ./tecla [archivo | carpeta]

    ┌ archivos ┐┌──────── editor ────────┐┌─ agente ─┐
    │          ││                        ││          │
    │          │├─────── terminal ───────┤│          │
    └──────────┘└────────────────────────┘└──────────┘

- F6 cambia de panel (o clic) · Ctrl+B árbol · F7 terminal · F4 agente
- Árbol: Enter abre, ←→ pliega, `r` usa la carpeta como raíz, Retroceso sube
- Terminal: un shell de verdad (PTY); F12 / Ctrl+F9 ejecuta el archivo actual
- Agente: Ctrl+K para escribir, Tab alterna Agente/Pregunta, F5 aplica código
- Temas (F9): DOS · Turbo · Mac · Ámbar · Fósforo · F1: todos los atajos

Configuración en `~/.config/tecla/config`.
