# Atención y actividad

[English](../attention-and-activity.md) · **Español**

Cada sesión tiene un **estado** (qué hace el proceso), una **atención** (por qué te necesita) y
datos de **actividad** (qué está haciendo ahora, cuánto lleva, si parece trabada).

## Estados

| estado | significado |
|---|---|
| `running` | trabajando |
| `waiting_input` | esperando tu respuesta (aprobación, pregunta o señal de la terminal) |
| `idle` | inactivo / turno terminado |
| `exited` | el proceso terminó (estado final) |
| `suspended` | restaurada tras un reinicio del daemon: aún sin proceso, solo su scrollback guardado; reanúdala o descártala (ver [architecture.md](architecture.md#persistencia-y-reanudación)) |

Hay dos fuentes:

- **Heurística** (perfiles `status: heuristic`): salida → `running`; 3 s sin salida (o
  `idle_seconds` del perfil, o `agenterm daemon --idle`) → `idle`. `waiting_input` no lo termina la
  salida (los spinners siguen pintando), sino lo que escribas. Las respuestas automáticas de la
  terminal (foco, DA, DSR…) no cuentan como escribir.
- **Eventos explícitos** (hooks de los agentes o `agenterm notify`). En perfiles `status: hooks`, el
  primer evento explícito desactiva la heurística para esa sesión.

## Atención

| campo | |
|---|---|
| `attentionKind` | `permission`, `question`, `done` o `error` (vacío si no te necesita) |
| `attentionMessage` | texto (la orden que pide permiso, la pregunta, el último mensaje…) |
| `attentionAt` | cuándo se levantó |
| `unread` | no la has visto todavía |

- `unread` se borra cuando la sesión está **a la vista** en la app, seleccionada o en cualquier
  panel, con la ventana activa (RPC `session.ack`), al **escribir en
  ella** o con `agenterm ack <id>`.
- Un `idle` explícito tras un turno (hook `Stop` de Claude Code, `agent-turn-complete` de codex) es
  un `done`. El `idle` heurístico **no** avisa: que no haya salida no significa que terminó.

## De dónde salen las señales

Las integraciones solo se aplican a las sesiones que lanza agenterm. Nunca se tocan
`~/.claude/settings.json`, `~/.codex/config.toml`, la configuración de opencode ni el proyecto:
todo va por flags en la línea de comandos y archivos generados en `~/.agenterm/hooks/` (o
`$AGENTERM_HOME/hooks/`). Los hooks llaman a `<ejecutable del daemon> notify --sock <socket>
--from-hook …` con rutas absolutas: la CLI, o el propio ejecutable del `.app` si el daemon lo
arrancó la app.

### Claude Code (`setup_hooks: claude`)

agenterm genera `hooks/claude-settings.json` y lanza `claude --settings <ese archivo> …`. Los hooks
de `--settings` se **suman** a los tuyos. Timeout de 5 s por hook (3 s para `PreToolUse`).

| hook | efecto |
|---|---|
| `SessionStart` | sin cambio de estado: guarda el `session_id` de Claude para poder reanudar la conversación tras un reinicio del daemon |
| `UserPromptSubmit` | `running`, actividad "Pensando…" y nuevo turno (reinicia el cronómetro) |
| `PreToolUse` | `running` y línea de actividad según la herramienta (ver abajo) |
| `PostToolUse` | `running` |
| `PermissionRequest` | `waiting_input` + `permission`, con un resumen ("Bash: rm -rf node_modules") |
| `Notification` `permission_prompt` | `waiting_input` + `permission` |
| `Notification` `elicitation_dialog`, `elicitation_url_dialog`, `agent_needs_input` | `waiting_input` + `question` |
| `Stop` | `idle` + `done`, con el último mensaje del asistente |
| `StopFailure` | `idle` + `error`, con el detalle del error |

Las demás notificaciones (como el recordatorio `idle_prompt`) se ignoran. `notify --from-hook` no
imprime nada y sale con 0, así que **nunca** emite una decisión de permisos: cada herramienta sigue
el flujo normal de aprobación de Claude.

### codex (`setup_hooks: codex`)

```sh
codex -c 'notify=["<exe>","notify","--sock","<socket>","--from-hook","codex"]' \
      -c 'tui.notifications=["approval-requested","plan-mode-prompt","async-question"]' \
      -c 'tui.notification_method="osc9"' \
      -c 'tui.notification_condition="always"' …
```

- `notify` con `agent-turn-complete` → `idle` + `done` (con el último mensaje).
- Las aprobaciones y preguntas llegan como notificaciones OSC 9 del TUI y las recoge el parser de
  señales de terminal.
- codex no avisa cuando vuelve a trabajar, así que el perfil usa `status: heuristic`.

### Señales de terminal (cualquier agente)

El parser es agnóstico del agente y no modifica el flujo: los bytes llegan intactos a la terminal.

| señal | |
|---|---|
| BEL suelto | sin mensaje (no cuenta el BEL que cierra un OSC) |
| `OSC 9 ; mensaje` | iTerm2 / Ghostty. `OSC 9;4` es **progreso** (abajo) y nunca pide atención; el resto de subcomandos numéricos de ConEmu se ignoran |
| `OSC 777 ; notify ; título ; cuerpo` | urxvt / foot / Ghostty |
| `OSC 99` | kitty |
| `attention_patterns` | regex del perfil sobre la salida reciente sin ANSI, evaluadas cuando la salida lleva ~300 ms quieta |

Una señal pone la sesión en `waiting_input` con atención `question`, que dura hasta que escribas. En sesiones con hooks solo cuentan mientras el agente está `running` (los
hooks ya informan esperas y finales de turno); si ya hay una atención sin leer, una señal solo
completa su mensaje.

## Notificaciones

### Con la app abierta

La app se suscribe al daemon como cliente `ui` y es ella quien notifica:

- Notificación nativa si la ventana no está activa o la sesión no está a la vista (en ningún
  panel). Título `<proyecto> · <nombre>` (el nombre de la sesión, o su perfil); cuerpo, el mensaje o "Necesita tu aprobación", "Está esperando tu
  respuesta", "Terminó", "Terminó con un error" (en el idioma de la app, `ui.language`). Al pulsarla
  se activa la app y se selecciona la sesión.
- Sonido solo en aprobaciones y preguntas (`notify.sound`).
- Badge del Dock con el número de sesiones pendientes y rebote del icono en aprobaciones (si la app
  no está al frente).
- En la sidebar: punto ámbar pulsante, etiqueta (aprobación / pregunta / terminó / error), tooltip
  con el mensaje, contador por proyecto y `agenterm (N)` en el título de la ventana.
- Al marcar la atención como vista se retira la notificación entregada.

### Con la app cerrada

Cuando ningún cliente `ui` está suscrito, avisa el daemon:

- Si su ejecutable está dentro de un `.app` (lo arrancó la app) y el bundle tiene permiso, usa
  `UNUserNotificationCenter`: la notificación sale como **agenterm**, con su icono y el mismo
  identificador que usa la app (`agenterm-<sessionId>`). La primera vez pide el permiso.
- Al pulsarla, el daemon ejecuta `open -a <su .app> [--env AGENTERM_HOME=…]
  agenterm://session/<id>`: la app se abre (o se activa) y selecciona esa sesión.
- Si el daemon no está en un bundle (lo arrancó la CLI), no hay permiso o falla el envío, cae a
  `osascript` (`display notification`): macOS la atribuye a *Script Editor* y pulsarla no abre
  agenterm.
- Nada de esto bloquea el daemon: el envío nativo tiene un límite de ~3 s y `osascript` de 5 s.
- `daemon.log` registra cada aviso (`notifier=native` u `osascript`, o el estado del permiso si no
  se pudo usar el nativo) y los clics (`notification clicked`).

### Permiso de notificaciones

La primera vez que abras el `.app` macOS pregunta si permites notificaciones. Si lo rechazaste:
*Ajustes del Sistema → Notificaciones → agenterm* → "Permitir notificaciones" (y estilo "Alertas"
si quieres que se queden en pantalla). Las de `osascript` se controlan en *Notificaciones → Script
Editor*.

### Esquema de URL

`agenterm://session/<id>` abre la app en esa sesión. Está registrado en `CFBundleURLTypes` (vía
`protocols` en `app/wails.json`) y la app lo recibe con `mac.Options.OnUrlOpen`; si la app no estaba
abierta, selecciona la sesión al terminar de cargar.

## Actividad

Cada sesión lleva, en `session.list` y en los eventos:

| campo | |
|---|---|
| `activity`, `activityAt`, `activitySource` | línea de actividad y su origen: `hook` u `output` |
| `turnStartedAt`, `turnEndedAt` | inicio del turno actual (o último) y su fin (al pasar a `idle`/`exited`) |
| `lastOutputAt`, `lastEventAt` | última salida del PTY y último evento explícito |
| `stalled`, `stalledAt` | el turno parece trabado (es una marca, no un estado) |
| `progress` | `{state: normal\|error\|indeterminate\|paused, value: 0-100}` |

### Línea de actividad

- **Claude Code** (hook `PreToolUse`): `Edit`/`Write`/`MultiEdit`/`NotebookEdit` → "Editando
  main.go", `Read` → "Leyendo go.mod", `Bash` → "Bash: <comando, ~60 caracteres>", `Grep`/`Glob` →
  "Buscando <patrón>", `WebFetch` → "Web: <host>", `WebSearch` → "Web: <consulta>", `Task`/`Agent` →
  "Subagente: <descripción>", `TodoWrite` → "Actualizando tareas", `mcp__srv__tool` → "MCP:
  srv.tool"; cualquier otra, su nombre. Con hooks, la salida no pisa la actividad. El daemon manda
  `activityKind` (`thinking`, `edit`, `read`, `bash`, `search`, `web`, `subagent`, `todo`, `mcp`,
  `tool`) y `activityTarget` junto al texto `activity` en inglés, y la app lo muestra en su idioma
  (los textos de arriba son los de la interfaz en español).
- **Resto de agentes**: la última línea con contenido de la salida (sin ANSI), recortada. Un CR que
  redibuja la línea se queda con su última versión. Se ignoran líneas solo de spinner o de dibujo de
  cajas, con menos de dos letras o dígitos, o que parecen el **prompt de la shell** (cortas y
  terminadas en `$ % # > ❯ ➜` en las formas típicas, o empezando por `usuario@host`), así que en una
  shell ves la última salida y no `usuario@mac ~ %`. En TUIs que redibujan la pantalla (opencode,
  codex…) es la última línea que pintaron, que no siempre es la más informativa.
- `agenterm notify --activity "texto"` también la fija (como `hook`).
- El daemon emite `session.activity` agrupado: como mucho uno cada 500 ms por sesión (siempre con el
  último valor), más un latido cada 5 s mientras haya salida.

### Cronómetros

`turnStartedAt` se fija al pasar a `running` desde `idle` (o al responder tú a una espera detectada
por la terminal) y en cada `UserPromptSubmit`; una aprobación (espera explícita → `running`)
continúa el mismo turno. La app muestra "trabajando 4m 12s", "última salida hace 12s" (a partir de
10 s) y, en reposo, "terminó en 6m".

### «¿Trabado?»

Si un turno abierto **por un hook** (`running` explícito) pasa `activity.stall_seconds` (180 por
defecto) sin salida y sin eventos, la sesión se marca `stalled` (evento `session.activity` con
`stallRaised: true`) y la app lo muestra en naranja: "¿trabado? sin actividad hace 5m". Se borra con
cualquier salida o evento.

- Con `notify.notify_on_stall` se envía una notificación de tipo `stall`: sin sonido, sin rebote
  del Dock, no cuenta en el badge ni como pendiente. Con la app cerrada va por el mismo camino que
  el resto de avisos del daemon.
- Las sesiones heurísticas no tienen "turno abierto" (3 s sin salida ya es `idle`), así que nunca
  se marcan.
- El spinner de Claude Code redibuja cada segundo mientras trabaja y eso cuenta como salida: en
  Claude el aviso detecta sobre todo cuelgues completos (proceso parado, terminal congelada), no una
  herramienta lenta.

### Progreso

`OSC 9 ; 4 ; st ; pr` (ConEmu / Windows Terminal / Ghostty; `st`: 0 borra, 1 normal, 2 error, 3
indeterminado, 4 pausa; `pr`: 0-100) se muestra como una barra fina bajo la fila de la sesión y
arriba del panel de terminal. Nunca levanta atención.

### Vista general (⌘0)

El botón ▦ de la cabecera de la sidebar (o el de cada proyecto) abre una rejilla con las sesiones
del proyecto actual (con un interruptor para ver todos los proyectos): perfil, estado, atención,
actividad, cronómetros, progreso, rama y las últimas 8 líneas de salida en texto plano (RPC
`session.tail`, refrescadas al llegar eventos y como mucho cada 1,5 s por sesión). Pulsar una
tarjeta abre la sesión; Esc o ⌘0 la cierran.
