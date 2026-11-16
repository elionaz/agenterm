# Desarrollo

[English](../development.md) · **Español**

Requisitos: los de [README.es.md](../../README.es.md#compilar-desde-el-código-fuente) (Go, Node 24+ con npm y la CLI de
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
npm run check   # svelte-check + scripts/check-i18n.ts (requiere Node 22.18+)
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
- `internal/daemon/persist_test.go` cubre la persistencia: una sesión de shell escribe salida, el
  daemon se detiene (`stop --force` y cancelación), un daemon nuevo sobre el mismo `AGENTERM_HOME`
  la lista como `suspended` con su scrollback reproducido, la reanuda (misma carpeta, línea «sesión
  restaurada») y descarta otra; las políticas `always`/`never`; un script falso con
  `setup_hooks: claude` que comprueba que `--resume <session_id>` va tras la inyección de
  `--settings`; reanudaciones fallidas y un `sessions.json` corrupto. Nunca se lanza un agente real.
- Los tests de `internal/gitx` usan repos reales en directorios temporales con la configuración de
  git aislada (`GIT_CONFIG_NOSYSTEM=1`, `GIT_CONFIG_GLOBAL` vacío, `HOME` temporal).
- `go test` nunca envía notificaciones reales: los daemons de prueba reciben un notificador vacío o
  que solo registra (un `daemon.Config` sin él no envía nada), los tests de la app usan un
  escritorio falso, y los tests de `app`, `internal/daemon` e `internal/daemoncmd` ponen
  `AGENTERM_NOTIFY=off` en `TestMain`. Mantenlo así en los tests nuevos.

## Probar sin tocar tu instalación

Usa un `AGENTERM_HOME` propio y define `AGENTERM_NOTIFY=off`. Es la regla para **cualquier**
instancia de prueba o demo (daemon, app o CLI que pueda arrancar un daemon):

- `AGENTERM_HOME` tiene que ser una ruta **corta** (`/tmp/…`): en macOS la ruta de un socket unix
  tiene un límite de unos 104 bytes y las carpetas temporales de `/var/folders/…` se pasan.
- `AGENTERM_NOTIFY=off` desactiva todas las notificaciones de escritorio de esa app y ese daemon
  (nativas y la alternativa con `osascript`), diga lo que diga su `settings.yaml`, para que las
  sesiones de prueba no te llenen el escritorio de avisos (ni de Editor de Scripts). El daemon lo
  hereda de quien lo arranca.

```sh
export AGENTERM_HOME=/tmp/agt AGENTERM_NOTIFY=off
go run ./cmd/agenterm project add .
go run ./cmd/agenterm spawn agenterm shell
go run ./cmd/agenterm ls
go run ./cmd/agenterm stop --force      # apaga solo el daemon de /tmp/agt

cd app && AGENTERM_HOME=/tmp/agt AGENTERM_NOTIFY=off wails dev   # la app contra ese mismo daemon
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

## Publicar una release

La versión tiene una única fuente de verdad: el tag de git `vX.Y.Z`. Los builds de release la
inyectan con `-ldflags -X` en `internal/version` (lo que muestran `agenterm version` y el panel
Acerca de de la app) y la escriben en el `Info.plist` del bundle (`CFBundleShortVersionString` y
`CFBundleVersion`). El `productVersion` de `app/wails.json` solo lo usa un `wails build` normal; los
builds sin ldflags dicen `dev`.

1. Mueve las entradas de `[Unreleased]` de [CHANGELOG.md](../../CHANGELOG.md) a una sección
   `## [X.Y.Z] - fecha` (y actualiza los enlaces del final), y mergéalo a `main`.
2. Etiqueta `main` y sube el tag:

   ```sh
   git switch main && git pull
   git tag -a v0.1.0 -m "agenterm 0.1.0"
   git push origin v0.1.0
   ```

3. `.github/workflows/release.yml` corre con el tag (runner de macOS) y:
   - compila todo con `scripts/release-local.sh <versión>` (el mismo script que corres en local): la
     app universal (`wails build -platform darwin/universal`), la CLI universal (cgo, `lipo`)
     incluida en `agenterm.app/Contents/Helpers/agenterm`, y la CLI de Linux (`CGO_ENABLED=0`);
   - firma la CLI y luego la app con el Developer ID, hardened runtime, sello de tiempo seguro y
     `app/build/darwin/entitlements.plist`, si existen los secrets del certificado; si no, deja una
     firma ad-hoc, avisa, y las notas de la release dicen que es un build sin firmar;
   - notariza el zip y luego el dmg con `notarytool` y les grapa el ticket (*staple*), si además
     existen los secrets de App Store Connect; si Apple rechaza el envío, el job falla e imprime el
     `notarytool log`;
   - empaqueta `agenterm-<v>-macos-universal.{zip,dmg}`, `agenterm-<v>-darwin-universal.tar.gz`,
     `agenterm-<v>-linux-{amd64,arm64}.tar.gz` y `checksums.txt`, y los sube al run;
   - crea la release `vX.Y.Z` en GitHub con la sección del CHANGELOG como notas (o notas generadas
     si no hay) y todos los artefactos adjuntos;
   - si existe `HOMEBREW_TAP_TOKEN`, rellena `packaging/homebrew/agenterm.rb.tmpl` con la versión y
     el sha256 del zip (`scripts/render-cask.sh`) y lo sube a `Casks/agenterm.rb` en
     [elionaz/homebrew-tap](https://github.com/elionaz/homebrew-tap) como "agenterm X.Y.Z".

### Secrets

Se configuran en *Settings → Secrets and variables → Actions* del repo, o con `gh secret set`. Todos
son opcionales: sin ellos la release se publica igual, sin firmar, y el cask no se actualiza.

| secret | para qué |
|---|---|
| `MACOS_CERT_P12_BASE64` | el certificado Developer ID Application con su clave privada (`.p12`), en base64 |
| `MACOS_CERT_PASSWORD` | la contraseña del `.p12` |
| `MACOS_SIGN_IDENTITY` | opcional: el nombre de la identidad, p. ej. `Developer ID Application: Jane Doe (TEAMID)`; por defecto, la primera identidad Developer ID Application del `.p12` |
| `APPLE_API_KEY_ID` | id de la clave de la API de App Store Connect (notarización) |
| `APPLE_API_ISSUER_ID` | issuer id de App Store Connect (notarización) |
| `APPLE_API_KEY_P8_BASE64` | el `.p8` de la clave, en base64 (notarización) |
| `HOMEBREW_TAP_TOKEN` | un token de GitHub que pueda hacer push a `elionaz/homebrew-tap` |

**Certificado Developer ID Application** (requiere el rol Account Holder en el Apple Developer
Program):

1. En *Acceso a Llaveros → Asistente de Certificados → Solicitar un certificado de una autoridad de
   certificación…*, escribe tu email, elige "Guardado en el disco" y guarda el
   `.certSigningRequest`.
2. En [developer.apple.com → Certificates](https://developer.apple.com/account/resources/certificates/list),
   pulsa **+**, elige **Developer ID Application** (la Sub-CA G2), sube la solicitud y descarga el
   `.cer`. Haz doble clic para añadirlo a tu llavero de inicio de sesión.
3. En Acceso a Llaveros → *Mis certificados*, clic derecho en "Developer ID Application: … (TEAMID)"
   (debe mostrar su clave privada) → **Exportar…** → `.p12` con una contraseña robusta.
4. Guárdalo:

   ```sh
   base64 -i DeveloperID.p12 | gh secret set MACOS_CERT_P12_BASE64
   gh secret set MACOS_CERT_PASSWORD          # pega la contraseña
   security find-identity -v -p codesigning   # el nombre exacto, para MACOS_SIGN_IDENTITY
   ```

   Después borra el `.p12` exportado.

**Clave de la API de App Store Connect** (notarización):

1. En [App Store Connect](https://appstoreconnect.apple.com/access/integrations/api) → *Users and
   Access → Integrations → App Store Connect API → Team Keys*, genera una clave con el rol
   **Developer**.
2. Descarga `AuthKey_<KEYID>.p8` (Apple solo deja descargarlo una vez) y anota el **Key ID** y el
   **Issuer ID** que aparece encima de la lista.
3. Guárdalos:

   ```sh
   base64 -i AuthKey_XXXXXXXXXX.p8 | gh secret set APPLE_API_KEY_P8_BASE64
   gh secret set APPLE_API_KEY_ID --body XXXXXXXXXX
   gh secret set APPLE_API_ISSUER_ID --body 00000000-0000-0000-0000-000000000000
   ```

**`HOMEBREW_TAP_TOKEN`**: un
[token personal fine-grained](https://github.com/settings/personal-access-tokens/new) con
*Repository access → Only select repositories → elionaz/homebrew-tap* y el permiso
*Contents: Read and write* (nada más). Guárdalo con `gh secret set HOMEBREW_TAP_TOKEN` y renuévalo
antes de que caduque.

### Pruebas en seco

En local (macOS, con los requisitos del README):

```sh
scripts/release-local.sh 0.1.0        # -> dist/ (ignorado por git), firma ad-hoc
codesign --verify --deep --strict --verbose=2 dist/work/agenterm.app
lipo -info dist/work/agenterm.app/Contents/MacOS/agenterm
scripts/render-cask.sh 0.1.0 "$(shasum -a 256 dist/agenterm-0.1.0-macos-universal.zip | cut -d' ' -f1)"
```

Para probar la firma, pasa una identidad de tu llavero: `SIGN_IDENTITY="Developer ID Application: …"
scripts/release-local.sh 0.1.0`. La notarización necesita además `NOTARY_KEY_PATH` (el `.p8`),
`NOTARY_KEY_ID` y `NOTARY_ISSUER_ID`.

En GitHub: *Actions → Release → Run workflow* con una versión. Compila, firma y notariza (si existen
los secrets) y sube los artefactos al run, pero no crea la release ni toca el tap.

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
- **Actualizar no reinicia el daemon**: tras instalar una versión nueva, un daemon que ya estaba
  corriendo sigue con el código viejo hasta `agenterm stop` (o un reinicio de la computadora).
- La **CLI de Linux** se compila y se publica, pero no está probada; ahí no hay app ni
  notificaciones.
- **Un reinicio del daemon (o de la computadora) mata los procesos.** Las sesiones vuelven como
  `suspended`, con su scrollback, y se relanzan al reanudarlas: Claude Code y codex continúan su
  conversación por id, opencode con `--continue` (la *última* sesión del proyecto, que puede no ser
  la misma si hay varias en una carpeta), otros agentes solo con `resume_args`. Una shell vuelve
  en su carpeta inicial, sin lo que estuviera corriendo en ella. codex solo informa su thread id
  tras el primer turno, así que una sesión de codex que nunca terminó un turno empieza de cero.
- **Persistir el scrollback cuesta escrituras en disco**: se reescribe el ring entero (hasta 8 MiB)
  como mucho cada 2 s por sesión mientras produce salida.
- **Notificaciones con la app cerrada**: si el daemon no está dentro del `.app` (lo arrancó una CLI
  instalada fuera del bundle) o el bundle no tiene permiso, las notificaciones no se muestran
  (`daemon.log` dice por qué). Con `notify.fallback: osascript` se envían con `osascript`: aparecen
  como *Editor de Scripts* y al pulsarlas se abre Editor de Scripts, no agenterm.
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
