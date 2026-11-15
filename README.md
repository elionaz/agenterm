# agenterm

Workspace de escritorio para agentes de código (Claude Code, codex, opencode, shell…),
organizado por proyecto. **Fase 1**: daemon + PTYs + CLI. **Fase 2**: app de escritorio
macOS (Wails v2 + Svelte + xterm.js) en `app/`, que es un cliente más del daemon
(`internal/client`). **Fase 3**: detección de "necesita tu atención" (hooks de los agentes +
señales de terminal) y notificaciones nativas, badge del Dock y sidebar.

## Uso

```sh
go build -o agenterm ./cmd/agenterm

agenterm daemon --detach            # opcional: cualquier comando lo arranca si no corre
agenterm project add ~/Dev/agenterm # -> <projectId>  name  path
agenterm project list
agenterm profiles
agenterm spawn <projectId|nombre> shell   # -> <sessionId>
agenterm ls
agenterm attach <sessionId>         # Ctrl-\ para desacoplar; la sesión sigue viva
agenterm notify --kind permission --message "¿Borro node_modules?"  # dentro de una sesión
agenterm ack <sessionId>            # marca la atención como vista
agenterm kill <sessionId>           # una segunda vez la elimina de la lista
agenterm stop                       # mata las sesiones y apaga el daemon
```

`notify` acepta `--state running|waiting_input|idle` (alias `waiting`, `done`), `--kind
permission|question|done|error` (implica el estado si no se da) y `--message`. Usa
`$AGENTERM_SESSION` / `$AGENTERM_SOCK`, nunca arranca el daemon y tiene timeout de 3 s.
`notify --from-hook claude|codex` lee el payload del hook (stdin, o último argumento en codex),
no imprime nada y **siempre** sale con 0 (timeout 2 s): nunca bloquea ni rompe al agente.

### Rutas

| | por defecto | con `AGENTERM_HOME=/x` |
|---|---|---|
| socket, pid, log | `~/.agenterm/{d.sock,d.pid,daemon.log}` | `/x/…` |
| perfiles, proyectos, ajustes | `~/.config/agenterm/{profiles.yaml,projects.json,settings.yaml}` | `/x/…` |
| ficheros de hooks generados | `~/.agenterm/hooks/` | `/x/hooks/` |

Ojo: en macOS la ruta de un unix socket tiene un límite de ~104 bytes.

### Perfiles

Si no existe `profiles.yaml` se usan los integrados (`claude`, `codex`, `opencode`, `shell`).
Formato:

```yaml
- id: claude
  cmd: claude          # se expanden $VARS (p. ej. $SHELL)
  args: []
  env: {}
  status: hooks        # hooks | heuristic
  setup_hooks: claude  # claude | codex | none: integra los hooks del agente (ver abajo)
  idle_seconds: 3      # opcional, sobreescribe el timeout heurístico
  attention_patterns:  # opcional: regex sobre la salida reciente (sin ANSI)
    - 'Do you want to proceed\?'
    - '\(y/n\)'
```

Los perfiles integrados son `claude` (`status: hooks`, `setup_hooks: claude`), `codex`
(`status: heuristic`, `setup_hooks: codex`), `opencode` y `shell` (heurística). Si ya tienes un
`profiles.yaml` propio, añade `setup_hooks` a mano.

## Atención y notificaciones (fase 3)

Cada sesión tiene, además del estado, una **atención**: `attentionKind`
(`permission` | `question` | `done` | `error`), `attentionMessage`, `attentionAt` y `unread`.
`unread` se borra al seleccionar la sesión en la app (RPC `session.ack`), al escribir en ella o
con `agenterm ack`. Un `idle` explícito tras un turno (Stop / turn-complete) es un `done`; el
`idle` heurístico (3 s sin salida) **no** avisa.

### De dónde salen las señales

| agente | mecanismo (solo en sesiones lanzadas por agenterm) | señales |
|---|---|---|
| Claude Code | `claude --settings ~/.agenterm/hooks/claude-settings.json` (generado). Los hooks de `--settings` se **suman** a los tuyos | `UserPromptSubmit`/`PostToolUse` → trabajando; `PermissionRequest` y `Notification` (`permission_prompt`, `elicitation_dialog`, `agent_needs_input`) → aprobación/pregunta con mensaje; `Stop` → terminó (con el último mensaje); `StopFailure` → error |
| codex | `codex -c notify=[…] -c tui.notification_method="osc9" -c tui.notification_condition="always" -c tui.notifications=[…]` | `notify` (`agent-turn-complete`) → terminó; las notificaciones OSC 9 del TUI (aprobaciones, preguntas) → las recoge el parser de terminal |
| opencode y cualquier otro | — | BEL y notificaciones de terminal (abajo) + `attention_patterns` |

Nunca se tocan `~/.claude/settings.json`, `~/.codex/config.toml`, la config de opencode ni el
proyecto: todo va por flags en la línea de comandos y ficheros en `~/.agenterm/hooks/`. Los hooks
llaman a `<ejecutable del daemon> notify --from-hook …` con ruta absoluta (la CLI o el propio
ejecutable del `.app`, que también implementa `notify`).

**Señales de terminal** (agnósticas del agente; los bytes llegan intactos a la terminal): BEL
suelto (no el que cierra un OSC), `OSC 9 ; mensaje` (iTerm2/Ghostty; se ignora `OSC 9;4` de
progreso y los subcomandos numéricos de ConEmu), `OSC 777 ; notify ; título ; cuerpo` y
`OSC 99` (kitty). En sesiones `heuristic` levantan "esperando" (que dura hasta que escribas); en
sesiones con hooks solo cuentan mientras el agente trabaja o para completar el mensaje.

### Notificaciones

- **App abierta**: notificación nativa (UNUserNotificationCenter) si la ventana no está activa o
  la sesión no es la visible. Título `"<proyecto> · <perfil>"`, cuerpo el mensaje (o "Necesita tu
  aprobación", "Está esperando tu respuesta", "Terminó", "Terminó con un error"). Al pulsarla se
  activa la app y se selecciona la sesión. Sonido por defecto solo en aprobación/pregunta. Badge
  del Dock con el número de sesiones pendientes y rebote del icono en aprobaciones. En la sidebar:
  punto ámbar pulsante, etiqueta (aprobación/pregunta/terminó/error) y tooltip con el mensaje;
  contador por proyecto y `agenterm (N)` en el título de la ventana.
- **App cerrada**: el daemon avisa con `osascript` (`display notification`) cuando ningún
  cliente UI está suscrito (`events.subscribe` con `{"client":"ui"}`). Esas notificaciones salen
  a nombre de "Script Editor" y no abren la app.
- **Permiso**: la primera vez que abras el `.app` macOS pregunta si permites notificaciones. Si lo
  rechazaste: *Ajustes del Sistema → Notificaciones → agenterm* → "Permitir notificaciones" (y
  estilo "Alertas" si quieres que se queden en pantalla). Si la app no tiene permiso, cae a
  `osascript` (para esas: *Notificaciones → Script Editor*).

### `settings.yaml`

```yaml
notify:
  enabled: true         # notificaciones (app y daemon)
  sound: true           # sonido en aprobación/pregunta
  notify_on_done: true  # avisar también cuando un agente termina
```

Todo está activado por defecto; el fichero es opcional y se relee en cada aviso.

## App de escritorio (fase 2)

Requisitos: Go, Node/npm y la CLI de Wails v2 (`go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1`;
`wails doctor` debe pasar).

```sh
cd app
wails dev     # app con hot reload (frontend en Vite); también sirve http://localhost:34115
wails build   # -> app/build/bin/agenterm.app
open build/bin/agenterm.app
```

Solo frontend: `cd app/frontend && npm install && npm run check && npm run build`.

- El `.app` es autosuficiente: su ejecutable también es el daemon (`agenterm.app/Contents/MacOS/agenterm daemon`).
  Al abrirse, si el daemon no responde, lo lanza desacoplado (`client.StartDaemon` con su propio
  `os.Executable()`). Cerrar la ventana o salir de la app **no** mata el daemon ni las sesiones; al
  reabrir se reconecta y repinta el scrollback. Para apagarlo todo: `agenterm stop`. La CLI y la app comparten daemon.
- Antes de lanzar el daemon, la app toma el `PATH` de tu shell de login (`$SHELL -l -i -c`) y fija
  `TERM=xterm-256color`, `COLORTERM=truecolor` y `LANG` si falta: las apps abiertas desde Finder
  heredan el entorno mínimo de launchd y no encontrarían `claude`/`codex`.
- Atajos: ⌘1…⌘9 seleccionan la sesión n del proyecto actual, ⌘N lanza una sesión con el primer
  perfil en el proyecto actual, ⌘C/⌘V copian/pegan en la terminal.
- `AGENTERM_HOME` también vale para la app (útil para probar sin tocar `~/.agenterm`):
  `AGENTERM_HOME=/tmp/at wails dev`.

### Eventos Go → frontend

| evento | payload |
|---|---|
| `pty:<id>` | salida en base64, agrupada (≤ 8 ms o 32 KiB por evento) |
| `pty-replay-done:<id>` | fin del replay del scrollback |
| `pty-exit:<id>` | código de salida |
| `pty-error:<id>` | stream perdido (el frontend se reengancha y repinta) |
| `daemon:event` | `session.spawned/state/attention/removed` del daemon (con `attention*`, `unread`, `raised`) |
| `app:focus-session` | se pulsó una notificación: seleccionar esa sesión |
| `daemon:resync` | (re)suscrito: recargar listas |
| `daemon:down` | sin conexión con el daemon (se reintenta y se relanza) |

## Arquitectura

```
cmd/agenterm        CLI + subcomando `daemon` (un solo binario)
app/                App Wails: bindings (app.go), coalescer de salida, entorno del daemon
app/frontend        Svelte 5 + TypeScript + xterm.js (WebGL, fit, unicode11)
internal/daemoncmd  subcomando `daemon` compartido por la CLI y la app
internal/daemon     servidor: registro de sesiones, RPC, attach, eventos, pid/socket
internal/client     cliente reutilizable (CLI hoy, Wails mañana) + StartDaemon
internal/protocol   tipos del wire: JSON por líneas + frames binarios
internal/transport  interfaz Listen/Dial/Cleanup (unix socket; named pipe en el futuro)
internal/session    PTY (creack/pty), ring buffer, fan-out a suscriptores, kill/wait
internal/status     máquina de estados + atención + heurística (reloj inyectable)
internal/termsig    parser streaming de BEL / OSC 9 / 777 / 99
internal/hooks      config de hooks por sesión (claude, codex) y parseo de payloads
internal/notifycmd  subcomando `notify` compartido por la CLI y la app
internal/desktopnotify  textos de notificación + fallback osascript
internal/settings   settings.yaml
internal/profile    perfiles YAML
internal/project    registro de proyectos en JSON
internal/paths      resolución de directorios
```

### Protocolo

- **Control**: JSON delimitado por `\n`. Petición `{"id":1,"method":"session.list","params":{…}}`,
  respuesta `{"id":1,"result":{…}}` o `{"id":1,"error":"…"}`. Una conexión admite varias
  peticiones seguidas. Métodos: `project.add|list|remove`, `profile.list`,
  `session.spawn|list|kill|resize|notify|ack|attach`, `events.subscribe`, `daemon.ping|shutdown`.
- **`events.subscribe`** (`{"client":"ui"}` opcional): tras la respuesta, el daemon emite un evento
  JSON por línea (`session.spawned`, `session.state`, `session.attention`, `session.removed`).
  `raised: true` marca una atención nueva (lo que merece notificación). Si el cliente se retrasa se le
  desconecta (debe resuscribirse y hacer `session.list`).
- **`session.attach`**: tras la respuesta JSON la conexión pasa a frames binarios
  `tipo(1 byte) | longitud(4 bytes BE) | payload` (máx. 1 MiB):
  - daemon → cliente: `0x01` salida, `0x02` fin del replay, `0x03` exit (int32 BE), `0x04` error;
  - cliente → daemon: `0x10` input, `0x11` resize (cols, rows uint16 BE).

  El daemon envía primero el snapshot del ring buffer y después la salida en vivo. Snapshot y
  suscripción se toman bajo el mismo lock, así que no hay huecos ni duplicados.

### Sesiones y estado

- Cada sesión tiene un ring buffer (8 MiB por defecto) y N suscriptores con cola acotada; un
  suscriptor lento se desconecta (frame `0x04`) en vez de bloquear la lectura del PTY.
- Se inyecta `AGENTERM_SESSION` y `AGENTERM_SOCK` en el entorno del proceso.
- Estados: `running`, `waiting_input`, `idle`, `exited`. Heurística: salida → `running`, 3 s
  sin salida → `idle`; `waiting_input` no lo termina la salida (spinners), sino lo que escribas.
  En perfiles `status: hooks`, el primer evento explícito desactiva la heurística para esa sesión.
  `exited` es terminal. Las respuestas automáticas de la terminal (focus, DA, DSR) no cuentan como
  "escribir".
- `kill` envía SIGHUP+SIGTERM al grupo de procesos y SIGKILL tras 2 s. SIGTERM/SIGINT al
  daemon (o `agenterm stop`) mata todas las sesiones y borra socket y pid file. Un socket
  huérfano (nadie escuchando) se detecta y se reemplaza al arrancar.

## Tests

```sh
go vet ./... && go test -race ./...
```

Incluye un test de integración que levanta el daemon en un `AGENTERM_HOME` temporal y ejercita
spawn, attach/detach/reattach con scrollback, notify, eventos, kill y apagado.
