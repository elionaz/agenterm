# Desarrollo

[English](../development.md) · **Español**

Requisitos: los de [README.es.md](../../README.es.md#estado-y-requisitos) (Go, Node 24+ con npm y la CLI de
Wails v2). Las reglas para abrir un pull request están en [CONTRIBUTING.es.md](../../CONTRIBUTING.es.md).

## App en modo desarrollo

```sh
cd app
wails dev     # hot reload (frontend en Vite); también sirve http://localhost:34115
wails build   # -> app/build/bin/agenterm.app
```

Solo el frontend:

```sh
cd app/frontend
npm ci
npm run check   # svelte-check
npm run build   # vite build
```

## Tests

```sh
gofmt -l .                       # no debe listar nada
go vet ./... && go test -race ./...
```

Es lo mismo que corre el CI (más `npm run check` y `npm run build` del frontend).

- Hay un test de integración que levanta el daemon en un `AGENTERM_HOME` temporal y ejercita spawn,
  attach/detach/reattach con scrollback, notify, eventos, kill y apagado, y otro que lanza una shell
  en modo worktree sobre un repo temporal, hace cambios escribiendo en ella y comprueba
  `session.diff`, que `kill` conserva el worktree, el huérfano tras reiniciar el daemon, reabrirlo,
  el merge y `worktree.remove` (rechazo y force).
- Los tests de `internal/gitx` usan repos reales en directorios temporales con la configuración de
  git aislada (`GIT_CONFIG_NOSYSTEM=1`, `GIT_CONFIG_GLOBAL` vacío, `HOME` temporal).

## Probar sin tocar tu instalación

Usa un `AGENTERM_HOME` propio. Tiene que ser una ruta **corta** (`/tmp/…`): en macOS la ruta de un
socket unix tiene un límite de unos 104 bytes y las carpetas temporales de `/var/folders/…` se
pasan.

```sh
export AGENTERM_HOME=/tmp/agt
go run ./cmd/agenterm project add .
go run ./cmd/agenterm spawn agenterm shell
go run ./cmd/agenterm ls
go run ./cmd/agenterm stop --force      # apaga solo el daemon de /tmp/agt

cd app && AGENTERM_HOME=/tmp/agt wails dev   # la app contra ese mismo daemon
```

Como `AGENTERM_HOME` manda sobre `AGENTERM_SOCK`, esto también es seguro dentro de una sesión de
agenterm: no toca el daemon que aloja la sesión.

## Bindings de Wails

Los bindings TypeScript de los métodos de `app/app.go` están en `app/frontend/wailsjs/` y se
versionan. `wails dev` y `wails build` los regeneran; si cambias la firma de un método exportado y
solo quieres los bindings:

```sh
cd app && wails generate module
```

## Icono

Las fuentes están en `assets/icon/`:

- `agenterm-icon.svg`: el icono cuadrado, a sangre completa;
- `agenterm-icon-rounded.svg`: con esquinas redondeadas (el del README);
- `agenterm-icon-macos.svg`: con el margen de la plantilla de iconos de macOS, que es el que usa la
  app.

Wails genera el icono del `.app` a partir de `app/build/appicon.png` (1024×1024). Para regenerarlo
con Inkscape 1.x:

```sh
inkscape assets/icon/agenterm-icon-macos.svg \
  --export-type=png --export-filename=app/build/appicon.png -w 1024 -h 1024
```

En macOS la CLI de Inkscape no queda en el `PATH`; está en
`/Applications/Inkscape.app/Contents/MacOS/inkscape`. Usa colores sólidos en los bordes (no
`stroke-opacity`): los trazos semitransparentes se suman donde se cruzan.

## Limitaciones conocidas

- **Solo macOS.** La app, las notificaciones y el Dock son exclusivos de macOS y solo se prueba en
  macOS (Apple Silicon). El transporte es un socket unix; no hay soporte para Windows.
- **Sin releases firmadas** todavía: hay que compilar desde el código fuente.
- **Las sesiones no sobreviven a un reinicio del daemon** (ni de la computadora): sus procesos
  mueren y el scrollback se pierde. Los worktrees sí se conservan, como huérfanos.
- **Notificaciones con la app cerrada**: si el daemon lo arrancó la CLI (no está dentro del `.app`)
  o el bundle no tiene permiso, se usa `osascript`: la notificación aparece como *Script Editor* y
  pulsarla no abre agenterm.
- **codex** no avisa cuando empieza a trabajar: "trabajando" lo decide la heurística y no hay
  aviso de «¿trabado?».
- **«¿Trabado?»** solo existe en sesiones con hooks (Claude Code), y como el spinner de Claude
  redibuja cada segundo, detecta sobre todo cuelgues completos, no herramientas lentas.
- **Línea de actividad en TUIs** que redibujan la pantalla (opencode, codex…): es la última línea
  que pintaron, que no siempre es la más informativa.
- **Kiro CLI**: el nombre del binario (`kiro-cli chat`) viene de la documentación y no se ha
  verificado con una instalación real; se puede corregir con `cmd:` en `profiles.yaml`.
- **`profiles.yaml` pierde los comentarios** cuando la app lo guarda (se reescribe en formato
  canónico). `settings.yaml` sí los conserva.
- El merge de un worktree **no hace stash**: la carpeta principal tiene que estar limpia y en la
  rama base.
