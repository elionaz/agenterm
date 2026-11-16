# Configuración

## Rutas

| | por defecto | con `AGENTERM_HOME=/x` |
|---|---|---|
| socket, pid y log del daemon | `~/.agenterm/{d.sock,d.pid,daemon.log}` | `/x/…` |
| perfiles, proyectos y ajustes | `~/.config/agenterm/{profiles.yaml,projects.json,settings.yaml}` | `/x/…` |
| archivos de hooks generados | `~/.agenterm/hooks/` | `/x/hooks/` |
| worktrees de las sesiones | `~/.agenterm/worktrees/<proyecto>-<projectId>/<sessionId>/` (o `git.worktree_dir`) | `/x/worktrees/…` |
| registro de worktrees | `~/.agenterm/worktrees.json` | `/x/worktrees.json` |

- Los directorios se crean con permisos `0700` y el socket con `0600`.
- En macOS la ruta de un socket unix tiene un límite de unos 104 bytes: si usas `AGENTERM_HOME`,
  que sea una ruta corta (`/tmp/agt`, no una carpeta temporal larga de `/var/folders/…`).
- `profiles.yaml` y `settings.yaml` son opcionales. Cuando la app (o una RPC) los guarda, la
  escritura es atómica y deja la versión anterior en `<archivo>.bak`.

## Agentes: catálogo y detección

agenterm trae un catálogo de CLIs de agentes conocidos. Un agente cuenta como **instalado** si su
binario se encuentra en el `PATH` del daemon o en el de tu shell de login (`$SHELL -l -i -c`); esa
búsqueda se guarda en caché 5 minutos. `agenterm profiles scan` o **Re-escanear** en *Ajustes →
Agentes* la refrescan.

| id | agente | comando | estado | hooks | notas |
|---|---|---|---|---|---|
| `claude` | Claude Code | `claude` | `hooks` | `claude` | |
| `codex` | Codex CLI | `codex` | `heuristic` | `codex` | `notify` solo avisa el fin de turno, así que "trabajando" lo decide la heurística |
| `opencode` | opencode | `opencode` | `heuristic` | | |
| `gemini` | Gemini CLI | `gemini` | `heuristic` | | |
| `cursor-agent` | Cursor CLI | `cursor-agent` | `heuristic` | | |
| `copilot` | GitHub Copilot CLI | `copilot` | `heuristic` | | |
| `aider` | Aider | `aider` | `heuristic` | | patrón de atención `\(Y\)es/\(N\)o` |
| `amp` | Amp | `amp` | `heuristic` | | |
| `goose` | goose | `goose session` | `heuristic` | | |
| `qwen` | Qwen Code | `qwen` | `heuristic` | | |
| `crush` | Crush | `crush` | `heuristic` | | |
| `kiro` | Kiro CLI | `kiro-cli chat` | `heuristic` | | nombre del binario sin verificar |
| `shell` | Shell | `$SHELL` (o `/bin/sh`) | `heuristic` | | siempre disponible |

- **estado** (`status`): `hooks` = el estado lo deciden eventos explícitos (hooks / `notify`) y la
  heurística solo se usa hasta el primer evento; `heuristic` = salida → trabajando, unos segundos sin
  salida → inactivo. Ver [attention-and-activity.md](attention-and-activity.md).
- **hooks** (`setup_hooks`): integración que agenterm configura al lanzar el agente.
- Los agentes no instalados (o desactivados) no salen en el menú **+** ni en ⌘N; en *Ajustes* sí,
  con el comando de instalación y un enlace.

## `profiles.yaml`

Es una lista YAML. Cada entrada tiene un `id` obligatorio y único, y se **superpone** al catálogo:

| campo | |
|---|---|
| `id` | identificador (letras, dígitos, `.`, `_`, `-`) |
| `name` | nombre visible |
| `extends` | id del perfil del que hereda (solo perfiles propios) |
| `enabled` | `false` lo oculta de los menús; por defecto `true` (no se hereda) |
| `order` | posición explícita (menor primero); sin ella cuenta la posición en la lista |
| `cmd` | comando; se expanden `$VARIABLES` |
| `args` | lista de argumentos |
| `env` | mapa `CLAVE: valor` (los valores también expanden `$VARIABLES`) |
| `status` | `hooks` o `heuristic` |
| `setup_hooks` | `claude`, `codex` o `none` |
| `idle_seconds` | segundos sin salida para pasar a inactivo (sobrescribe el valor del daemon, 3 s) |
| `attention_patterns` | expresiones regulares (sintaxis de Go) sobre la salida reciente sin ANSI; una coincidencia marca la sesión como esperando tu respuesta |

### Reglas de combinación

- **Entrada con el id de un agente del catálogo**: solo cambia los campos que pongas; el resto
  (incluido `setup_hooks`) se hereda. Un campo vacío u omitido hereda; `setup_hooks: none` desactiva
  la integración; `args: []` vacía los argumentos heredados. Una entrada del catálogo no puede usar
  `extends`.
- **Id nuevo sin `extends`**: perfil propio; `cmd` es obligatorio.
- **Id nuevo con `extends: <id>`**: variante. Hereda todo del padre ya resuelto (con tus cambios
  incluidos) y sus campos se aplican encima; `args`, `env` y `attention_patterns` **reemplazan** a
  los del padre (no se mezclan). El nombre por defecto de una variante es su id.
- **Orden**: primero las entradas del archivo, por `order` o por posición; después los agentes del
  catálogo que el archivo no menciona, en el orden del catálogo. El primero activado e instalado es
  el de ⌘N. Una entrada que solo tiene `- id: x` sirve para fijar la posición de un agente del
  catálogo.
- **Errores**: un error en una entrada (regex inválida, `extends` desconocido o cíclico, falta
  `cmd`, `setup_hooks` desconocido…) solo inutiliza esa entrada; se ve en `agenterm profiles` y en
  *Ajustes*. Los errores de estructura (YAML inválido, id vacío o duplicado) invalidan el archivo entero: los
  comandos que lo leen fallan hasta que lo corrijas.

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
  cmd: $HOME/bin/mi-agente
  args: []
  env: {}
  status: heuristic         # hooks | heuristic
  setup_hooks: none         # claude | codex | none
  idle_seconds: 3
  attention_patterns:
    - '\(y/n\)'
```

### Edición desde la app

*Ajustes → Agentes* (⌘,, el ⚙ de la sidebar o *agenterm → Preferencias…*) permite activar y
desactivar, reordenar (arrastrar o ▲▼), *Editar* (nombre, comando, argumentos, entorno
`CLAVE=valor`, modo de estado, integración de hooks, segundos de inactividad y patrones de
atención), *Duplicar como variante*, *+ Agente personalizado*, *Borrar* (perfiles propios) o
*Restablecer* (agentes del catálogo modificados) y *Re-escanear*.

Al guardar desde la app (RPC `profile.save`, `profile.delete`, `profile.reorder`) el archivo se
reescribe en **formato canónico**: una entrada por perfil en el orden del menú, solo con los campos
que difieren de lo heredado (los agentes del catálogo sin cambios quedan como `- id: x`) y sin
`order`. **Los comentarios no se conservan.** El archivo se relee en cada `spawn` y cada listado,
así que también puedes editarlo a mano.

## `settings.yaml`

Todas las claves son opcionales; las que faltan toman el valor por defecto.

| clave | por defecto | |
|---|---|---|
| `notify.enabled` | `true` | notificaciones (de la app y del daemon) |
| `notify.sound` | `true` | sonido, solo en aprobaciones y preguntas |
| `notify.notify_on_done` | `true` | avisar también cuando un agente termina su turno (los errores avisan siempre) |
| `notify.notify_on_stall` | `true` | aviso discreto cuando un turno parece trabado |
| `activity.stall_seconds` | `180` | segundos sin salida ni eventos para marcar «¿trabado?»; `0` lo desactiva |
| `git.editor` | `""` | para «Abrir en editor»: un comando en el `PATH` (`code`, `cursor -n`, `zed`), una ruta absoluta o el nombre de una app de macOS (`Visual Studio Code`, `Cursor.app`). Vacío: `code` si está en el `PATH`, si no Finder |
| `git.worktree_dir` | `""` | dónde crear los worktrees (ruta absoluta o `~/…`). Vacío: `~/.agenterm/worktrees` (o `$AGENTERM_HOME/worktrees`) |
| `git.default_mode` | `same` | `same` o `worktree`: lo que propone el menú **+** (y ⌘N) en un proyecto donde aún no elegiste |

```yaml
notify:
  enabled: true
  sound: true
  notify_on_done: true
  notify_on_stall: true
activity:
  stall_seconds: 180
git:
  editor: ""
  worktree_dir: ""
  default_mode: same
```

Cuándo se lee:

- `notify.*` se relee en cada notificación (en la app y en el daemon).
- `activity.stall_seconds` se lee al lanzar cada sesión: afecta a las sesiones nuevas.
- `git.worktree_dir` se lee al crear cada worktree (y al listar los de un proyecto).
- `git.editor` se lee en cada «Abrir en editor».

*Ajustes → Notificaciones* edita las claves `notify.*` y `activity.stall_seconds` (RPC
`settings.save`). Al guardar, el archivo se edita en su sitio: **se conservan** las claves
desconocidas y los comentarios, y los campos que un cliente no envía mantienen su valor. Si el
archivo no se puede interpretar se usan los valores por defecto (y se anota en el log); al guardar
se reemplaza.

> Las claves `git.*` todavía no se pueden editar desde la app (ver
> [limitaciones conocidas](development.md#limitaciones-conocidas)): edítalas en el archivo.
