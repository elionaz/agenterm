# Arquitectura

[English](../architecture.md) · **Español**

```
          ┌──────────────┐   ┌──────────────┐   ┌───────────────────────────┐
          │ agenterm.app │   │ agenterm CLI │   │ hooks de los agentes      │
          │ (Wails)      │   │              │   │ (`agenterm notify …`)     │
          └──────┬───────┘   └──────┬───────┘   └─────────────┬─────────────┘
                 │   socket unix (~/.agenterm/d.sock)          │
                 └──────────────────┬──────────────────────────┘
                             ┌──────┴──────┐
                             │   daemon    │  PTYs, estado, git, eventos
                             └──────┬──────┘
                 ┌──────────────────┼──────────────────┐
              claude              codex               shell …
```

- El **daemon** es el único dueño de las PTYs: lanza los procesos, guarda su scrollback, calcula
  estado, atención, actividad y git, y publica eventos. Escucha en un socket unix (`0600`).
- La **app** y la **CLI** son clientes (`internal/client`). Cualquiera de los dos arranca el daemon
  si no responde (`client.StartDaemon`: proceso desacoplado en su propia sesión, salida a
  `daemon.log`). Cerrar la app o desacoplar `attach` no afecta a las sesiones.
- El `.app` es autosuficiente: su ejecutable también implementa `daemon` y `notify`
  (`agenterm.app/Contents/MacOS/agenterm daemon`), así que no hace falta la CLI.
- Antes de lanzar el daemon, la app toma el `PATH` de tu shell de login (`$SHELL -l -i -c`, con
  timeout de 3 s) y fija `TERM=xterm-256color`, `COLORTERM=truecolor`, `TERM_PROGRAM=agenterm` y
  `LANG` si falta: las apps abiertas desde Finder heredan el entorno mínimo de launchd y no
  encontrarían `claude` o `codex`. El daemon, a su vez, busca los agentes en su `PATH` más el del
  shell de login.

## Paquetes

```
cmd/agenterm            CLI + subcomandos `daemon` y `notify` (un solo binario)
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
internal/hooks          configuración de hooks por sesión (claude, codex) y parseo de payloads
internal/profile        catálogo de agentes, detección, combinación con profiles.yaml y escritura
internal/settings       settings.yaml (lectura con valores por defecto y edición en su sitio)
internal/gitx           git por CLI: repo, ramas, status, worktrees, diff, merge, abrir en editor
internal/worktree       registro de worktrees de agenterm (worktrees.json)
internal/project        registro de proyectos (projects.json)
internal/desktopnotify  textos de las notificaciones + respaldo con osascript
internal/macnotify      UNUserNotificationCenter (cgo, darwin): envío, permiso, clics, run loop
internal/peercred       PID del proceso al otro lado del socket (LOCAL_PEERPID / SO_PEERCRED) + `ps`
internal/shellenv       PATH del shell de login
internal/atomicfile     escritura atómica con .bak
internal/paths          resolución de directorios (AGENTERM_HOME)
```

## Protocolo

### Control

JSON delimitado por `\n`. Petición `{"id":1,"method":"session.list","params":{…}}`; respuesta
`{"id":1,"result":{…}}` o `{"id":1,"error":"…"}`. Una conexión admite varias peticiones seguidas.

| grupo | métodos |
|---|---|
| proyectos | `project.add`, `project.list`, `project.remove`, `project.git` |
| perfiles | `profile.list`, `profile.catalog`, `profile.save`, `profile.delete`, `profile.reorder`, `profile.rescan` |
| ajustes | `settings.get`, `settings.save` |
| sesiones | `session.spawn`, `session.list`, `session.kill`, `session.resize`, `session.notify`, `session.ack`, `session.attach`, `session.tail`, `session.diff` |
| worktrees | `worktree.list`, `worktree.diff`, `worktree.remove`, `worktree.merge`, `worktree.open` |
| daemon | `events.subscribe`, `daemon.ping`, `daemon.shutdown` |

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
| `merge_nothing` | `{branch, base}` |
| `merge_conflict` | `{branch, base, files}` |
| `branch_missing`, `branch_exists`, `branch_invalid` | `{branch}` |
| `profile_not_installed` | `{profileId, cmd, install}` |
| `not_a_repo` | `{dir, project?}` |
| `no_commits` | `{dir}` |
| `base_missing` | `{base}` (vacío = no se pudo deducir una base) |
| `dir_missing` | `{dir}` |

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
| `session.removed` | la sesión se quitó de la lista |
| `worktree.changed` | cambió la lista de worktrees de un proyecto (`projectId`) |

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
| `app:open-settings` | menú *agenterm → Preferencias…* |

## Sesiones

- Cada sesión tiene un ring buffer de scrollback (8 MiB) y N suscriptores con cola acotada; un
  suscriptor lento se desconecta (frame `0x04`) en vez de bloquear la lectura del PTY.
- `kill` envía SIGHUP y SIGTERM al grupo de procesos y SIGKILL a los 2 s. Un segundo `kill` sobre
  una sesión terminada la quita de la lista.
- Los ids de proyectos y sesiones son 4 bytes aleatorios en hexadecimal.

### Entorno de las sesiones

El daemon inyecta en cada sesión:

- `AGENTERM_SESSION=<id>`;
- `AGENTERM_SOCK=<su socket>`;
- `AGENTERM_HOME=<su directorio>` si el daemon usa `AGENTERM_HOME`; si usa las rutas por defecto,
  **quita** cualquier `AGENTERM_HOME` heredado.

Los clientes dan prioridad a `AGENTERM_HOME` sobre `AGENTERM_SOCK` (ver
[cli.md](cli.md#qué-daemon-usa-un-comando)), así que ambos tienen que apuntar al mismo daemon. Los
hooks generados pasan además `--sock <socket del daemon>`, así que llegan a su daemon aunque cambies
`AGENTERM_HOME` dentro de la sesión.

## Persistencia y sus límites

| sobrevive a… | cerrar la app | reiniciar el daemon / la computadora |
|---|---|---|
| procesos de las sesiones y su scrollback | sí | **no** |
| proyectos (`projects.json`), perfiles, ajustes | sí | sí |
| worktrees y su registro (`worktrees.json`) | sí | sí (quedan como huérfanos) |

Las sesiones viven en memoria del daemon: si el daemon se detiene (`agenterm stop --force`,
SIGTERM, un fallo, reiniciar la computadora), sus procesos mueren y su scrollback se pierde. Al
reabrir la app, el daemon nuevo empieza sin sesiones; los worktrees siguen ahí para reabrirlos.

## Seguridad y apagado

- El socket es `0600` y los directorios `0700`: solo tu usuario puede hablar con el daemon.
- `daemon.shutdown` **se niega** sin `force` mientras haya sesiones vivas, así que un `agenterm stop`
  dirigido por error al daemon equivocado (por ejemplo, la limpieza de un test) no tumba trabajo
  en curso. SIGTERM/SIGINT al daemon lo siguen apagando siempre (es el sistema quien lo pide).
- Cada petición de apagado, aceptada o rechazada, queda en `daemon.log` con quién la hizo:
  `peer_pid`, `peer_cmd`, `peer_ppid` y `peer_parent_cmd` (PID del otro extremo del socket con
  `LOCAL_PEERPID` en macOS / `SO_PEERCRED` en Linux, y su línea de comandos vía `ps`), además de
  `force` y `live`. El motivo del apagado (señal o RPC) también queda registrado.
- SIGTERM/SIGINT se capturan con un manejador `SA_SIGINFO` (`internal/sigsender`, cgo en macOS y
  Linux), así que una línea `termination signal received` registra también **quién la envió**:
  `sender_pid`, `sender_uid`, `sender_cmd`, `sender_ppid` y `sender_parent_cmd` (se consultan al
  momento; si el remitente ya terminó, solo queda su PID). Sin cgo, el daemon usa `os/signal`.
- `AGENTERM_HOME` manda sobre `AGENTERM_SOCK`: `AGENTERM_HOME=/tmp/x agenterm stop` lanzado desde
  dentro de una sesión va a `/tmp/x`, no al daemon real.
- Al apagarse, el daemon mata todas las sesiones y borra el socket y el pid file. Un socket huérfano
  (nadie escuchando, tras un fallo) se detecta y se reemplaza al arrancar.
