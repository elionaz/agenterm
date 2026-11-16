# agenterm

Workspace de escritorio para agentes de código (Claude Code, codex, opencode, Gemini, Cursor,
Copilot, aider, shell…),
organizado por proyecto. **Fase 1**: daemon + PTYs + CLI. **Fase 2**: app de escritorio
macOS (Wails v2 + Svelte + xterm.js) en `app/`, que es un cliente más del daemon
(`internal/client`). **Fase 3**: detección de "necesita tu atención" (hooks de los agentes +
señales de terminal) y notificaciones nativas, badge del Dock y sidebar. **Fase 4**: visibilidad de la
actividad — qué está haciendo cada agente ahora mismo, cuánto lleva, si parece trabado, barras de
progreso y una vista general en rejilla. **Fase 5**: git — cada sesión puede ir en su propio
**worktree** (rama y carpeta aparte) y la app muestra sus **cambios** (diff con diff2html) con acciones
de merge, editor y borrado seguro.

## Uso

```sh
go build -o agenterm ./cmd/agenterm

agenterm daemon --detach            # opcional: cualquier comando lo arranca si no corre
agenterm project add ~/Dev/agenterm # -> <projectId>  name  path
agenterm project list
agenterm profiles                   # catálogo + profiles.yaml, con instalado/activado
agenterm profiles scan              # re-detecta los agentes instalados
agenterm spawn <projectId|nombre> shell   # -> <sessionId>
agenterm spawn --worktree [--branch b] [--base main] <proyecto> claude   # -> <id> <rama> <carpeta>
agenterm spawn --in-worktree <worktreeId> <proyecto> shell   # reabrir un worktree existente
agenterm diff [--stat] <sessionId|worktreeId>   # diff unificado, o resumen por archivo
agenterm worktrees <proyecto>       # worktrees de agenterm: live / exited / orphan
agenterm worktree rm [--force] [--delete-branch] [--dry-run] <id>
agenterm worktree merge <id>        # merge --no-ff en la carpeta principal
agenterm worktree open [--finder] <id>
agenterm ls                         # con TURN (cronómetro del turno), GIT (rama ±N ↑N) y ACTIVITY
agenterm tail [-n 8] <sessionId>    # últimas líneas de salida en texto plano
agenterm attach <sessionId>         # Ctrl-\ para desacoplar; la sesión sigue viva
agenterm notify --kind permission --message "¿Borro node_modules?"  # dentro de una sesión
agenterm ack <sessionId>            # marca la atención como vista
agenterm kill <sessionId>           # una segunda vez la elimina de la lista
agenterm stop                       # apaga el daemon si no quedan sesiones vivas
agenterm stop --force               # mata las sesiones vivas y apaga el daemon
```

`agenterm stop` **se niega** (y sale con 1) si hay sesiones sin terminar: `hay N sesiones activas:
<id> (<perfil>), …; usa --force para cerrarlas`. La comprobación la hace el propio daemon
(`daemon.shutdown` con `{"force": true|false}`; sin parámetros = sin force), así que también protege
de clientes antiguos o scripts. Las sesiones ya terminadas (`exited`) no cuentan. SIGTERM/SIGINT al
daemon lo siguen apagando siempre (es el sistema quien lo pide). Cada petición de apagado, aceptada o
rechazada, queda en `daemon.log` con quién la hizo: `peer_pid`, `peer_cmd`, `peer_ppid` y
`peer_parent_cmd` (PID del otro extremo del socket con `LOCAL_PEERPID` en macOS / `SO_PEERCRED` en
Linux, y su línea de comandos vía `ps`), además de `force` y `live`.

`notify` acepta `--state running|waiting_input|idle` (alias `waiting`, `done`), `--kind
permission|question|done|error` (implica el estado si no se da), `--message`, `--activity "texto"`
(la línea de actividad; sin `--state` solo actualiza eso) y `--sock RUTA`. Usa
`$AGENTERM_SESSION` y el socket según la regla de abajo (o `--sock`), nunca arranca el daemon y tiene
timeout de 3 s.
`notify --from-hook claude|codex` lee el payload del hook (stdin, o último argumento en codex),
no imprime nada y **siempre** sale con 0 (timeout 2 s): nunca bloquea ni rompe al agente.

### Rutas

| | por defecto | con `AGENTERM_HOME=/x` |
|---|---|---|
| socket, pid, log | `~/.agenterm/{d.sock,d.pid,daemon.log}` | `/x/…` |
| perfiles, proyectos, ajustes | `~/.config/agenterm/{profiles.yaml,projects.json,settings.yaml}` | `/x/…` |
| archivos de hooks generados | `~/.agenterm/hooks/` | `/x/hooks/` |
| worktrees de las sesiones | `~/.agenterm/worktrees/<proyecto>-<id>/<sesión>/` (o `git.worktree_dir`) | `/x/worktrees/…` |
| registro de worktrees | `~/.agenterm/worktrees.json` | `/x/worktrees.json` |

Ojo: en macOS la ruta de un unix socket tiene un límite de ~104 bytes.

**Qué daemon usa un comando.** Si `AGENTERM_HOME` está definido, **manda siempre** (su `d.sock`);
`$AGENTERM_SOCK` (que el daemon inyecta en sus sesiones) solo se usa si `AGENTERM_HOME` no está
definido; si no hay ninguno, `~/.agenterm/d.sock`. Así `AGENTERM_HOME=/tmp/x agenterm stop` lanzado
desde dentro de una sesión va a `/tmp/x` y no al daemon real. Con solo `$AGENTERM_SOCK` (dentro de una
sesión) la CLI nunca arranca otro daemon. Para que todo sea coherente, el daemon inyecta en cada
sesión `AGENTERM_SOCK=<su socket>` y, si usa `AGENTERM_HOME`, también `AGENTERM_HOME=<su dir>` (si
usa las rutas por defecto, quita cualquier `AGENTERM_HOME` heredado). Los hooks generados además
llaman a `notify --sock <socket del daemon>`, así que llegan a su daemon aunque cambies
`AGENTERM_HOME` dentro de la sesión.

### Agentes y perfiles

agenterm trae un **catálogo** de CLIs de agentes conocidos y detecta cuáles tienes instalados
(`exec.LookPath` contra el `PATH` del daemon más el de tu shell de login, cacheado 5 min;
`agenterm profiles scan` o "Re-escanear" en la app lo refrescan):

| id | agente | binario | notas |
|---|---|---|---|
| `claude` | Claude Code | `claude` | `status: hooks`, `setup_hooks: claude` |
| `codex` | Codex CLI | `codex` | `setup_hooks: codex` |
| `opencode` | opencode | `opencode` | |
| `gemini` | Gemini CLI | `gemini` | |
| `cursor-agent` | Cursor CLI | `cursor-agent` | |
| `copilot` | GitHub Copilot CLI | `copilot` | |
| `aider` | Aider | `aider` | patrón de atención `(Y)es/(N)o` |
| `amp` | Amp | `amp` | |
| `goose` | goose | `goose session` | |
| `qwen` | Qwen Code | `qwen` | |
| `crush` | Crush | `crush` | |
| `kiro` | Kiro CLI | `kiro-cli chat` | nombre sin verificar |
| `shell` | Shell | `$SHELL` | siempre disponible |

Los agentes no instalados no salen en el menú **+** (sí en Ajustes, con la pista de instalación).

`profiles.yaml` es la fuente de verdad y se **superpone** al catálogo por `id`:

- una entrada con el id de un agente del catálogo solo cambia los campos que pongas; el resto
  (incluido `setup_hooks`) se hereda. Un `profiles.yaml` antiguo sigue valiendo tal cual y ahora
  hereda la integración de hooks. `setup_hooks: none` la desactiva; `args: []` vacía los
  argumentos heredados;
- un id nuevo es un perfil propio (`cmd` obligatorio) o una **variante** (`extends: <id>`: hereda
  todo del padre, ya con sus cambios; `args`/`env`/patrones **reemplazan** a los del padre);
- `enabled: false` lo oculta del menú; `name:` es el nombre visible;
- orden: el de la lista (o `order: N` explícito); los del catálogo que no aparecen van detrás. El
  primero visible es el de ⌘N;
- un error en una entrada (regex inválida, `extends` desconocido…) solo inutiliza esa entrada.

```yaml
- id: claude-yolo           # variante
  name: Claude (sin permisos)
  extends: claude
  args: ["--dangerously-skip-permissions"]
- id: claude
  env: {CLAUDE_CODE_USE_BEDROCK: "1"}
- id: gemini
  enabled: false
- id: mio                   # perfil propio
  cmd: $HOME/bin/mi-agente  # se expanden $VARS
  args: []
  env: {}
  status: heuristic         # hooks | heuristic
  setup_hooks: none         # claude | codex | none
  idle_seconds: 3           # opcional, sobreescribe el timeout heurístico
  attention_patterns:       # opcional: regex sobre la salida reciente (sin ANSI)
    - '\(y/n\)'
```

Al guardar desde la app (o con las RPC `profile.save|delete|reorder`) el archivo se reescribe en
formato canónico: una entrada por perfil en el orden del menú, solo con los campos que difieren de
lo heredado (los del catálogo sin cambios quedan como `- id: x`). La escritura es atómica y deja
el archivo anterior en `profiles.yaml.bak`; los comentarios no se conservan. Se relee en cada
`spawn`/`list`, así que también puedes editarlo a mano.

```sh
agenterm profiles        # ID NAME ENABLED INSTALLED(ruta) SOURCE(catalog|override|custom) STATUS CMD
agenterm profiles scan   # vuelve a leer el PATH del shell de login y detecta de nuevo
```

### Ajustes en la app

⚙ en la sidebar, **⌘,** o *agenterm → Preferencias…* abren **Ajustes**:

- **Agentes**: lista con ✓ instalado / no encontrado (+ comando de instalación y enlace), interruptor
  de activado, arrastrar o ▲▼ para reordenar (el primero es el de ⌘N), *Editar* (nombre, comando,
  argumentos, entorno `CLAVE=valor`, modo de estado, integración de hooks, segundos de inactividad,
  patrones de atención), *Duplicar como variante*, *+ Agente personalizado*, *Borrar* (propios) o
  *Restablecer* (los del catálogo modificados), con confirmación en línea, y *Re-escanear*.
- **Notificaciones**: los interruptores de `settings.yaml` (incluido *Avisar si parece trabado*) y
  los segundos sin actividad para el aviso de «¿trabado?», con *Guardar*.
- **Git**: modo por defecto al lanzar (misma carpeta / worktree), editor para «Abrir en editor» y
  carpeta de worktrees (ver fase 5).

Los menús de **+** y ⌘N usan solo los perfiles activados e instalados, en ese orden.

## Atención y notificaciones (fase 3)

Cada sesión tiene, además del estado, una **atención**: `attentionKind`
(`permission` | `question` | `done` | `error`), `attentionMessage`, `attentionAt` y `unread`.
`unread` se borra al seleccionar la sesión en la app (RPC `session.ack`), al escribir en ella o
con `agenterm ack`. Un `idle` explícito tras un turno (Stop / turn-complete) es un `done`; el
`idle` heurístico (3 s sin salida) **no** avisa.

### De dónde salen las señales

| agente | mecanismo (solo en sesiones lanzadas por agenterm) | señales |
|---|---|---|
| Claude Code | `claude --settings ~/.agenterm/hooks/claude-settings.json` (generado). Los hooks de `--settings` se **suman** a los tuyos | `UserPromptSubmit`/`PreToolUse`/`PostToolUse` → trabajando (y línea de actividad, ver fase 4); `PermissionRequest` y `Notification` (`permission_prompt`, `elicitation_dialog`, `agent_needs_input`) → aprobación/pregunta con mensaje; `Stop` → terminó (con el último mensaje); `StopFailure` → error |
| codex | `codex -c notify=[…] -c tui.notification_method="osc9" -c tui.notification_condition="always" -c tui.notifications=[…]` | `notify` (`agent-turn-complete`) → terminó; las notificaciones OSC 9 del TUI (aprobaciones, preguntas) → las recoge el parser de terminal |
| opencode y cualquier otro | — | BEL y notificaciones de terminal (abajo) + `attention_patterns` |

Nunca se tocan `~/.claude/settings.json`, `~/.codex/config.toml`, la config de opencode ni el
proyecto: todo va por flags en la línea de comandos y archivos en `~/.agenterm/hooks/`. Los hooks
llaman a `<ejecutable del daemon> notify --sock <socket> --from-hook …` con ruta absoluta (la CLI o
el propio ejecutable del `.app`, que también implementa `notify`).

**Señales de terminal** (agnósticas del agente; los bytes llegan intactos a la terminal): BEL
suelto (no el que cierra un OSC), `OSC 9 ; mensaje` (iTerm2/Ghostty; `OSC 9;4` es **progreso**, ver fase 4, y nunca pide
atención; el resto de subcomandos numéricos de ConEmu se ignoran), `OSC 777 ; notify ; título ; cuerpo` y
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
- **App cerrada**: el daemon avisa cuando ningún cliente UI está suscrito (`events.subscribe` con
  `{"client":"ui"}`). Si el ejecutable del daemon está dentro de un `.app` (ruta con
  `.app/Contents/MacOS/`, es decir, lo arrancó la app) y el bundle tiene permiso, usa
  UNUserNotificationCenter (`internal/macnotify`, el mismo código que la app): la notificación sale
  como **agenterm**, con su icono, `userInfo.sessionId` y el mismo identificador que usa la app
  (`agenterm-<id>`, así que al hacer *ack* en la app se retira también). Al pulsarla, el daemon (que
  sirve el run loop principal de Cocoa en el hilo principal mientras el resto corre en otras
  goroutines) ejecuta `open -a <su .app> [--env AGENTERM_HOME=…] agenterm://session/<id>`: la app se
  abre (o se activa) y selecciona esa sesión (esquema `agenterm://` registrado en `CFBundleURLTypes`
  vía `wails.json` → `protocols`, recibido con `mac.Options.OnUrlOpen`). Si el daemon no está en un
  bundle (la CLI), no hay permiso o falla el envío, cae a `osascript` (`display notification`, a
  nombre de "Script Editor" y sin abrir la app). `daemon.log` dice en cada aviso qué se usó
  (`notifier=native|osascript`, o `auth=denied|not-determined` si no hay permiso) y registra los
  clics (`notification clicked`). Nada de esto bloquea el daemon: el envío nativo tiene timeout de
  3 s, `osascript` de 5 s, todo corre en su goroutine y con `recover`.
- **Permiso**: la primera vez que abras el `.app` macOS pregunta si permites notificaciones. Si lo
  rechazaste: *Ajustes del Sistema → Notificaciones → agenterm* → "Permitir notificaciones" (y
  estilo "Alertas" si quieres que se queden en pantalla). Si la app no tiene permiso, cae a
  `osascript` (para esas: *Notificaciones → Script Editor*).

### `settings.yaml`

```yaml
notify:
  enabled: true          # notificaciones (app y daemon)
  sound: true            # sonido en aprobación/pregunta
  notify_on_done: true   # avisar también cuando un agente termina
  notify_on_stall: true  # aviso discreto si un turno parece trabado (sin sonido)
activity:
  stall_seconds: 180     # segundos sin salida ni eventos para marcar «¿trabado?»; 0 lo desactiva
git:
  editor: ""             # "code", "cursor -n", "Visual Studio Code"…; vacío: code si existe, si no Finder
  worktree_dir: ""       # dónde crear worktrees (ruta absoluta o ~/…); vacío: ~/.agenterm/worktrees
  default_mode: same     # same | worktree: lo que propone el menú + (y ⌘N) si no elegiste antes
```

Todo está activado por defecto; el archivo es opcional y se relee en cada aviso (`stall_seconds`
se lee al lanzar cada sesión). La app lo edita desde *Ajustes → Notificaciones* (RPC
`settings.save`, escritura atómica con `.bak`). Al guardar se edita el archivo en su sitio: se
**conservan** las claves desconocidas y los comentarios, y los campos que un cliente no envía
mantienen su valor actual.

## Actividad (fase 4)

"Trabajando" ya no significa solo "hubo salida en los últimos 3 s". Cada sesión lleva, en
`session.list` y en los eventos:

| campo | qué es |
|---|---|
| `activity`, `activityAt`, `activitySource` | línea de actividad y de dónde sale: `hook` u `output` |
| `turnStartedAt`, `turnEndedAt` | inicio del turno actual (o último) y su fin (al pasar a `idle`/`exited`) |
| `lastOutputAt`, `lastEventAt` | última salida del PTY y último evento explícito (hook / `notify`) |
| `stalled`, `stalledAt` | el turno parece trabado (es una marca, no un estado) |
| `progress` | `{state: normal\|error\|indeterminate\|paused, value: 0-100}` (OSC 9;4) |

**Línea de actividad.**

- *Claude Code*: el archivo de hooks generado incluye `PreToolUse` (sin `matcher`: todas las
  herramientas; timeout 3 s). `notify --from-hook` no imprime nada y sale con 0, así que **nunca**
  emite una decisión de permisos: la llamada sigue el flujo normal de aprobación de Claude.
  `tool_name` + `tool_input` se traducen así: `Edit`/`Write`/`MultiEdit`/`NotebookEdit` →
  "Editando main.go", `Read` → "Leyendo go.mod", `Bash` → "Bash: <comando, ~60 caracteres>",
  `Grep`/`Glob` → "Buscando <patrón>", `WebFetch` → "Web: <host>", `WebSearch` → "Web: <consulta>",
  `Task`/`Agent` → "Subagente: <descripción>", `TodoWrite` → "Actualizando tareas", `mcp__srv__tool`
  → "MCP: srv.tool"; cualquier otra, su nombre. `UserPromptSubmit` → "Pensando…" (y reinicia el
  cronómetro del turno). Con hooks, la salida no pisa la actividad.
- *Resto de agentes*: la última línea con contenido de la salida (sin ANSI, el texto que ya extrae el
  parser de terminal), recortada, con un CR que redibuja la línea quedándose con su última versión,
  ignorando líneas solo de spinner, de dibujo de cajas, con menos de dos letras/dígitos o que parecen el
  **prompt de la shell** (cortas y terminadas en `$ % # > ❯ ➜`, con el símbolo suelto, tras una
  palabra tipo directorio, tras `~ / \ ] ) }`, tras `bash-5.2`, o empezando por `usuario@host`; o
  empezando por `➜`; "45%" o "<div>" no cuentan), así que en una shell se ve la última salida y no
  `usuario@mac ~ %`; y como mucho
  una actualización cada 500 ms. En TUIs que redibujan la pantalla (opencode, codex…) será la última
  línea que pintaron, que no siempre es la más informativa.
- El daemon emite `session.activity` agrupado: como mucho uno cada 500 ms por sesión (siempre con el
  último valor), más un latido cada 5 s mientras haya salida (para refrescar "última salida").

**Cronómetros.** `turnStartedAt` se fija al pasar a `running` desde `idle` (o al responder tú a una
espera detectada por la terminal) y en cada `UserPromptSubmit`; una aprobación (espera explícita →
`running`) continúa el mismo turno. La app muestra "trabajando 4m 12s", "última salida hace 12s"
(si pasan ≥ 10 s) y, en reposo, "terminó en 6m".

**¿Trabado?** Si un turno abierto **por un hook** (`running` explícito) pasa `activity.stall_seconds`
sin salida y sin eventos, la sesión se marca `stalled` (evento `session.activity` con
`stallRaised: true`) y la app lo muestra en naranja: "¿trabado? sin actividad hace 5m". Se borra con
cualquier salida o evento. Con `notify.notify_on_stall` se envía una notificación de tipo `stall`:
sin sonido, sin rebote del Dock, no cuenta en el badge ni como pendiente (por el mismo camino que el
resto; con la app cerrada, el daemon vía `osascript`). Las sesiones heurísticas no tienen "turno
abierto" (3 s sin salida ya es `idle`), así que nunca se marcan. Ojo: el spinner de Claude Code
redibuja cada segundo mientras trabaja y eso cuenta como salida, así que en Claude el aviso detecta
sobre todo cuelgues completos (proceso parado, terminal congelada), no una herramienta lenta.

**Progreso.** `OSC 9 ; 4 ; st ; pr` (ConEmu / Windows Terminal / Ghostty; `st` 0 borra, 1 normal,
2 error, 3 indeterminado, 4 pausa; `pr` 0-100) se interpreta como progreso: barra fina bajo la fila
de la sesión y arriba del panel de terminal. Nunca levanta atención. Los bytes siguen llegando
intactos a xterm.

**Vista general (⌘0).** El botón ▦ de la cabecera de la sidebar (o el de cada proyecto) abre una
rejilla con las sesiones del proyecto actual (interruptor "todos los proyectos"): perfil, estado,
atención, actividad, cronómetros, progreso y las últimas 8 líneas de salida en texto plano (RPC
`session.tail`, refrescadas al llegar eventos y como mucho cada 1,5 s por sesión; sin instancias
extra de xterm). Pulsar una tarjeta abre la sesión; Esc o ⌘0 la cierran.

**Sidebar.** Bajo el nombre del perfil: la actividad (recortada, tenue; completa en el tooltip) y el
cronómetro del turno.

## Git: worktrees y cambios (fase 5)

Todo va por la CLI de `git` (con timeouts; nunca libgit2). Las consultas de solo lectura usan
`GIT_OPTIONAL_LOCKS=0`, así que nunca toman `index.lock` mientras el agente usa git en la misma
carpeta.

### Modos de una sesión

- **Misma carpeta** (`same`, por defecto): el agente trabaja en la carpeta del proyecto.
- **Worktree nuevo** (`worktree`): `git worktree add <worktrees>/<proyecto>-<id>/<sesión> -b <rama> <base>`.
  La rama por defecto es `agent/<perfil>-<4 primeros del id>` (se sanea: espacios y caracteres raros →
  `-`, sin `..`, etc.; si existe se le añade `-2`, `-3`…). La base por defecto es la rama actual del
  proyecto (o `main`/`master` si HEAD está separado). Los worktrees viven en el directorio de
  agenterm, **nunca dentro del proyecto**. Se permite aunque la carpeta principal tenga cambios: el
  worktree parte del commit de la base. Si el proyecto es una subcarpeta del repo, la sesión arranca
  en esa misma subcarpeta del worktree. Errores claros si el proyecto no es un repo git (la opción sale
  desactivada en la app), si el repo no tiene commits o si la base no existe.
- Las integraciones de hooks funcionan igual en un worktree: Claude Code recibe
  `--settings ~/.agenterm/hooks/claude-settings.json` (ruta absoluta) y no se escribe nada en el
  worktree ni en el proyecto. Claude Code puede pedir confianza la primera vez para la carpeta nueva.

**Matar una sesión nunca borra su worktree.** El registro (`worktrees.json`) sobrevive a reinicios
del daemon. Un worktree es *huérfano* cuando ninguna sesión del daemon lo usa (la quitaste de la
lista o el daemon se reinició); `worktree.list` además olvida los que ya no existen en disco (y hace
`git worktree prune`) y **adopta** los worktrees de agenterm que git conoce pero el registro no.

### Cambios (diff)

- Worktree: `git diff <merge-base con la base>` (lo commiteado + lo no commiteado, en una lista) más
  los archivos sin seguimiento no ignorados (`git ls-files --others --exclude-standard` + `git diff
  --no-index /dev/null <archivo>`), y los commits por delante (`git log <base>..HEAD`, máx. 200). Si
  la rama base desaparece se usa el commit base guardado al crear el worktree.
- Misma carpeta: `git diff HEAD` + sin seguimiento (árbol vacío si el repo no tiene commits).
- Por archivo: ruta, ruta anterior (renombrados), estado `A/M/D/R/C/T`, `+`/`−` (`--numstat`),
  binario (sin contenido) y su parche. Tope de **2 MB** de parche en total: el resto de archivos se
  listan sin contenido y el diff se marca `truncated`. Archivos sin seguimiento de más de 1 MB o más
  allá de 300 tampoco llevan contenido.
- **Resumen** en cada sesión (`git`: `files`, `uncommitted`, `ahead`): se recalcula de forma
  perezosa — al pasar a inactivo/esperando/terminado y al pedir el diff — como mucho una vez cada 3 s
  por sesión y nunca por cada trozo de salida. Cambios → evento `session.git`.

### Acciones

- **Merge a la base**: `git merge --no-ff` de la rama en la **carpeta principal** del repo, solo si
  está en la rama base y sin cambios en archivos con seguimiento (los sin seguimiento no bloquean; no
  hay stash automático). Si hay conflictos se hace `git merge --abort` y se informa. Los cambios sin
  commitear del worktree no entran (se avisa).
- **Eliminar worktree**: se niega si una sesión viva lo usa; sin *force* también si se perderían
  cambios sin commitear o (con *borrar también la rama*) commits sin mergear en la base, con un error
  que lista qué se perdería. `dryRun` solo lo comprueba. Tras borrar: `git worktree prune` y, si se
  pidió, `git branch -D` (seguro: los commits sin mergear ya se comprobaron contra la base).
- **Abrir en editor** (`git.editor`; por defecto `code` si está en el PATH, si no Finder), **Abrir en
  Finder** (`open <carpeta>`), **Copiar rama** (portapapeles, en la app).

### En la app

- Menú **+**: tras elegir el agente, *Misma carpeta* o *Worktree nuevo* (rama editable ya rellena y
  selector de base con la rama actual por defecto). La opción worktree sale desactivada, con el
  motivo, si el proyecto no es un repo git. Se recuerda la última elección por proyecto (y ⌘N la usa).
- Fila de la sesión: chip con la rama (sin `agent/`) y `±N` archivos cambiados; igual en las
  tarjetas de la vista general (con `↑N` commits por delante).
- Pestañas **Terminal | Cambios (N)** en sesiones con git: lista de archivos a la izquierda (estado,
  `+`/`−`), commits por delante, diff a la derecha con diff2html (lado a lado / unificado, tema oscuro
  de la app), ⟳, aviso de diff truncado y la barra de acciones (*Abrir en editor*, *Abrir en Finder*,
  *Copiar rama*, *Merge a <base>*, *Eliminar worktree* con confirmación en línea que enseña qué se
  perdería y casilla para forzar).
- Bajo cada proyecto, **Worktrees huérfanos** (si hay): *Ver cambios*, *Abrir shell aquí* (una
  sesión `shell` nueva en ese worktree) y *Eliminar*.

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
  reabrir se reconecta y repinta el scrollback. Para apagarlo todo: `agenterm stop --force` (sin `--force` se niega si quedan sesiones vivas). La CLI y la app comparten daemon.
- Antes de lanzar el daemon, la app toma el `PATH` de tu shell de login (`$SHELL -l -i -c`) y fija
  `TERM=xterm-256color`, `COLORTERM=truecolor` y `LANG` si falta: las apps abiertas desde Finder
  heredan el entorno mínimo de launchd y no encontrarían `claude`/`codex`.
- Atajos: ⌘0 abre/cierra la vista general, ⌘1…⌘9 seleccionan la sesión n del proyecto actual, ⌘N lanza una sesión con el primer
  perfil activado e instalado en el proyecto actual, ⌘, abre Ajustes, ⌘C/⌘V copian/pegan en la terminal.
- `AGENTERM_HOME` también vale para la app (útil para probar sin tocar `~/.agenterm`):
  `AGENTERM_HOME=/tmp/at wails dev`.

### Eventos Go → frontend

| evento | payload |
|---|---|
| `pty:<id>` | salida en base64, agrupada (≤ 8 ms o 32 KiB por evento) |
| `pty-replay-done:<id>` | fin del replay del scrollback |
| `pty-exit:<id>` | código de salida |
| `pty-error:<id>` | stream perdido (el frontend se reengancha y repinta) |
| `daemon:event` | `session.spawned/state/attention/activity/removed/git` y `worktree.changed` del daemon (con `attention*`, `unread`, `raised`, los campos de actividad y `git`) |
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
internal/session    PTY (creack/pty), ring buffer, fan-out, kill/wait, actividad (línea, progreso, tail)
internal/status     máquina de estados + atención + heurística (reloj inyectable)
internal/termsig    parser streaming de BEL / OSC 9 / 777 / 99 y progreso OSC 9;4
internal/hooks      config de hooks por sesión (claude, codex) y parseo de payloads
internal/notifycmd  subcomando `notify` compartido por la CLI y la app
internal/desktopnotify  textos de notificación + fallback osascript
internal/macnotify  UNUserNotificationCenter (cgo, darwin): envío, permiso, clics y run loop del daemon
internal/peercred   PID del proceso al otro lado del socket (LOCAL_PEERPID / SO_PEERCRED) + `ps`
internal/settings   settings.yaml
internal/gitx       git por CLI: repo, ramas, status, worktrees, diff, merge, abrir en editor
internal/worktree   registro de worktrees de agenterm (worktrees.json)
internal/profile    catálogo de agentes, detección, merge con profiles.yaml y escritura
internal/shellenv   PATH del shell de login (app y daemon)
internal/atomicfile escritura atómica con .bak
internal/project    registro de proyectos en JSON
internal/paths      resolución de directorios
```

### Protocolo

- **Control**: JSON delimitado por `\n`. Petición `{"id":1,"method":"session.list","params":{…}}`,
  respuesta `{"id":1,"result":{…}}` o `{"id":1,"error":"…"}`. Una conexión admite varias
  peticiones seguidas. Métodos: `project.add|list|remove`,
  `profile.list|catalog|save|delete|reorder|rescan`, `settings.get|save`,
  `session.spawn|list|kill|resize|notify|ack|attach|tail|diff`, `project.git`,
  `worktree.list|diff|remove|merge|open`, `events.subscribe`, `daemon.ping|shutdown`.
  `session.spawn` acepta `mode` (`same`|`worktree`), `branch`, `base` y `worktreeId` (reabrir); la
  info de sesión lleva `mode`, `worktreeId`, `worktreePath`, `branch`, `base` y `git`
  (`{files, uncommitted, ahead, at}`). `worktree.remove` toma `{id, force, deleteBranch, dryRun}` y
  devuelve `{removed, loss}`; `worktree.open` `{id | sessionId, target: editor|finder}`.
  `daemon.shutdown` (`{"force":false}`) falla con `hay N sesiones activas: …` si quedan sesiones
  vivas y no se pasa `force: true`.
  `session.tail` (`{"sessionId":…,"lines":8}`, máx. 200) devuelve `{"lines":[…]}`: el final del
  scrollback (últimos 64 KiB) sin ANSI, con CR sobrescribiendo desde la columna 0 y sin tandas de
  líneas en blanco.
- **`events.subscribe`** (`{"client":"ui"}` opcional): tras la respuesta, el daemon emite un evento
  JSON por línea (`session.spawned`, `session.state`, `session.attention`, `session.activity`,
  `session.removed`, `session.git` y `worktree.changed` con `projectId`). `raised: true` marca una atención nueva (lo que merece notificación);
  `stallRaised: true` (en `session.activity`) una sesión que acaba de quedarse "¿trabada?". Si el cliente se retrasa se le
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
- Se inyecta `AGENTERM_SESSION` y `AGENTERM_SOCK` en el entorno del proceso (y `AGENTERM_HOME` si el
  daemon usa uno; ver *Rutas*).
- Estados: `running`, `waiting_input`, `idle`, `exited`. Heurística: salida → `running`, 3 s
  sin salida → `idle`; `waiting_input` no lo termina la salida (spinners), sino lo que escribas.
  En perfiles `status: hooks`, el primer evento explícito desactiva la heurística para esa sesión.
  `exited` es terminal. Las respuestas automáticas de la terminal (focus, DA, DSR) no cuentan como
  "escribir".
- `kill` envía SIGHUP+SIGTERM al grupo de procesos y SIGKILL tras 2 s. SIGTERM/SIGINT al
  daemon (o `agenterm stop` sin sesiones vivas / `agenterm stop --force`) mata todas las sesiones y
  borra socket y pid file. Un socket
  huérfano (nadie escuchando) se detecta y se reemplaza al arrancar.

## Tests

```sh
go vet ./... && go test -race ./...
```

Incluye un test de integración que levanta el daemon en un `AGENTERM_HOME` temporal y ejercita
spawn, attach/detach/reattach con scrollback, notify, eventos, kill y apagado, y otro que lanza una
shell en modo worktree sobre un repo temporal, hace cambios escribiendo en ella, comprueba
`session.diff`, que `kill` conserva el worktree, el huérfano tras reiniciar el daemon, reabrirlo, el
merge y `worktree.remove` (rechazo y force). Los tests de `internal/gitx` usan repos reales en
directorios temporales con la configuración de git aislada (`GIT_CONFIG_NOSYSTEM=1`,
`GIT_CONFIG_GLOBAL` vacío, `HOME` temporal).

## Contribuir

Las contribuciones son bienvenidas vía pull request; lee [CONTRIBUTING.md](CONTRIBUTING.md).
Licencia: [MIT](LICENSE).
