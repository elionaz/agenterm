# Arquitectura

[English](../architecture.md) · **Español**

```
          ┌──────────────┐   ┌──────────────┐   ┌───────────────────────────┐
          │ panal.app │   │ panal CLI │   │ hooks de los agentes      │
          │ (Wails)      │   │              │   │ (`panal notify …`)     │
          └──────┬───────┘   └──────┬───────┘   └─────────────┬─────────────┘
                 │   socket unix (~/.panal/d.sock)          │
                 └──────────────────┬──────────────────────────┘
                             ┌──────┴──────┐
                             │   daemon    │  PTYs, estado, git, eventos
                             └──────┬──────┘
                 ┌──────────────────┼──────────────────┐
              claude              codex               shell …
```

- El **daemon** es el único dueño de las PTYs (las crea él mismo: no hay tmux ni screen debajo): lanza los procesos, guarda su scrollback, calcula
  estado, atención, actividad y git, y publica eventos. Escucha en un socket unix (`0600`).
- La **app** y la **CLI** son clientes (`internal/client`). Cualquiera de los dos arranca el daemon
  si no responde (`client.StartDaemon`: proceso desacoplado en su propia sesión, salida a
  `daemon.log`). Cerrar la app o desacoplar `attach` no afecta a las sesiones.
- El `.app` es autosuficiente: su ejecutable también implementa `daemon` y `notify`
  (`panal.app/Contents/MacOS/panal daemon`), así que no hace falta la CLI.
- Antes de lanzar el daemon, la app toma el `PATH` de tu shell de login (`$SHELL -l -i -c`, con
  timeout de 3 s) y fija `TERM=xterm-256color`, `COLORTERM=truecolor`, `TERM_PROGRAM=panal` y
  `LANG` si falta: las apps abiertas desde Finder heredan el entorno mínimo de launchd y no
  encontrarían `claude` o `codex`. El daemon, a su vez, busca los agentes en su `PATH` más el del
  shell de login.

## Paquetes

```
cmd/panal            CLI + subcomandos `daemon` y `notify` (un solo binario)
app/                    app Wails: bindings (app.go), agrupado de salida, atención, entorno del daemon
app/frontend            Svelte 5 + TypeScript + xterm.js (WebGL, fit, unicode11) + diff2html
internal/daemoncmd      subcomando `daemon` compartido por la CLI y la app (+ notificador de respaldo)
internal/notifycmd      subcomando `notify` compartido por la CLI y la app
internal/daemon         servidor: sesiones, RPC, attach, eventos, git, apagado, pid/socket
internal/client         cliente reutilizable + StartDaemon + resolución del socket
internal/protocol       tipos del protocolo: JSON por líneas + frames binarios
internal/transport      Listen/Dial/Cleanup sobre socket unix
internal/session        PTY (creack/pty), ring buffer, fan-out, kill/wait, actividad, tail
internal/status         máquina de estados + atención + heurística + «¿trabado?»
internal/termsig        parser streaming de BEL / OSC 9 / 777 / 99 y progreso OSC 9;4
internal/hooks          configuración de hooks por sesión (claude, codex, gemini, opencode, cursor) y parseo de payloads
internal/profile        catálogo de agentes, detección, combinación con profiles.yaml y escritura
internal/settings       settings.yaml (lectura con valores por defecto y edición en su sitio)
internal/gitx           git por CLI: repo, ramas, status, worktrees, diff, merge, abrir en editor
internal/forge          pull requests en GitHub / GitLab con gh / glab (la sesión del usuario)
internal/mailbox        mensajes entre las sesiones de un proyecto (messages.json)
internal/worktree       registro de worktrees de panal (worktrees.json)
internal/project        registro de proyectos (projects.json)
internal/desktopnotify  textos de las notificaciones + respaldo con osascript
internal/macnotify      UNUserNotificationCenter (cgo, darwin): envío, permiso, clics, run loop
internal/peercred       PID del proceso al otro lado del socket (LOCAL_PEERPID / SO_PEERCRED) + `ps`
internal/shellenv       PATH del shell de login
internal/atomicfile     escritura atómica con .bak
internal/paths          resolución de directorios (PANAL_HOME)
```

## Protocolo

### Control

JSON delimitado por `\n`. Petición `{"id":1,"method":"session.list","params":{…}}`; respuesta
`{"id":1,"result":{…}}` o `{"id":1,"error":"…"}`. Una conexión admite varias peticiones seguidas.

| grupo | métodos |
|---|---|
| proyectos | `project.add`, `project.list`, `project.remove`, `project.reorder`, `project.update`, `project.git` |
| perfiles | `profile.list`, `profile.catalog`, `profile.save`, `profile.delete`, `profile.reorder`, `profile.rescan` |
| ajustes | `settings.get`, `settings.save` |
| sesiones | `session.spawn`, `session.list`, `session.kill`, `session.resize`, `session.notify`, `session.ack`, `session.rename`, `session.reorder`, `session.attach`, `session.tail`, `session.diff`, `session.resume`, `session.discard` |
| worktrees | `worktree.list`, `worktree.diff`, `worktree.remove`, `worktree.merge`, `worktree.pr`, `worktree.open` |
| mensajes | `msg.peers`, `msg.send`, `msg.list`, `msg.take`, `msg.ack`, `msg.deliver` (el remitente es la sesión en la que corre el proceso que llama; ver [messages.md](messages.md)) |
| daemon | `events.subscribe`, `daemon.ping`, `daemon.shutdown` |

`daemon.ping` devuelve la `version`, el `commit` y la versión de `protocol` del daemon; un método que
el daemon no conoce falla con el código `unknown_method`. Ver [compatibility.md](compatibility.md#protocolo-del-daemon).

Algunos parámetros:

- `session.spawn`: `projectId` (id o nombre), `profileId`, `cols`, `rows`, `mode` (`same` |
  `worktree`), `branch`, `base` y `worktreeId` (reabrir un worktree; implica `worktree`).
- La información de sesión incluye `mode`, `worktreeId`, `worktreePath`, `branch`, `base`, `git`
  (`{files, uncommitted, ahead, at}`), los campos de atención y los de actividad.
- `session.tail`: `{"sessionId":…,"lines":8}` (máximo 200) devuelve `{"lines":[…]}`: el final del
  scrollback (últimos 64 KiB) sin ANSI, con CR sobrescribiendo desde la columna 0 y sin tandas de
  líneas en blanco.
- `session.diff` / `worktree.diff`: `{sessionId | worktreeId, stat}`.
- `worktree.remove`: `{id, force, deleteBranch, dryRun}` → `{removed, loss}`.
- `worktree.open`: `{id | sessionId, target: "editor" | "finder"}`.
- `session.resume` / `session.discard`: `{sessionId}` o `{all: true}` → `{sessions, failed}`
  (`failed`: `[{sessionId, error, errorCode, errorData}]`, solo con `all`; una sola sesión falla
  con el error en sí). Solo para sesiones `suspended`.
- `session.notify` acepta además `agent`, `agentSessionId` y `transcriptPath` (los envía
  `notify --from-hook`): el id de sesión propio del agente, que se guarda para reanudar. Sin
  `state` ni `activity` solo se guarda eso.
- Las sesiones suspendidas añaden `suspendedAt`, `resumeMode` (`agent` | `args` | `fresh`) y, tras
  un intento fallido, `resumeError` / `resumeErrorCode`.
- `daemon.shutdown`: `{"force": false}` (sin parámetros = sin force); falla con un error que
  lista las sesiones vivas si quedan y no se pasa `force: true`.

Los mensajes de error de las RPC están en inglés. Los rechazos que ve el usuario llevan además un
`errorCode` estable y un `errorData` estructurado junto al mensaje `error` (los clientes antiguos
siguen funcionando), para que la app pueda mostrarlos en su idioma (ver
[configuration.md](configuration.md#idioma)):

| código | `errorData` |
|---|---|
| `live_sessions` | `{count, sessions: [{id, profileId}]}` |
| `worktree_dirty`, `worktree_unmerged` | `{path, branch, base, uncommitted, unmerged}` |
| `worktree_live` | `{sessionId}` |
| `worktree_not_found` | `{id}` |
| `merge_main_dirty` | `{dir, count}` |
| `merge_wrong_branch` | `{dir, current, base}` (`current` vacío = HEAD separado) |
| `merge_base_behind` | `{dir, base, upstream, count}` |
| `setup_untrusted` | `{repo, file, command}` |
| `merge_nothing` | `{branch, base}` |
| `merge_conflict` | `{branch, base, files}` |
| `branch_missing`, `branch_exists`, `branch_invalid` | `{branch}` |
| `profile_not_installed` | `{profileId, cmd, install}` |
| `not_a_repo` | `{dir, project?}` |
| `no_commits` | `{dir}` |
| `base_missing` | `{base}` (vacío = no se pudo deducir una base) |
| `dir_missing` | `{dir}` |
| `resume_failed` | `{sessionId, reason, pid?}`; `reason`: `still_running`, `project_missing`, `not_suspended`, `busy` |

Para códigos desconocidos se muestra el texto `error` en inglés. La lista está en `internal/apierr/apierr.go`.

### Eventos

`events.subscribe` (con `{"client":"ui"}` opcional): tras la respuesta, el daemon emite un evento
JSON por línea.

| evento | |
|---|---|
| `session.spawned` | sesión nueva |
| `session.state` | cambio de estado (con atención y actividad) |
| `session.attention` | cambió la atención sin cambiar el estado (mensaje completado, *ack*) |
| `session.activity` | línea de actividad, progreso, «¿trabado?» o latido de salida |
| `session.git` | nuevo resumen de git (`git`) |
| `session.renamed` | cambió el nombre de la sesión (`sessionName`; ausente: vuelve al del perfil) |
| `session.removed` | la sesión se quitó de la lista |
| `message.changed` | se enviaron o entregaron mensajes de o para `sessionId` (en `projectId`); `msgPending` es cuántos esperan a esa sesión |
| `worktree.changed` | cambió la lista de worktrees de un proyecto (`projectId`) |
| `session.reordered` | se reordenaron las sesiones de `projectId` (`session.list` trae el orden nuevo) |
| `project.changed` | cambió la lista de proyectos: se añadió, quitó, reordenó o renombró uno, o cambió su insignia |

`raised: true` marca una atención nueva (la que merece notificación); `stallRaised: true` (en
`session.activity`), una sesión que acaba de quedarse «¿trabada?». Si un suscriptor se retrasa, se
le desconecta: debe volver a suscribirse y pedir `session.list`. Mientras haya al menos un
suscriptor `ui`, el daemon deja las notificaciones a la app.

### Attach

`session.attach` (`{sessionId, cols, rows}`): tras la respuesta JSON, la conexión pasa a frames
binarios `tipo (1 byte) | longitud (4 bytes BE) | payload` (máximo 1 MiB):

- daemon → cliente: `0x01` salida, `0x02` fin del replay, `0x03` exit (int32 BE), `0x04` error;
- cliente → daemon: `0x10` input, `0x11` resize (cols, rows uint16 BE).

El daemon envía primero el snapshot del ring buffer y después la salida en vivo. Snapshot y
suscripción se toman bajo el mismo lock, así que no hay huecos ni duplicados.

Al acoplarse a una sesión `suspended` se reproduce su scrollback guardado más una línea tenue
«suspendida desde …» y el stream queda abierto sin salida (la entrada se ignora). Cuando la sesión
se reanuda, el stream termina con un frame de error `session resumed; reattach` (la app se vuelve a
acoplar sola y recibe el proceso nuevo); cuando se descarta, con `session discarded`.

### Eventos Go → frontend (app)

| evento | payload |
|---|---|
| `pty:<id>` | salida en base64, agrupada (cada 8 ms o 32 KiB) |
| `pty-replay-done:<id>` | fin del replay del scrollback |
| `pty-exit:<id>` | código de salida |
| `pty-error:<id>` | stream perdido (el frontend se reengancha y repinta) |
| `daemon:event` | cualquier evento del daemon (tabla de arriba) |
| `daemon:resync` | (re)suscrito: recargar listas |
| `daemon:down` | sin conexión con el daemon (se reintenta y se relanza) |
| `app:focus-session` | se pulsó una notificación: seleccionar esa sesión |
| `app:open-settings` | menú *panal → Preferencias…* |

## Sesiones

- Cada sesión tiene un ring buffer de scrollback (8 MiB) y N suscriptores con cola acotada; un
  suscriptor lento se desconecta (frame `0x04`) en vez de bloquear la lectura del PTY.
- `kill` envía SIGHUP y SIGTERM al grupo de procesos y SIGKILL a los 2 s. Un segundo `kill` sobre
  una sesión terminada la quita de la lista.
- Los ids de proyectos y sesiones son 4 bytes aleatorios en hexadecimal.

### Entorno de las sesiones

El daemon inyecta en cada sesión:

- `PANAL_SESSION=<id>`;
- `PANAL_SOCK=<su socket>`;
- `PANAL_HOME=<su directorio>` si el daemon usa `PANAL_HOME`; si usa las rutas por defecto,
  **quita** cualquier `PANAL_HOME` heredado.

Los clientes dan prioridad a `PANAL_HOME` sobre `PANAL_SOCK` (ver
[cli.md](cli.md#qué-daemon-usa-un-comando)), así que ambos tienen que apuntar al mismo daemon. Los
hooks generados pasan además `--sock <socket del daemon>`, así que llegan a su daemon aunque cambies
`PANAL_HOME` dentro de la sesión.

## Persistencia y reanudación

| sobrevive a… | cerrar la app | reiniciar el daemon / la computadora |
|---|---|---|
| procesos de las sesiones | sí | **no** (se relanzan al reanudar) |
| lista de sesiones y scrollback | sí | sí (como sesiones `suspended`) |
| la conversación del agente | sí | claude, codex, gemini, opencode y Cursor CLI: sí (se reanudan por id); otros: con `resume_args` |
| proyectos (`projects.json`), perfiles, ajustes | sí | sí |
| worktrees y su registro (`worktrees.json`) | sí | sí |

Los procesos viven en el daemon: si se detiene (`panal stop --force`, SIGTERM, un fallo,
reiniciar la computadora) mueren. Lo que el daemon siguiente necesita para traerlos de vuelta se
guarda en disco, en el directorio de runtime:

- **`sessions.json`**: un registro por sesión — `id`, `projectId`, `profileId`, `name` (el nombre puesto, si lo hay), `mode`,
  `worktreeId`, `cwd`, `cmd`/`args` tal como se lanzaron (se muestran mientras está suspendida),
  `pid`, tamaño, `createdAt`, el último `state`, `suspendedAt` y `resume`
  (`{agent, sessionId, transcriptPath, at}`). El entorno resuelto **nunca** se guarda (los perfiles
  pueden poner secretos en `env`): al reanudar se vuelve a resolver el perfil desde
  `profiles.yaml`. Se escribe al lanzar, al terminar, al quitar y cuando el agente informa su id de
  sesión; de forma atómica, `0600`. Un archivo que no se puede leer se aparta
  (`sessions.json.corrupt-<hora>`, queda en el log) y el daemon arranca sin sesiones que restaurar.
- **`scrollback/<id>.log`**: el ring buffer de la sesión (8 MiB como máximo, la capacidad del
  ring), reescrito de forma atómica (`0600`) como mucho cada 2 s mientras hay salida nueva, y una
  vez más cuando el daemon se detiene (antes de matar las sesiones).

Una sesión que termina, o que matas, pierde ambos (no se restaura). Al arrancar, las sesiones que
estaban vivas cuando se detuvo el daemon anterior vuelven como **`suspended`**: aparecen en la
lista, con su scrollback, pero sin proceso. Luego decide `sessions.restore`
([configuration.md](configuration.md#settingsyaml)): `ask` (por defecto) las deja suspendidas hasta
que eliges **Reanudar** o **Descartar** (en la fila de la barra lateral o en el aviso «N sesiones de
la ejecución anterior · Reanudar todas / Descartar todas»; CLI `panal resume|discard <id>|--all`),
`always` las reanuda enseguida y `never` las descarta.

**Reanudar** relanza el perfil en la misma carpeta — el mismo worktree en las sesiones de worktree
— con el mismo id, tamaño y fecha de creación. La terminal nueva empieza con el scrollback
guardado, los modos que dejó activos un programa de pantalla completa reiniciados (pantalla
alternativa, ratón, cursor oculto…) y una línea tenue `── sesión restaurada · <hora> ──` que el
daemon escribe en el scrollback en el idioma de `ui.language` (una línea ANSI normal en vez de una
marca del protocolo: todos los clientes —app, `attach` de la CLI, `tail`— la muestran sin cambios).
Cómo se relanza el agente (`resumeMode`):

| perfil | se relanza como | `resumeMode` |
|---|---|---|
| `setup_hooks: claude` con id de sesión conocido | `claude --settings <hooks> --resume <session_id> <args>` | `agent` |
| `setup_hooks: codex` con thread id conocido | `codex resume -c notify=… <args> <thread-id>` | `agent` |
| `setup_hooks: gemini` / `cursor` con id de sesión (chat) conocido | `<cmd> <hooks> --resume <id> <args>` | `agent` |
| `setup_hooks: opencode` con id de sesión conocido | `opencode --standalone --session <id> <args>` | `agent` |
| con `resume_args` (opencode, Cursor CLI: `--continue`) | `<cmd> <hooks> <args> <resume_args>` | `args` |
| cualquier otro (shell, agentes aún sin id de sesión) | `<cmd> <hooks> <args>`: un proceso nuevo | `fresh` |

El `session_id` de Claude llega con cada payload de hook (panal escucha también `SessionStart`,
así que se conoce desde el principio y sigue a `/clear`); codex informa su `thread-id` en el
payload de `notify`, es decir, tras su primer turno. Ambos flags se comprobaron con
`claude --help` (`--resume <id>`) y `codex resume --help` (`codex resume [OPTIONS] [SESSION_ID]`).
El `session_id` de Gemini CLI (lo acepta `--resume`; una sesión reanudada lo conserva) y el
`conversation_id` de Cursor (`--resume [chatId]`) llegan en sus hooks de inicio de sesión; el id de
sesión de opencode, en los eventos del plugin (`--session` la continúa).

Lo que **no** se restaura: el proceso en sí. Una shell vuelve en la carpeta inicial de la sesión sin
nada en marcha; lo que corría en ella (servidores, `vim`…) se pierde. Un agente sin id reanudable
empieza una conversación nueva. Reanudar falla —y la sesión sigue suspendida, con el error en su
tooltip / `ls`— cuando la carpeta o el worktree ya no existen (`dir_missing`), el agente ya no está
instalado (`profile_not_installed`), el proyecto se quitó o el proceso anterior sigue de algún modo
en marcha (`resume_failed`; un pid solo cuenta como vivo si lidera su propio grupo de procesos y
sigue ejecutando el mismo comando, así que un pid reciclado no lo bloquea).

## Seguridad y apagado

- El socket es `0600` y los directorios `0700`: solo tu usuario puede hablar con el daemon.
- `daemon.shutdown` **se niega** sin `force` mientras haya sesiones vivas, así que un `panal stop`
  dirigido por error al daemon equivocado (por ejemplo, la limpieza de un test) no tumba trabajo
  en curso. SIGTERM/SIGINT al daemon lo siguen apagando siempre (es el sistema quien lo pide).
- Cada petición de apagado, aceptada o rechazada, queda en `daemon.log` con quién la hizo:
  `peer_pid`, `peer_cmd`, `peer_ppid` y `peer_parent_cmd` (PID del otro extremo del socket con
  `LOCAL_PEERPID` en macOS / `SO_PEERCRED` en Linux, y su línea de comandos vía `ps`), además de
  `force` y `live`. El motivo del apagado (señal o RPC) también queda registrado.
- SIGTERM/SIGINT se capturan con un manejador `SA_SIGINFO` (`internal/sigsender`, cgo en macOS y
  Linux), así que una línea `termination signal received` registra también **quién la envió**:
  `sender_pid`, `sender_uid`, `sender_cmd`, `sender_ppid` y `sender_parent_cmd` (se consultan al
  momento; si el remitente ya terminó, solo queda su PID; algunos kernels o sandboxes no reportan el
  remitente y queda `sender_pid=0`). Sin cgo, el daemon usa `os/signal`.
- `PANAL_HOME` manda sobre `PANAL_SOCK`: `PANAL_HOME=/tmp/x panal stop` lanzado desde
  dentro de una sesión va a `/tmp/x`, no al daemon real.
- Al apagarse, el daemon mata todas las sesiones y borra el socket y el pid file. Un socket huérfano
  (nadie escuchando, tras un fallo) se detecta y se reemplaza al arrancar.
