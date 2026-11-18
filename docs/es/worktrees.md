# Worktrees y cambios

[English](../worktrees.md) · **Español**

Cada sesión puede trabajar en la carpeta del proyecto o en su propio **worktree de git** (rama y
carpeta aparte), para que varios agentes trabajen en paralelo sobre el mismo repo sin pisarse.

Todo va por la CLI de `git` (con timeouts; nunca libgit2). Las consultas de solo lectura usan
`GIT_OPTIONAL_LOCKS=0`, así que nunca toman `index.lock` mientras el agente usa git en la misma
carpeta.

## Varios agentes en un proyecto

Las sesiones en **la misma carpeta** comparten todo lo que git guarda por carpeta: la rama actual,
los cambios sin commitear y el índice. Lo que edita o commitea un agente lo ven los demás; si uno
cambia de rama, hace stash o commitea, se lleva el trabajo a medias de los otros. El **±N** de la
barra lateral y la pestaña Cambios muestran esa carpeta, así que todas las sesiones de ahí muestran
los mismos archivos.

Para agentes trabajando en paralelo, dale a cada uno su propio **worktree**. Es lo que agenterm
hace por defecto (`git.default_mode: worktree`, desde 0.6.1): un agente nuevo empieza en su propia
rama y carpeta, y un **Shell** simple empieza en la carpeta del proyecto. **Misma carpeta** sigue a
un clic en el menú **+**.

- En un repositorio git, si ya hay un agente (no terminado ni suspendido) trabajando en la carpeta
  del proyecto, el menú **+** elige **Worktree nuevo** para el siguiente agente y dice qué sesión está
  ahí; igual puedes elegir **Misma carpeta**. **⌘N** y dividir un panel (⌘D) hacen lo mismo. La
  sugerencia no se recuerda como el modo del proyecto. Un **Shell** simple ni cuenta ni recibe la
  sugerencia: una terminal junto a un agente en su carpeta está bien.
- Una sesión que sí comparte la carpeta lo dice: la pestaña Cambios muestra *Carpeta compartida* con
  los otros agentes, y también el tooltip del **±N**.
- `agenterm spawn` sin `--worktree` avisa en stderr cuando la sesión nueva comparte la carpeta
  (stdout sigue siendo solo el id).

El trabajo de cada worktree vuelve con **Merge a la base** o **Crear pull request** (abajo).

## Modos de una sesión

- **Misma carpeta** (`same`, por defecto): el agente trabaja en la carpeta del proyecto.
- **Worktree nuevo** (`worktree`): `git worktree add -b <rama> <carpeta> <base>`.
  - **Rama**: por defecto `agent/<perfil>-<4 primeros caracteres del id de la sesión>`. El nombre se
    sanea (espacios y caracteres no válidos → `-`, sin `..`, sin `.lock`…) y, si ya existe, se le
    añade `-2`, `-3`…
  - **Base**: por defecto la rama actual del proyecto (o `main`/`master` si `HEAD` está separado).
    Puede ser cualquier rama o commit.
  - **Carpeta**: `<worktrees>/<proyecto>-<projectId>/<sessionId>/`, donde `<worktrees>` es
    `~/.agenterm/worktrees` (o `git.worktree_dir`) y `<proyecto>` es el nombre del proyecto en
    minúsculas y saneado. Los worktrees **nunca** se crean dentro del proyecto.
  - Se permite aunque la carpeta principal tenga cambios: el worktree parte del commit de la base.
  - Si el proyecto es una subcarpeta del repo, la sesión arranca en esa misma subcarpeta del
    worktree.
  - Errores claros si el proyecto no es un repo git (en la app la opción sale desactivada, con el
    motivo), si el repo no tiene commits todavía o si la base no existe.
- **Reabrir un worktree** (`--in-worktree <id>` en la CLI, *Abrir shell aquí* en la app): una
  sesión nueva en un worktree de agenterm que ya existe.

El id de un worktree es el de la sesión que lo creó.

Las integraciones de hooks funcionan igual en un worktree: Claude Code recibe `--settings` con una
ruta absoluta y no se escribe nada en el worktree ni en el proyecto. Claude Code puede pedir
confianza la primera vez para la carpeta nueva.

## Ciclo de vida

- **Matar una sesión nunca borra su worktree.** El registro (`worktrees.json`) sobrevive a los
  reinicios del daemon.
- Estados de un worktree (`agenterm worktrees <proyecto>`):
  - `live`: una sesión viva lo usa;
  - `exited`: solo lo usan sesiones ya terminadas que siguen en la lista, o sesiones `suspended`
    tras un reinicio del daemon (reanudar una la vuelve a abrir en este worktree);
  - `orphan` (huérfano): ninguna sesión del daemon lo usa (las quitaste o descartaste, o ya habían
    terminado antes de que el daemon se reiniciara).
- Al listar los worktrees de un proyecto, el daemon olvida los que ya no existen en disco (y hace
  `git worktree prune`) y **adopta** los worktrees que git conoce dentro de la carpeta de worktrees
  de ese proyecto pero que el registro no tiene (por ejemplo, si se perdió `worktrees.json`).

## Cambios (diff)

- **Worktree**: `git diff <merge-base con la base>` (lo commiteado y lo no commiteado, en una sola
  lista) más los archivos sin seguimiento no ignorados, y los commits por delante
  (`git log <base>..HEAD`, máximo 200). Si la rama base desaparece se usa el commit base guardado al
  crear el worktree.
- **Misma carpeta**: `git diff HEAD` más los archivos sin seguimiento (contra el árbol vacío si el
  repo no tiene commits).
- Por archivo: ruta, ruta anterior (renombrados), estado `A`/`M`/`D`/`R`/`C`/`T`/`U`, líneas `+`/`−`,
  si es binario (sin contenido) y su parche.
- Límites: **2 MB** de parche en total (el resto de archivos se listan sin contenido y el diff se
  marca `truncated`); los archivos sin seguimiento de más de 1 MB, o más allá de los primeros 300,
  tampoco llevan contenido.
- **Resumen** de cada sesión (`git`: `files`, `uncommitted`, `ahead`): se recalcula de forma
  perezosa (al pasar a inactivo, esperando o terminado, y al pedir el diff), como mucho una vez cada
  3 s por sesión y nunca por cada trozo de salida. Los cambios se publican con el evento
  `session.git`.

## Acciones

- **Merge a la base** (`agenterm worktree merge <id>`): `git merge --no-ff` de la rama en la
  **carpeta principal** del repo. Solo si:
  - la carpeta principal está en la rama base;
  - no tiene cambios en archivos con seguimiento (los sin seguimiento no bloquean; no hay stash
    automático);
  - la rama tiene commits que la base no tiene.

  Si hay conflictos se hace `git merge --abort` y se informa qué archivos chocaron. Los cambios sin
  commitear del worktree no entran en el merge (se avisa cuántos quedaron fuera). En la app, tras un
  merge correcto se ofrece eliminar el worktree y su rama (la confirmación de siempre, con *Borrar
  también la rama* ya marcada).
- **Crear pull request** (`agenterm worktree pr [--no-open] <id>`): `git push -u <remoto> <rama>` y
  luego abre la página del servidor para crear el pull request hacia la base. No usa `gh`, nunca
  hace push forzado ni sube la base.
  - Remoto: el mismo al que haría push git: `branch.<rama>.pushRemote`; si no, `remote.pushDefault`;
    si no, el remoto del upstream de la rama; si no, `origin`; si no, el único remoto. Se niega si no
    hay ninguno (o si hay varios y no aplica ninguna de estas reglas).
  - Se niega si la rama no tiene commits que la base no tenga.
  - El push no es interactivo (`GIT_TERMINAL_PROMPT=0`, ssh en `BatchMode`): si tuviera que pedir
    credenciales, falla con el mensaje de git en lugar de quedarse esperando. Los credential helpers
    y `ssh-agent` siguen funcionando. Si el servidor no responde o se traba, se abandona (timeouts
    de conexión y keep-alive de ssh, una velocidad mínima en http) y el push entero tiene un límite
    de dos minutos.
  - La página: GitHub (`https://github.com/<owner>/<repo>/compare/<base>...<rama>?expand=1`) y
    GitLab (gitlab.com y servidores cuyo nombre contiene `gitlab`: *new merge request*), a partir de
    URLs de remoto http(s), ssh o `git@host:ruta` (`ssh.github.com` y alias de ssh como
    `github.com-work` cuentan como GitHub). Otros servidores: se sube la rama y el pull request
    lo abres tú allí.
  - Los cambios sin commitear del worktree no se suben (se avisa cuántos quedaron fuera).
- **Eliminar worktree** (`agenterm worktree rm <id>`):
  - se niega si una sesión viva lo usa;
  - sin *force* también se niega si se perderían cambios sin commitear o, con *borrar también la
    rama* (`--delete-branch`), commits sin mergear en la base; el error lista qué se perdería;
  - `--dry-run` solo hace la comprobación;
  - tras borrar: `git worktree prune` y, si se pidió, `git branch -D` (seguro: los commits sin
    mergear ya se comprobaron contra la base, salvo con *force*).
- **Abrir en editor** (`git.editor`; por defecto `code` si está en el `PATH`, si no Finder) y
  **Abrir en Finder** (`agenterm worktree open [--finder] <id>`).
- **Copiar rama** (solo en la app, al portapapeles).

## Pull requests en GitHub y GitLab

La pestaña **Cambios** muestra el pull request de la rama, en sesiones con worktree y en sesiones
de la carpeta principal (la rama en la que está; nada en la rama base). Pasa por la CLI del propio
servidor con tu sesión: [`gh`](https://cli.github.com) para GitHub y
[`glab`](https://gitlab.com/gitlab-org/cli) para GitLab (gitlab.com y servidores cuyo nombre
contiene `gitlab`). agenterm no guarda tokens. Sin la CLI, o sin sesión, Cambios explica cómo
activarla (`brew install gh`, `gh auth login`) y mantiene el flujo de subir y abrir la página de
arriba.

- **Estado**: número, abierto / borrador / mergeado / cerrado, checks (en verde, fallando con sus
  nombres, en curso; en GitLab, el pipeline) y la decisión de review. El título enlaza al servidor.
- **Comentarios**: comentarios de la conversación, reviews con texto (aprobado, cambios pedidos) y
  comentarios de línea con `archivo:línea`, que seleccionan ese archivo en el diff.
- **Merge en el servidor**: con los métodos que permite el repositorio (squash, commit de merge,
  rebase) y *borrar la rama en el servidor después*. Avisa de borradores, conflictos, checks que
  fallan o siguen en curso y merges que el servidor podría rechazar; la última palabra es del
  servidor. Nunca toca carpetas ni ramas locales (`gh pr merge` corre sin `--delete-branch`, que
  cambiaría tu carpeta a la base). El merge se rechaza si la carpeta cambió de rama desde que se
  mostró el pull request, o si es la rama de un fork con el mismo nombre (los pull requests de forks
  nunca se muestran como tuyos). Si el servidor solo lo encola (una cola de merge, el auto-merge de
  GitLab cuando pase el pipeline), Cambios lo dice y la rama se conserva hasta que se mergee de
  verdad.
- **Crear pull request**: sube la rama (nunca forzado) y lo abre hacia la base (la del worktree, o
  la rama por defecto del remoto en la carpeta principal) con título y descripción editables
  sugeridos a partir de los commits, opcionalmente como borrador.

Todo se recarga al abrir Cambios, con ⟳ y cuando la rama cambia (commits, pushes); no hay
sondeo periódico. Las CLIs se buscan en el `PATH` del shell de login, como los agentes. El
repositorio es aquel al que se sube la rama: si en un esquema con fork ese es tu fork, los pull
requests se buscan (y se abren) ahí. Los comentarios se leen en páginas de 100, todos.

## En la app

- Menú **+** del proyecto: tras elegir el agente, *Misma carpeta* o *Worktree nuevo* (rama editable
  ya rellena y selector de base con la rama actual por defecto). Se recuerda la última elección por
  proyecto (para agentes; las shells siempre empiezan en la carpeta), y ⌘N la usa; si no hay
  ninguna, se usa `git.default_mode` (`worktree` por defecto). Las elecciones de antes de 0.6.1 ya
  no se recuerdan.
- Fila de la sesión: chip con la rama (sin el prefijo `agent/`): azul para el worktree propio de la
  sesión, gris para la rama actual de la carpeta del proyecto (compartida con las otras sesiones de
  ahí); después `±N` archivos cambiados. Igual en las tarjetas de la vista general, con `↑N` commits
  por delante.
- Pestañas **Terminal | Cambios (N)** en las sesiones con git: lista de archivos a la izquierda
  (estado, `+`/`−`), commits por delante, diff a la derecha con diff2html (lado a lado o unificado),
  ⟳ para recargar, aviso de diff truncado y la barra de acciones (*Abrir en editor*, *Abrir en
  Finder*, *Copiar rama*, *Merge a &lt;base&gt;*, *Crear pull request* (con una confirmación que
  muestra remoto, rama y base), *Eliminar worktree* con una confirmación que
  muestra qué se perdería, la casilla *Borrar también la rama* y otra para forzar).
- Bajo cada proyecto, **Worktrees huérfanos** (si hay): *Ver cambios*, *Abrir shell aquí* (una
  sesión `shell` nueva en ese worktree) y *Eliminar*.
