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

<p align="center">
  <img src="docs/img/screenshot.png" alt="agenterm: proyectos y sesiones en la barra lateral, actividad en vivo, una aprobación pendiente y la terminal" width="900">
</p>

## Funciones

- **Proyectos y sesiones** en una barra lateral: cada proyecto es una carpeta y cada sesión una
  terminal (xterm.js) con un agente o una shell. Doble clic en una sesión para ponerle nombre;
  arrastra proyectos y sesiones para ordenarlos. Cada proyecto tiene una insignia con sus iniciales y
  un color; haz clic en ella (o clic derecho en el proyecto) para renombrarlo, elegir el color o
  poner un emoji en su lugar. Suelta archivos (p. ej. capturas) en una terminal
  para escribir sus rutas; Claude Code adjunta las imágenes soltadas.
- **Paneles divididos**: varias sesiones lado a lado o apiladas (⌘D / ⌘⇧D), con divisores que se
  arrastran; la disposición se guarda por proyecto.
- **Comodidades de terminal**: búsqueda en el scrollback (⌘F), ⌘-clic para abrir URLs, nueve temas integrados con nombre
  de discos (*Turn On the Bright Lights*, *Room on Fire*, *Is This It*, *Funeral*, *Silent Alarm*,
  *Z*, *It Still Moves*, *Alphabetical* y *Hot Fuss*), fuente, tamaño, interlineado y cursor, y la
  app en claro u oscuro (o siguiendo a macOS).
- **Cualquier CLI de agente**: catálogo integrado (Claude Code, codex, opencode, Gemini, Cursor,
  Copilot, aider, Amp, goose, Qwen, Crush, Kiro, shell) con detección automática de los instalados,
  más perfiles propios y variantes en `profiles.yaml`.
- **Las sesiones sobreviven a la app**: un daemon es dueño de las PTYs; cierras la ventana, la
  reabres y todo sigue ahí con su scrollback. La CLI y la app comparten el mismo daemon. Si el
  propio daemon se reinicia, las sesiones vuelven suspendidas con su scrollback y se reanudan con
  un clic (Claude Code y codex continúan la misma conversación).
- **Detección de atención**: aprobaciones, preguntas, turnos terminados y errores (hooks del agente
  o señales de terminal), con notificaciones nativas de macOS, badge del Dock y marcas en la sidebar.
- **Actividad en vivo**: qué está haciendo cada agente ("Editando main.go"), cronómetro del turno,
  aviso de «¿trabado?», barras de progreso (OSC 9;4) y una vista general en rejilla (⌘0).
- **Worktrees de git por sesión**: cada agente puede trabajar en su propia rama y carpeta; la
  pestaña **Cambios** muestra el diff y permite hacer merge a la base o eliminar el worktree de forma
  segura.
- **Pull requests en GitHub y GitLab** desde la pestaña **Cambios**, para worktrees y para la rama
  en la que está tu carpeta principal: estado, checks y review, los comentarios (los de línea llevan
  al archivo), merge en el servidor y crear uno nuevo con título y descripción sugeridos. Usa tu
  propia sesión de `gh` / `glab`; agenterm no guarda tokens.
- **Ajustes en la app**: activar, ordenar y editar agentes, y configurar el idioma, la apariencia,
  las notificaciones y git.
- **Actualizaciones integradas** (Sparkle): una comprobación diaria en segundo plano, **Buscar
  actualizaciones…** en el menú de la app y una sección *Actualizaciones* en *Ajustes → General*,
  también en las instalaciones con Homebrew.
- **En inglés y español**: la app sigue los idiomas preferidos de macOS (con inglés como respaldo)
  o el idioma que elijas en *Ajustes → General*.

## Instalación

Proyecto en etapa temprana (0.x): espera asperezas. La app necesita **macOS 13 (Ventura) o
posterior**; es un build universal (Apple Silicon e Intel), aunque se prueba sobre todo en Apple
Silicon.

### Homebrew

```sh
brew install --cask elionaz/tap/agenterm
```

Instala `agenterm.app` en `/Applications` y pone la CLI `agenterm` en tu `PATH` (la CLI viene dentro
de la app, en `agenterm.app/Contents/Helpers/agenterm`). La app mantiene ambas al día
([Actualizar](#actualizar)); `brew uninstall --zap --cask agenterm` borra además `~/.agenterm` y
`~/.config/agenterm`.

### Descarga

Cada [release](https://github.com/elionaz/agenterm/releases) trae:

- `agenterm-<versión>-macos-universal.dmg` / `.zip`: la app (abre el dmg y arrástrala a
  Aplicaciones, o descomprime el zip);
- `agenterm-<versión>-darwin-universal.tar.gz`: solo la CLI para macOS;
- `agenterm-<versión>-linux-{amd64,arm64}.tar.gz`: la CLI y el daemon para Linux (experimental, sin
  notificaciones ni app);
- `checksums.txt`: el SHA-256 de cada archivo (`shasum -a 256 -c checksums.txt`).

Las releases van firmadas con un Developer ID y notarizadas por Apple. Si las notas de una release
dicen que es un **build sin firmar**, macOS se negará a abrirla; después de moverla a
`/Applications`, ejecuta `xattr -dr com.apple.quarantine /Applications/agenterm.app`.

Los cambios de cada versión están en [CHANGELOG.md](CHANGELOG.md) (solo en inglés).

### Compilar desde el código fuente

Requisitos:

- **macOS** (la app y las notificaciones son exclusivas de macOS).
- **Go 1.27.4+** (ver `go.mod`).
- **Node 24+** y npm (para el frontend; es la versión que usa el CI).
- **CLI de Wails v2**: `go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.1` (`wails doctor`
  debe pasar).

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
*Ajustes del Sistema → Notificaciones → agenterm* → "Permitir notificaciones". Sin ese permiso (o
desde un daemon fuera del `.app`) las notificaciones no se muestran, salvo que actives la
alternativa con AppleScript (`notify.fallback: osascript`; entonces aparecen como *Editor de
Scripts*). La CLI incluida en la app (la que Homebrew pone en tu `PATH`) arranca el daemon como la
app, así que también tiene notificaciones nativas.

## Actualizar

- **Homebrew**: la app se actualiza sola, igual que la descargada (abajo). Un simple
  `brew upgrade` no la toca, como con cualquier cask que se actualiza solo; `brew upgrade --cask
  agenterm` (o `--greedy`) sí la actualiza. Las instalaciones de antes de la 0.4.2 necesitan un
  último `brew upgrade --cask agenterm` para llegar ahí.
- **App descargada** (dmg/zip): agenterm busca una release nueva una vez al día y ofrece instalarla;
  **agenterm → Buscar actualizaciones…** lo comprueba al momento. Si encuentra una versión nueva
  mientras trabajas, un aviso te lo indica (**Actualizar**, o **Reiniciar para actualizar** cuando
  ya se descargó; **Más tarde** lo oculta para esa versión hasta que se descargue) y también
  aparece en *Ajustes → General → Actualizaciones*. Reiniciar para actualizar solo relanza la ventana: el daemon en
  segundo plano y tus sesiones siguen corriendo. En *Ajustes → General →
  Actualizaciones* puedes desactivar la comprobación diaria o dejar que las actualizaciones se
  descarguen e instalen solas (al salir). Antes de instalarse se verifican con una firma EdDSA y la firma
  de código Developer ID de la app (las releases que se ofrecen desde la app siempre van
  notarizadas).
- Los builds desde el código fuente (`wails build`) no incluyen el actualizador.

Las sesiones corren en un daemon en segundo plano que sobrevive a la app, así que justo después de
actualizar el daemon sigue siendo la versión anterior. La app lo reinicia sola cuando ningún agente
está en medio de un turno (las sesiones en marcha se suspenden y se reanudan, diga lo que diga
`sessions.restore`; las que habías dejado suspendidas siguen así). Si hay un agente trabajando,
muestra en su lugar un aviso con **Reiniciar ahora**, para que lo hagas cuando te convenga, o más
tarde desde una terminal con `agenterm stop --force`.

## Desinstalar

Primero detén el daemon: cierra la app y luego `agenterm stop` (se niega si hay sesiones en marcha;
`agenterm stop --force` las termina). Después:

- **Homebrew**: `brew uninstall --cask agenterm` quita la app y la CLI y conserva tus datos.
  `brew uninstall --zap --cask agenterm` también los borra: `~/.agenterm`, `~/.config/agenterm` y
  los archivos de agenterm en `~/Library` (cachés, preferencias, datos de WebKit, estado guardado de
  la ventana).
- **A mano**: borra `/Applications/agenterm.app`, el enlace de la CLI si lo creaste
  (`/usr/local/bin/agenterm`) y, para borrar también tus datos, `~/.agenterm` (estado del daemon,
  scrollback y los worktrees de agenterm), `~/.config/agenterm` (ajustes, perfiles, proyectos) y
  `~/Library/{Caches,WebKit}/com.elionaz.agenterm`,
  `~/Library/Preferences/com.elionaz.agenterm.plist` y
  `~/Library/Saved Application State/com.elionaz.agenterm.savedState`.

**Antes de borrar `~/.agenterm`**, revisa `~/.agenterm/worktrees` (o tu `git.worktree_dir`): ahí están los
worktrees que creó agenterm, y el trabajo sin commitear que tengan se pierde con él (`--zap` incluido). Sus ramas
quedan en tus repositorios; `git worktree prune` limpia después las referencias.

No hay nada más que deshacer: agenterm no cambia la configuración de tus agentes (los hooks se
añaden a cada sesión que abre, no se instalan) ni toca las carpetas de tus proyectos.

## Inicio rápido

**App.** La primera vez, una pantalla de bienvenida muestra qué CLIs de agentes se encontraron, te
deja permitir las notificaciones y explica cómo poner el comando `agenterm` en tu `PATH`. Haz clic en
**+ Añadir tu primer proyecto** (o en **+ Proyecto** de la barra lateral) y elige una carpeta. Con el **+** junto al proyecto eliges un agente
y si corre en la misma carpeta o en un worktree nuevo; **⌘N** lanza el primer agente de la lista.
Cuando un agente te necesita verás un punto ámbar en la sidebar y una notificación.

**Varios agentes en un proyecto.** Las sesiones en la carpeta del proyecto comparten su rama y sus
cambios: lo que edita o commitea un agente lo ven los demás, y un cambio de rama los mueve a todos.
Dale a cada agente su propio **worktree** para mantener su trabajo aparte; si ya hay otro agente
trabajando en la carpeta, el menú **+** (y ⌘N) elige un worktree nuevo por ti. El trabajo vuelve con
**Merge a la base** o **Crear pull request** en la pestaña Cambios. Más en
[docs/es/worktrees.md](docs/es/worktrees.md#varios-agentes-en-un-proyecto).

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
version: 1
profiles:
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
version: 1
ui:
  language: auto         # auto (idiomas preferidos de macOS; si no, inglés) | en | es
  appearance: system     # colores de la app: system (sigue a macOS) | dark | light
terminal:
  font_family: '"SF Mono", Menlo, Monaco, monospace'
  font_size: 13          # 8-32 (también ⌘= / ⌘- / ⌘⌥0)
  line_height: 1.15      # 1.0-2.0
  cursor_style: block    # block | bar | underline
  theme: bright-lights   # bright-lights | room-on-fire | is-this-it | funeral | silent-alarm | z |
                         # it-still-moves | alphabetical | hot-fuss
notify:
  enabled: true          # notificaciones (app y daemon)
  sound: true            # sonido en aprobación/pregunta
  notify_on_done: true   # avisar también cuando un agente termina
  notify_on_stall: true  # aviso discreto si un turno parece trabado
  fallback: none         # none | osascript: si no se pueden usar las nativas
                         # (osascript aparece como Editor de Scripts)
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
| ⌘⇧↑ / ⌘⇧↓ | subir / bajar la sesión seleccionada en su proyecto (o arrastrarla en la barra lateral) |
| ⌘0 | abrir/cerrar la vista general |
| ⌘, | abrir Ajustes |
| ⌘D / ⌘⇧D | dividir el panel enfocado a la derecha / abajo con una sesión nueva del mismo agente y modo |
| ⌘W | cerrar el panel enfocado (la sesión sigue en marcha; se termina con × en la barra lateral) |
| ⌘[ / ⌘] | enfocar el panel anterior / siguiente |
| ⌘⌥← ↑ → ↓ | enfocar el panel en esa dirección |
| ⌘F | buscar en la terminal enfocada; Enter / ⇧Enter o ⌘G / ⌘⇧G para siguiente / anterior |
| ⌘= / ⌘- / ⌘⌥0 | fuente de la terminal más grande / más pequeña / por defecto (se guarda en `terminal.font_size`) |
| ⌘C / ⌘V | copiar la selección / pegar en la terminal |
| ⌘-clic | abrir en el navegador la URL bajo el puntero (URLs en texto y enlaces OSC 8; solo http y https) |
| ⇧Enter | salto de línea en el prompt en vez de enviarlo (Claude Code, Codex…); envía un salto de línea (Ctrl-J) |
| Esc | cerrar la búsqueda, la vista general, Ajustes o la vista de un worktree huérfano |
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
- [docs/es/development.md](docs/es/development.md): desarrollo, tests, publicación de releases,
  icono y limitaciones conocidas.
- [docs/es/compatibility.md](docs/es/compatibility.md): qué se mantiene estable entre versiones,
  versiones de formato y de protocolo, y cómo volver a una versión anterior.
- [CHANGELOG.md](CHANGELOG.md): los cambios de cada versión (en inglés).

Si cambias comandos, flags, rutas o `settings.yaml`, actualiza el README y el documento
correspondiente en `docs/`, en inglés (`README.md`, `docs/`) y en español (`README.es.md`,
`docs/es/`).

## Contribuir

Las contribuciones son bienvenidas vía pull request: lee [CONTRIBUTING.es.md](CONTRIBUTING.es.md). Para
reportar vulnerabilidades, sigue [SECURITY.es.md](SECURITY.es.md).

Licencia: [MIT](LICENSE).
