# agenterm

Workspace de escritorio para agentes de código (Claude Code, codex, opencode, shell…),
organizado por proyecto. **Fase 1**: daemon + PTYs + CLI. La UI (Wails) llega en la fase 2
y se conectará al daemon como un cliente más (`internal/client`).

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
agenterm notify --state waiting     # dentro de una sesión usa $AGENTERM_SESSION / $AGENTERM_SOCK
agenterm kill <sessionId>           # una segunda vez la elimina de la lista
agenterm stop                       # mata las sesiones y apaga el daemon
```

`notify` acepta `running`, `waiting_input` (`waiting`) e `idle` (`done`). Nunca arranca el
daemon y tiene timeout de 3 s, así que es seguro usarlo desde hooks.

### Rutas

| | por defecto | con `AGENTERM_HOME=/x` |
|---|---|---|
| socket, pid, log | `~/.agenterm/{d.sock,d.pid,daemon.log}` | `/x/…` |
| perfiles, proyectos | `~/.config/agenterm/{profiles.yaml,projects.json}` | `/x/…` |

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
  setup_hooks: claude  # reservado para la fase 3
  idle_seconds: 3      # opcional, sobreescribe el timeout heurístico
```

## Arquitectura

```
cmd/agenterm        CLI + subcomando `daemon` (un solo binario)
internal/daemon     servidor: registro de sesiones, RPC, attach, eventos, pid/socket
internal/client     cliente reutilizable (CLI hoy, Wails mañana) + StartDaemon
internal/protocol   tipos del wire: JSON por líneas + frames binarios
internal/transport  interfaz Listen/Dial/Cleanup (unix socket; named pipe en el futuro)
internal/session    PTY (creack/pty), ring buffer, fan-out a suscriptores, kill/wait
internal/status     máquina de estados + heurística (reloj inyectable)
internal/profile    perfiles YAML
internal/project    registro de proyectos en JSON
internal/paths      resolución de directorios
```

### Protocolo

- **Control**: JSON delimitado por `\n`. Petición `{"id":1,"method":"session.list","params":{…}}`,
  respuesta `{"id":1,"result":{…}}` o `{"id":1,"error":"…"}`. Una conexión admite varias
  peticiones seguidas. Métodos: `project.add|list|remove`, `profile.list`,
  `session.spawn|list|kill|resize|notify|attach`, `events.subscribe`, `daemon.ping|shutdown`.
- **`events.subscribe`**: tras la respuesta, el daemon emite un evento JSON por línea
  (`session.spawned`, `session.state`, `session.removed`). Si el cliente se retrasa se le
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
  sin salida → `idle`. En perfiles `status: hooks`, el primer evento explícito desactiva la
  heurística para esa sesión. `exited` es terminal.
- `kill` envía SIGHUP+SIGTERM al grupo de procesos y SIGKILL tras 2 s. SIGTERM/SIGINT al
  daemon (o `agenterm stop`) mata todas las sesiones y borra socket y pid file. Un socket
  huérfano (nadie escuchando) se detecta y se reemplaza al arrancar.

## Tests

```sh
go vet ./... && go test -race ./...
```

Incluye un test de integración que levanta el daemon en un `AGENTERM_HOME` temporal y ejercita
spawn, attach/detach/reattach con scrollback, notify, eventos, kill y apagado.
