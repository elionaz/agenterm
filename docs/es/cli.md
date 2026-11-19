# Referencia de la CLI

[English](../cli.md) · **Español**

```sh
go build -o agenterm ./cmd/agenterm
agenterm help            # también -h / --help
agenterm version         # también --version: "agenterm 0.1.0 (abc1234)"; "dev" en builds locales
```

Un solo binario hace de CLI, de daemon (`agenterm daemon`) y de callback para los hooks de los
agentes (`agenterm notify`). El ejecutable de la app (`agenterm.app/Contents/MacOS/agenterm`)
también implementa `daemon`, `notify` y `version`, así que la app no depende de la CLI. Las releases
incluyen además la CLI dentro de la app, en `agenterm.app/Contents/Helpers/agenterm` (Homebrew la
enlaza en el `PATH`).

Reglas generales:

- Cualquier comando que hable con el daemon lo **arranca si no está corriendo** (desacoplado, con la
  salida en `daemon.log`), salvo `stop`, `notify` y los comandos lanzados dentro de una sesión que
  solo tienen `$AGENTERM_SOCK` (ver [Qué daemon usa un comando](#qué-daemon-usa-un-comando)).
- Los flags pueden ir antes o después de los argumentos (`agenterm diff abc123 --stat`) y se
  aceptan con uno o dos guiones (`-force` / `--force`).
- Donde se pide un proyecto puedes usar su id o su nombre.
- Los ids de proyectos y sesiones son 8 caracteres hexadecimales. El id de un worktree es el de la
  sesión que lo creó.
- Si algo falla, el comando imprime `agenterm: <error>` y sale con 1.
- La salida de la CLI y los mensajes de error del daemon están en inglés, sea cual sea
  `ui.language` (ver [configuration.md](configuration.md#idioma)).

## Daemon

### `agenterm daemon [--detach] [--idle D]`

Ejecuta el daemon en primer plano (log a stderr) hasta recibir SIGTERM/SIGINT o una petición de
apagado.

| flag | |
|---|---|
| `--detach` | lo arranca en segundo plano (log en `daemon.log`) y vuelve cuando el socket responde; si ya corre, lo dice y sale con 0 |
| `--idle D` | timeout de inactividad de la heurística (duración de Go: `3s`, `1500ms`…); por defecto 3 s. Un perfil puede sobrescribirlo con `idle_seconds` |

### `agenterm stop [--force]`

Pide al daemon que se apague y espera (hasta 10 s) a que el socket desaparezca; imprime
`daemon stopped`. Nunca arranca un daemon.

- Sin `--force` **se niega** (sale con 1) si hay sesiones que no han terminado: el error lista
  esas sesiones (`<id> (<perfil>)`) y sugiere `--force`. Las sesiones `exited` no cuentan.
- `--force` mata las sesiones vivas y apaga el daemon. No se pierden: el daemon siguiente las
  restaura como `suspended` (ver [`resume`](#agenterm-resume-sessionid--all)). Las sesiones
  `suspended` tampoco cuentan como vivas.

La comprobación la hace el propio daemon, así que también protege de scripts o clientes antiguos.
Cada petición (aceptada o rechazada) queda en `daemon.log` con el proceso que la hizo (ver
[architecture.md](architecture.md#seguridad-y-apagado)).

## Proyectos

| comando | |
|---|---|
| `agenterm project add <dir> [--name N]` | registra una carpeta (rutas relativas se resuelven desde tu directorio actual). Imprime `<id>  <nombre>  <ruta>`. El nombre por defecto es el de la carpeta. Si la carpeta ya estaba registrada devuelve el proyecto existente |
| `agenterm project list` (o `ls`) | tabla `ID NAME PATH ADDED` |
| `agenterm project rm <id>` (o `remove`) | quita el proyecto de la lista (no borra archivos) |
| `agenterm project set <id\|nombre> [--name N] [--color C\|auto] [--icon E\|none]` | renombra el proyecto o cambia su insignia en la barra lateral. Colores: `blue`, `teal`, `green`, `amber`, `orange`, `red`, `pink`, `purple`; `auto` elige uno a partir del id del proyecto. `--icon` pone un emoji en lugar de las iniciales; `none` vuelve a las iniciales. Las ventanas abiertas se actualizan al instante. Imprime `<id>  <nombre>  <color>  <icono>` |

## Perfiles (agentes)

| comando | |
|---|---|
| `agenterm profiles` (o `profiles list` / `ls`) | perfiles efectivos (catálogo + `profiles.yaml`), en el orden del menú |
| `agenterm profiles scan` | vuelve a leer el `PATH` del shell de login, detecta de nuevo los agentes instalados y los lista |

Columnas: `ID NAME ENABLED INSTALLED SOURCE STATUS CMD`. `INSTALLED` es la ruta del ejecutable,
`-` si no se encontró, o `error: …` si la entrada de `profiles.yaml` es inválida. `SOURCE` es
`catalog`, `override` (del catálogo con cambios tuyos) o `custom`. Ver
[configuration.md](configuration.md).

## Sesiones

### `agenterm spawn [flags] <proyecto> <perfil>`

Lanza una sesión e imprime su id. En modo worktree imprime `<id>  <rama>  <carpeta>`.

| flag | |
|---|---|
| `--cols C`, `--rows R` | tamaño inicial (por defecto el de tu terminal, o 80×24) |
| `--worktree` | crea un worktree de git nuevo con su propia rama |
| `--branch B` | rama para `--worktree` (por defecto `agent/<perfil>-<4 primeros del id>`) |
| `--base X` | rama o commit base para `--worktree` (por defecto la rama actual del proyecto) |
| `--in-worktree ID` | lanza la sesión en un worktree de agenterm que ya existe (p. ej. un huérfano) |
| `--trust-setup` | confía en el setup de worktrees del repositorio (`.agenterm.yaml`) y lo corre |
| `--no-setup` | crea el worktree sin correr su setup (los archivos se copian igual) |

`--branch`, `--base`, `--trust-setup` y `--no-setup` requieren `--worktree`. Un setup en el que aún
no confías hace fallar `--worktree` con `setup_untrusted` (ver
[worktrees.md](worktrees.md#preparar-los-worktrees-nuevos)). Falla si el perfil no existe, tiene un error o su
comando no está instalado (el mensaje incluye la pista de instalación del catálogo). Ver
[worktrees.md](worktrees.md).

### `agenterm ls`

Tabla con `ID NAME PROJECT PROFILE STATE ATTENTION PID SIZE AGE TURN GIT ACTIVITY CWD`:

- `NAME`: el nombre puesto con `rename` (o en la app), `-` si no tiene.
- `STATE`: `running`, `waiting_input`, `idle`, `exited(<código>)` o `suspended[<modo>]`
  (restaurada tras un reinicio del daemon; `<modo>` es cómo la relanzaría `resume`: `agent`, `args`
  o `fresh`, seguido de `!<errorCode>` si el último intento falló).
- `ATTENTION`: `permission`, `question`, `done`, `error` o `-`; con `*` si no se ha visto.
- `TURN`: cronómetro del turno en curso (`4m12s`) o duración del último (`=6m0s`).
- `GIT`: rama, `±N` archivos cambiados y `↑N` commits por delante de la base.
- `ACTIVITY`: `[hook|output] <línea de actividad>`, con `(estado N%)` si hay progreso y el prefijo
  `STALLED` si el turno parece trabado.

### `agenterm tail [-n N] <sessionId>`

Últimas `N` líneas (por defecto 8, máximo 200) de la salida en texto plano, sin secuencias ANSI.

### `agenterm attach <sessionId>`

Conecta tu terminal a la sesión: primero repinta el scrollback y después sigue en vivo. Reenvía los
cambios de tamaño de la ventana. **Ctrl-\\** se desacopla; la sesión sigue viva. Sobre una sesión
`suspended` muestra el scrollback guardado y espera; si mientras tanto se reanuda, termina con
`[daemon: session resumed; reattach]` (vuelve a ejecutar `attach`).

### `agenterm kill <sessionId>`

Termina la sesión (SIGHUP + SIGTERM al grupo de procesos, SIGKILL a los 2 s) e imprime
`<id> <estado>`. Sobre una sesión que ya terminó, la **quita de la lista**; sobre una `suspended`,
la descarta. Nunca borra su worktree. Una sesión matada no se restaura tras reiniciar el daemon.

### `agenterm rename <sessionId> <nombre>`

Pone un nombre a la sesión (se ve en la barra lateral de la app, las cabeceras de los paneles, la
vista general y las notificaciones, que dicen `<proyecto> · <nombre>`) e imprime `<id> <nombre>`.
Las palabras restantes se unen con espacios (`agenterm rename ab12 servidor api`); usa `--` antes
de un nombre que empiece por `-`. `--reset` (o dejar el nombre vacío en la app) vuelve al nombre del
perfil. Los nombres se recortan, tienen como mucho 80 caracteres y sobreviven a los reinicios del
daemon y a las reanudaciones (RPC `session.rename`).

### `agenterm resume <sessionId>|--all`

Relanza sesiones `suspended` (restauradas tras un reinicio del daemon) en la misma carpeta o
worktree, con su scrollback guardado seguido de una línea `── sesión restaurada · <hora> ──`.
Claude Code y codex continúan la misma conversación (`--resume <session_id>` /
`codex resume <thread-id>`), los demás perfiles usan sus `resume_args` si los tienen y una shell
vuelve sin nada en marcha. Imprime `<id> <estado> pid <pid>`; con `--all`, los fallos van a stderr
(`<id>: <error>`) y el código de salida es 1. Ver
[architecture.md](architecture.md#persistencia-y-reanudación).

### `agenterm discard <sessionId>|--all`

Olvida sesiones `suspended`: salen de la lista y se borran su registro y su scrollback guardado
(su worktree, si lo hay, queda como huérfano).

### `agenterm ack <sessionId>`

Marca la atención de la sesión como vista (lo mismo que seleccionarla en la app).

## Mensajes

Mensajes entre las sesiones de un proyecto; ver [messages.md](messages.md).

| comando | |
|---|---|
| `agenterm msg peers [--session ID]` | las otras sesiones de este proyecto (fuera de una sesión: las de `--session`) |
| `agenterm msg send [--project ID] <a> <texto…>` | a un id de sesión, un nombre o `all`; con el texto `-` lee stdin |
| `agenterm msg read` | tus mensajes nuevos (solo dentro de una sesión); los marca como entregados |
| `agenterm msg list [--pending] [--session ID \| --project ID]` | historial de mensajes (una sesión solo ve los suyos) |
| `agenterm wait <sessionId> [--timeout 30m]` | hasta que la sesión quede inactiva, esperando input o terminada; código de salida 2 si se agota el tiempo |

## Git

| comando | |
|---|---|
| `agenterm diff [--stat] <sessionId\|worktreeId>` | diff unificado de la sesión (worktree: contra su base; misma carpeta: contra `HEAD`). Con `--stat`, resumen por archivo, commits por delante y totales |
| `agenterm worktrees <proyecto>` | worktrees de agenterm del proyecto: `ID BRANCH BASE STATE GIT SESSIONS PATH`, con `STATE` = `live`, `exited` u `orphan` |
| `agenterm worktree rm [--force] [--delete-branch] [--dry-run] <id>` (o `remove`) | elimina un worktree. `--dry-run` solo dice qué se perdería |
| `agenterm worktree merge <id>` | `git merge --no-ff` de su rama en la carpeta principal |
| `agenterm worktree pr [--no-open] <id>` | sube su rama (`git push -u`, nunca forzado) e imprime la página para crear el pull request hacia la base (GitHub / GitLab), y la abre salvo con `--no-open`. Otros servidores: solo el push |
| `agenterm worktree open [--finder] <id>` | abre el worktree en el editor (`git.editor`) o, con `--finder`, en Finder |

Las reglas de seguridad de `rm`, `merge` y `pr` están en [worktrees.md](worktrees.md#acciones).

## `agenterm notify`

Lo usan los hooks de los agentes, pero también puedes llamarlo tú (o un script) **dentro de una
sesión**. Nunca arranca el daemon y tiene timeout de 3 s.

```sh
agenterm notify --kind permission --message "¿Borro node_modules?"
agenterm notify --activity "Ejecutando migraciones"
agenterm notify --state idle
```

| flag | |
|---|---|
| `--session ID` | sesión (por defecto `$AGENTERM_SESSION`, que el daemon inyecta en cada sesión) |
| `--state S` | `running`, `waiting_input` o `idle` (alias: `waiting`, `done`) |
| `--kind K` | `permission`, `question`, `done` o `error`; si no das `--state`, lo implica (`permission`/`question` → `waiting_input`, `done`/`error` → `idle`) |
| `--message M` | texto de la notificación |
| `--activity A` | línea de actividad; sin `--state` solo actualiza eso |
| `--sock RUTA` | socket del daemon (por defecto el de la regla de abajo) |
| `--from-hook claude\|codex` | lee el payload de un hook: Claude Code por stdin, codex como último argumento |

Con `--from-hook` no imprime nada y **siempre** sale con 0 (timeout de 2 s, payload de hasta
1 MiB): nunca bloquea ni rompe al agente. Fuera de una sesión de agenterm (sin `--session` ni
`$AGENTERM_SESSION`) no hace nada.

## Variables de entorno

| variable | |
|---|---|
| `AGENTERM_HOME` | un solo directorio que sustituye a `~/.agenterm` y `~/.config/agenterm` (útil para pruebas) |
| `AGENTERM_NOTIFY` | `off` desactiva todas las notificaciones de escritorio de la app y del daemon (nativas y alternativa), diga lo que diga `settings.yaml`. Defínela, junto con `AGENTERM_HOME`, en cualquier instancia de prueba o demo |
| `AGENTERM_SESSION` | id de la sesión; el daemon lo inyecta en cada sesión y `notify` lo usa por defecto |
| `AGENTERM_SOCK` | socket del daemon; inyectado en cada sesión. Se ignora si `AGENTERM_HOME` está definido |

### Qué daemon usa un comando

1. Si `AGENTERM_HOME` está definido, **manda siempre**: se usa `$AGENTERM_HOME/d.sock`.
2. Si no, `$AGENTERM_SOCK` (dentro de una sesión). En ese caso la CLI nunca arranca otro daemon.
3. Si no hay ninguno, `~/.agenterm/d.sock`.

Así `AGENTERM_HOME=/tmp/x agenterm stop` lanzado desde dentro de una sesión va a `/tmp/x` y no al
daemon que aloja la sesión. Ver [architecture.md](architecture.md#entorno-de-las-sesiones).
