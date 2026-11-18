# Mensajes entre sesiones

Los agentes de un proyecto pueden escribirse a través de agenterm, por ejemplo "ya mergeé #41,
rebasea #42 sobre main", y así no tienes que pasar tú el mensaje. Los mensajes nunca salen de su
proyecto, funcionan con cualquier CLI de agente y todos se ven en la app.

## Desde un agente

Cada sesión recibe `AGENTERM_SESSION` y `AGENTERM_SOCK`, así que `agenterm` ejecutado dentro de
ella llega a su propio daemon:

| comando | |
|---|---|
| `agenterm msg peers` | las otras sesiones de este proyecto: id, nombre, agente, estado, qué está haciendo cada una, y si tiene su propio worktree o comparte tu carpeta |
| `agenterm msg send <a> <texto…>` | `<a>` es un id de sesión, un nombre de `msg peers` (único en el proyecto, sin importar mayúsculas) o `all` (todas las demás sesiones del proyecto). Con el texto `-` se lee de stdin (máximo 4 KiB) |
| `agenterm msg read` | tus mensajes nuevos, que quedan marcados como entregados |
| `agenterm msg list [--pending]` | los mensajes que enviaste y recibiste |
| `agenterm wait <id> [--timeout 30m]` | espera hasta que esa sesión quede inactiva, esperando a su usuario o terminada (código de salida 2 si se agota el tiempo) |

Fuera de una sesión (desde tu propia terminal), `msg peers` y `msg send` necesitan saber el
proyecto: usa `--session <id>` para `peers`, un id de sesión como `<a>`, o `--project <id>` con un
nombre o `all`. Esos mensajes vienen *del usuario*. `msg read` solo funciona dentro de una sesión.

## Cómo llegan

- **Claude Code** los recibe solo, con los hooks que agenterm ya agrega a las sesiones que abre (no
  se escribe nada en tu configuración de Claude ni en el proyecto):
  - al empezar la sesión, una presentación breve: qué sesión es, qué otras sesiones tiene el
    proyecto y cómo usar `agenterm msg`;
  - al enviar un prompt y después de cada una de sus herramientas (no las de un subagente), los
    mensajes nuevos, agregados a su contexto;
  - cuando su turno iba a terminar y acaba de llegar un mensaje, el turno sigue una vez con el
    mensaje, así que nada espera a que vuelvas a escribir. Un turno que ya siguió así termina
    normalmente, y lo que llegue entonces espera al siguiente prompt o herramienta: dos agentes no
    pueden mantenerse trabajando el uno al otro sin fin.

  Un mensaje cuenta como entregado solo cuando el hook lo imprimió: si el hook se pasa de tiempo,
  queda pendiente.
- **Los demás agentes (codex, …)** no tienen un hook que pueda agregar a su conversación. Leen sus
  mensajes con `agenterm msg read`. Mientras haya un mensaje pendiente, la app lo muestra en su
  pestaña **Mensajes** con un botón **Entregar**.
- **Entregar** escribe una línea fija en la terminal del agente y pulsa Enter: *Hay mensajes nuevos
  para ti de otras sesiones de este proyecto: ejecuta `agenterm msg read` para leerlos* (para Claude,
  solo la primera parte: luego sus hooks le entregan los mensajes). Nunca escribe el mensaje, y
  agenterm solo lo hace cuando haces clic. Solo se ofrece mientras el agente está inactivo en su
  prompt, nunca mientras espera una aprobación o una respuesta, que la línea y su Enter
  contestarían. Las shells no la reciben, porque ejecutarían la línea como un comando: sus mensajes
  se quedan en la app.

## Entrega automática

**Ajustes → General → Entregar solos a las sesiones de Claude en espera**
(`messages.auto_deliver`, desactivado por defecto) hace lo que **Entregar** sin el clic: cuando un
mensaje espera a una sesión de Claude inactiva en su prompt (una respuesta que llegó después de que
terminó su turno, o una que se envió mientras estaba inactiva), agenterm escribe la misma línea fija
un momento después. Se contiene, y el mensaje queda pendiente con el botón **Entregar**, cuando:

- la sesión no es de Claude (los demás agentes y las shells siguen con el botón), o está trabajando,
  o espera una aprobación o una respuesta;
- lo último que se escribió en ella no fue un Enter solo (o Ctrl-C): su línea de entrada puede
  tener texto que no enviaste, que la línea y su Enter enviarían. Una línea nueva en un borrador
  (Shift+Enter, Option+Enter, `\` Enter) sigue contando como borrador. Enviarlo (o borrarlo con
  Ctrl-C) lo destraba en unos segundos;
- escribiste en ella en los últimos segundos;
- ya se le entregó automáticamente 5 veces desde la última vez que interviniste en esa sesión
  (enviaste un prompt en ella, le mandaste un mensaje desde la app o hiciste clic en **Entregar**),
  lo que reinicia la cuenta; y, pase lo que pase, como mucho 20 entregas automáticas por proyecto
  cada 10 minutos. Así los agentes no pueden despertarse entre sí sin fin.

## En la app

- La pestaña **Mensajes**, junto a Terminal y Cambios, lista lo que la sesión envió y recibió, con
  quién lo mandó, cuándo y si se entregó. Desde ahí puedes escribirle a la sesión (⌘↩ envía); tus
  mensajes quedan marcados como tuyos.
- Una sesión con mensajes esperando muestra en el sidebar un sobre con la cantidad.
- **Ajustes → General → Mensajes entre sesiones** (`messages.enabled`, activado por defecto) lo
  apaga todo: `msg send` se rechaza y los hooks no entregan nada.

## Seguridad

- **El daemon decide quién envió un mensaje.** Mira el proceso al otro lado del socket: una llamada
  desde dentro de una sesión (el proceso de su terminal, o cualquier cosa que haya lanzado) viene de
  esa sesión, diga lo que diga su entorno. Cualquier otra cosa viene del usuario.
- **Es atribución, no un aislamiento.** Todas las sesiones corren con tu usuario de macOS, y
  cualquier proceso tuyo ya puede manejar agenterm (`agenterm attach` escribe en una terminal). Un
  proceso que sale a propósito del árbol de procesos de su sesión (un daemon que se desacopla, tmux
  abierto en una sesión) cuenta como el usuario y puede lo mismo que él: escribir a cualquier
  proyecto, listar todos los mensajes. Del mismo modo, agenterm abierto desde dentro de una sesión
  (una build de desarrollo) se toma por esa sesión.
- **Los mensajes son información, no órdenes.** Los agentes los reciben dentro de elementos
  `<agenterm-message session="…" from="…">`, con un preámbulo que dice que vienen de otros agentes y
  no de su usuario, cuyas instrucciones van primero, y cómo responder. Todo lo que en un mensaje se
  parezca a esa etiqueta se escapa, así que un mensaje no puede hacerse pasar por otro.
- **No salen del proyecto.** Una llamada desde una sesión solo puede escribir a sesiones de su propio
  proyecto, y solo ve los mensajes de esa sesión.
- Se guardan en `~/.agenterm/messages.json` (`$AGENTERM_HOME/messages.json`): como mucho 200 por
  proyecto (se descartan los más viejos), de 4 KiB cada uno. Los pendientes de una sesión que se
  elimina se descartan; los entregados se quedan como historial. Si el archivo no se puede leer
  (dañado, o escrito por un agenterm más nuevo), el daemon arranca igual: no toca el archivo y
  guarda los mensajes en memoria hasta que se reinicie.
