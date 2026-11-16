<p align="center">
  <img src="assets/icon/agenterm-icon-rounded.svg" alt="agenterm" width="96">
</p>

# agenterm

[English](README.md) · **Español**

Workspace de escritorio para agentes de código, organizado por proyecto. Las pestañas de una
terminal no se pueden agrupar por proyecto, y cuando lanzas varios agentes (Claude Code, codex,
opencode, Gemini, Cursor…) sobre el mismo repo es difícil saber cuál está trabajando, cuál terminó
y cuál está esperando tu aprobación. agenterm agrupa las sesiones por proyecto, mantiene los agentes
vivos aunque cierres la app y te avisa cuando alguno te necesita.

<!-- TODO: captura — añadir docs/img/screenshot.png -->

## Funciones

- **Proyectos y sesiones** en una barra lateral: cada proyecto es una carpeta y cada sesión una
  terminal (xterm.js) con un agente o una shell.
- **Cualquier CLI de agente**: catálogo integrado (Claude Code, codex, opencode, Gemini, Cursor,
  Copilot, aider, Amp, goose, Qwen, Crush, Kiro, shell) con detección automática de los instalados,
  más perfiles propios y variantes en `profiles.yaml`.
- **Las sesiones sobreviven a la app**: un daemon es dueño de las PTYs; cierras la ventana, la
  reabres y todo sigue ahí con su scrollback. La CLI y la app comparten el mismo daemon.
- **Detección de atención**: aprobaciones, preguntas, turnos terminados y errores (hooks del agente
  o señales de terminal), con notificaciones nativas de macOS, badge del Dock y marcas en la sidebar.
- **Actividad en vivo**: qué está haciendo cada agente ("Editando main.go"), cronómetro del turno,
  aviso de «¿trabado?», barras de progreso (OSC 9;4) y una vista general en rejilla (⌘0).
- **Worktrees de git por sesión**: cada agente puede trabajar en su propia rama y carpeta; la
  pestaña **Cambios** muestra el diff y permite hacer merge a la base o eliminar el worktree de forma
  segura.
- **Ajustes en la app**: activar, ordenar y editar agentes, y configurar el idioma, las
  notificaciones y git.
- **En inglés y español**: la app sigue los idiomas preferidos de macOS (con inglés como respaldo)
  o el idioma que elijas en *Ajustes → General*.

## Estado y requisitos

Proyecto en etapa temprana: todavía no hay releases firmadas, así que hay que compilarlo desde el
código fuente.

- **macOS** (probado en Apple Silicon). La app y las notificaciones son exclusivas de macOS.
- **Go 1.27.4+** (ver `go.mod`).
- **Node 24+** y npm (para el frontend; es la versión que usa el CI).
- **CLI de Wails v2**: `go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1` (`wails doctor`
  debe pasar).

## Instalación desde el código fuente

```sh
git clone https://github.com/elionaz/agenterm.git
cd agenterm

# App de escritorio (incluye el daemon)
cd app && wails build          # -> app/build/bin/agenterm.app
open build/bin/agenterm.app    # o cópiala a /Applications

# CLI (opcional)
cd .. && go build -o agenterm ./cmd/agenterm
```

La primera vez que abras la app, macOS pregunta si permites notificaciones. Si lo rechazaste:
*Ajustes del Sistema → Notificaciones → agenterm* → "Permitir notificaciones".

## Inicio rápido

**App.** Haz clic en **+ Proyecto** y elige una carpeta. Con el **+** junto al proyecto eliges un agente
y si corre en la misma carpeta o en un worktree nuevo; **⌘N** lanza el primer agente de la lista.
Cuando un agente te necesita verás un punto ámbar en la sidebar y una notificación.

**CLI.** Cualquier comando arranca el daemon si no está corriendo:

```sh
agenterm project add ~/Dev/miapp           # -> <projectId>  miapp  /Users/yo/Dev/miapp
agenterm profiles                          # agentes del catálogo, instalados o no
agenterm spawn miapp claude                # -> <sessionId>
agenterm spawn --worktree miapp codex      # -> <sessionId>  <rama>  <carpeta>
agenterm ls                                # estado, atención, actividad, rama…
agenterm attach <sessionId>                # Ctrl-\ para desacoplar; la sesión sigue viva
agenterm stop                              # se niega si quedan sesiones vivas (usa --force)
```

Referencia completa: [docs/es/cli.md](docs/es/cli.md).

## Cómo se detecta la atención

Todo se configura **solo para las sesiones que lanza agenterm**, con flags en la línea de comandos y
archivos generados en `~/.agenterm/hooks/`. Nunca se modifican tus configuraciones
(`~/.claude/settings.json`, `~/.codex/config.toml`…) ni la carpeta del proyecto.

| agente | mecanismo | qué detecta |
|---|---|---|
| Claude Code | hooks vía `claude --settings <archivo generado>` (se suman a los tuyos) | trabajando + actividad, aprobación, pregunta, terminó, error |
| codex | `codex -c notify=[…]` + notificaciones OSC 9 del TUI | terminó (`notify`); aprobaciones y preguntas (OSC 9) |
| los demás | señales de terminal: BEL, OSC 9, OSC 777, OSC 99 y `attention_patterns` | esperando tu respuesta |

Detalles (estados, *ack*, «¿trabado?», progreso, notificaciones con la app cerrada):
[docs/es/attention-and-activity.md](docs/es/attention-and-activity.md).

## Configuración

| | por defecto | con `AGENTERM_HOME=/x` |
|---|---|---|
| socket, pid, log del daemon | `~/.agenterm/{d.sock,d.pid,daemon.log}` | `/x/…` |
| `profiles.yaml`, `settings.yaml`, `projects.json` | `~/.config/agenterm/` | `/x/` |
| hooks generados, worktrees, `worktrees.json` | `~/.agenterm/{hooks,worktrees,worktrees.json}` | `/x/…` |

`profiles.yaml` y `settings.yaml` son opcionales y la app los edita desde **Ajustes** (⌘,).

**`profiles.yaml`** se superpone al catálogo por `id`: cambias solo los campos que pongas, creas
variantes con `extends`, ocultas agentes con `enabled: false` y el orden de la lista es el del menú
(el primero es el de ⌘N):

```yaml
- id: claude-yolo                 # variante: hereda todo de claude
  name: Claude (sin permisos)
  extends: claude
  args: ["--dangerously-skip-permissions"]
- id: claude
  env: {CLAUDE_CODE_USE_BEDROCK: "1"}
- id: gemini
  enabled: false
- id: mio                         # perfil propio
  cmd: $HOME/bin/mi-agente
  attention_patterns: ['\(y/n\)']
```

**`settings.yaml`** (valores por defecto):

```yaml
ui:
  language: auto         # auto (idiomas preferidos de macOS; si no, inglés) | en | es
notify:
  enabled: true          # notificaciones (app y daemon)
  sound: true            # sonido en aprobación/pregunta
  notify_on_done: true   # avisar también cuando un agente termina
  notify_on_stall: true  # aviso discreto si un turno parece trabado
activity:
  stall_seconds: 180     # segundos sin salida ni eventos para «¿trabado?»; 0 lo desactiva
git:
  editor: ""             # para «Abrir en editor»; vacío: code si existe, si no Finder
  worktree_dir: ""       # dónde crear worktrees; vacío: ~/.agenterm/worktrees
  default_mode: same     # same | worktree
```

Todas las reglas y el catálogo completo: [docs/es/configuration.md](docs/es/configuration.md). Worktrees
y diff: [docs/es/worktrees.md](docs/es/worktrees.md).

## Atajos de teclado

| atajo | acción |
|---|---|
| ⌘N | nueva sesión en el proyecto actual con el primer agente activado e instalado |
| ⌘1 … ⌘9 | seleccionar la sesión n del proyecto actual |
| ⌘0 | abrir/cerrar la vista general |
| ⌘, | abrir Ajustes |
| ⌘C / ⌘V | copiar la selección / pegar en la terminal |
| Esc | cerrar la vista general, Ajustes o la vista de un worktree huérfano |
| `Ctrl-\` | desacoplarse de `agenterm attach` (CLI) |

## Documentación

- [docs/es/cli.md](docs/es/cli.md): todos los comandos y flags de la CLI.
- [docs/es/configuration.md](docs/es/configuration.md): rutas, `profiles.yaml`, catálogo de agentes y
  `settings.yaml`.
- [docs/es/attention-and-activity.md](docs/es/attention-and-activity.md): hooks, señales de terminal,
  notificaciones, actividad, «¿trabado?» y progreso.
- [docs/es/worktrees.md](docs/es/worktrees.md): worktrees por sesión, diff, merge y borrado.
- [docs/es/architecture.md](docs/es/architecture.md): daemon, app y CLI, protocolo, eventos y
  persistencia.
- [docs/es/development.md](docs/es/development.md): desarrollo, tests, icono y limitaciones conocidas.

Si cambias comandos, flags, rutas o `settings.yaml`, actualiza el README y el documento
correspondiente en `docs/`, en inglés (`README.md`, `docs/`) y en español (`README.es.md`,
`docs/es/`).

## Contribuir

Las contribuciones son bienvenidas vía pull request: lee [CONTRIBUTING.es.md](CONTRIBUTING.es.md). Para
reportar vulnerabilidades, sigue [SECURITY.es.md](SECURITY.es.md).

Licencia: [MIT](LICENSE).
