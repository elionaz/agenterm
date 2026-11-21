# Mensajes entre sesiones

Los agentes de un proyecto pueden escribirse a través de panal, por ejemplo "ya mergeé #41,
rebasea #42 sobre main", y así no tienes que pasar tú el mensaje. Los mensajes nunca salen de su
proyecto, funcionan con cualquier CLI de agente y todos se ven en la app.

## Desde un agente

Cada sesión recibe `PANAL_SESSION` y `PANAL_SOCK`, así que `panal` ejecutado dentro de
ella llega a su propio daemon:

| comando | |
|---|---|
| `panal msg peers` | las otras sesiones de este proyecto: id, nombre, agente, estado, qué está haciendo cada una, y si tiene su propio worktree o comparte tu carpeta |
| `panal msg send <a> <texto…>` | `<a>` es un id de sesión, un nombre de `msg peers` (único en el proyecto, sin importar mayúsculas) o `all` (todas las demás sesiones del proyecto menos las shells, que nunca leen mensajes solas). Con el texto `-` se lee de stdin (máximo 4 KiB) |
| `panal msg read` | tus mensajes nuevos, que quedan marcados como entregados |
| `panal msg list [--pending]` | los mensajes que enviaste y recibiste |
| `panal wait <id> [--timeout 30m]` | espera hasta que esa sesión quede inactiva, esperando a su usuario o terminada (código de salida 2 si se agota el tiempo) |

Los agentes con integración también reciben lo mismo como herramientas MCP (`peers`, `msg_send`,
`msg_read`, `wait`…): ver [mcp.md](mcp.md).

Fuera de una sesión (desde tu propia terminal), `msg peers` y `msg send` necesitan saber el
proyecto: usa `--session <id>` para `peers`, un id de sesión como `<a>`, o `--project <id>` con un
nombre o `all`. Esos mensajes vienen *del usuario*. `msg read` solo funciona dentro de una sesión.

## Cómo llegan

- **Claude Code** los recibe solo, con los hooks que panal ya agrega a las sesiones que abre (no
  se escribe nada en tu configuración de Claude ni en el proyecto):
  - al empezar la sesión, una presentación breve: qué sesión es, qué otras sesiones tiene el
    proyecto y cómo usar `panal msg`;
  - al enviar un prompt y después de cada una de sus herramientas (no las de un subagente), los
    mensajes nuevos, agregados a su contexto;
  - cuando su turno iba a terminar y acaba de llegar un mensaje, el turno sigue una vez con el
    mensaje, así que nada espera a que vuelvas a escribir. Un turno que ya siguió así termina
    normalmente, y lo que llegue entonces espera al siguiente prompt o herramienta: dos agentes no
    pueden mantenerse trabajando el uno al otro sin fin.
- **Gemini CLI** los recibe igual con sus hooks (`SessionStart`, `BeforeAgent` al enviar un prompt,
  `AfterAgent` cuando su turno iba a terminar), salvo después de cada herramienta: Gemini no tiene
  un hook que agregue a la conversación ahí.

  Un mensaje cuenta como entregado solo cuando el hook lo imprimió: si el hook se pasa de tiempo,
  queda pendiente.
- **Los demás agentes (codex, …)** no tienen un hook que pueda agregar a su conversación. Leen sus
  mensajes con `panal msg read`. Mientras haya un mensaje pendiente, la app lo muestra en su
  pestaña **Mensajes** con un botón **Entregar**.
- **Entregar** escribe una línea fija en la terminal del agente y pulsa Enter: *Hay mensajes nuevos
  para ti de otras sesiones de este proyecto: ejecuta `panal msg read` para leerlos* (para Claude,
  solo la primera parte: luego sus hooks le entregan los mensajes). Nunca escribe el mensaje, y
  panal solo lo hace cuando haces clic. Solo se ofrece mientras el agente está inactivo en su
  prompt, nunca mientras espera una aprobación o una respuesta, que la línea y su Enter
  contestarían. Las shells no la reciben, porque ejecutarían la línea como un comando: sus mensajes
  se quedan en la app.

## Automático o manual, por proyecto

Cada proyecto elige cómo viajan sus mensajes, con el botón **⇄** (automático) / **⏸** (manual) junto a su
nombre en la barra lateral (o en su conversación, o con
`panal project set <proyecto> --messages auto|manual|default`):

- **Automático:** el mensaje le llega a su sesión en cuanto puede recibirlo. Una sesión de Claude
  que está trabajando lo recibe en su siguiente paso, por sus hooks. A una inactiva en su prompt se
  la despierta: panal hace lo que **Entregar** (escribe la línea fija y Enter) un momento después.
  Eso vale para Claude Code y Gemini CLI, cuyos hooks entregan entonces los mensajes, y para los
  agentes que avisan el fin de su turno (codex con su notify, opencode con su plugin), a los que se
  les pide correr `panal msg read`. Gemini CLI solo lo recibe cuando avisó el fin de su turno, ya
  que una salida quieta no significa que terminó. Cualquier mensaje la despierta, también un aviso:
  los agentes lo saben, y se les pide no mandar mensajes que no necesitan respuesta.
- **Manual:** los mensajes de las sesiones quedan **retenidos** hasta que los entregas tú. No le
  llega nada al destino, ni por sus hooks ni con `panal msg read` (que dice cuántos esperan por ti),
  hasta que pulsas **Entregar** para esa sesión (en su pestaña Mensajes o en la conversación) o
  **Entregar a N sesiones** en la conversación. Al entregar, la sesión puede tomarlos: un agente que
  está trabajando los recibe por sus hooks, y a uno inactivo en su prompt se le avisa, como con
  **Entregar**. Tus propios mensajes nunca se retienen.

Un proyecto que no eligió sigue **Ajustes → General → Entregar solos los mensajes a las sesiones en
espera** (`messages.auto_deliver`, **activado por defecto**, o sea automático). Pasar un proyecto a
automático deja salir solo lo que estaba retenido.

Aun en automático, panal se frena cuando escribir una línea podría salir mal, y deja el mensaje
pendiente con **Entregar ahora**:

- el agente no tiene hooks de mensajes y solo se quedó callado (panal adivinó por el silencio que
  está inactivo): puede seguir pensando, y una línea escrita ahora caería en medio de su trabajo;
- la sesión es un shell (la línea se ejecutaría como comando), está trabajando, o espera una
  aprobación o una respuesta;
- lo último que se escribió en ella no fue un Enter solo (o Ctrl-C): su línea puede tener texto que
  no enviaste, que la línea y su Enter mandarían. Una línea nueva en un borrador (Shift+Enter,
  Option+Enter, `\` Enter) sigue contando como borrador. Enviarlo (o borrarlo con Ctrl-C) lo
  destraba en unos segundos;
- escribiste en ella hace unos segundos;
- ya se le entregó automáticamente 5 veces desde la última vez que hiciste algo en esa sesión
  (enviaste un prompt, le mandaste un mensaje o pulsaste **Entregar**), lo que reinicia la cuenta;
  y, pase lo que pase, como mucho 20 entregas automáticas por proyecto cada 10 minutos. Así los
  agentes no se despiertan entre ellos para siempre: la sesión dice que está en pausa, y
  **Entregar** la reanuda.

La pestaña **Mensajes** dice cuál de estos casos aplica (o que van en camino), así un mensaje que
espera nunca parece trabado sin motivo.

## En la app

- La pestaña **Mensajes**, junto a Terminal y Cambios, lista lo que la sesión envió y recibió, con
  quién lo mandó, cuándo y si se entregó. Desde ahí puedes escribirle a la sesión (⌘↩ envía); tus
  mensajes quedan marcados como tuyos.
- **La conversación del proyecto** (💬 junto al nombre del proyecto): todos los mensajes entre sus
  sesiones y los tuyos, en orden, cada sesión con su color, y si *espera por ti* (manual), va *en
  camino* o está *entregado*. Puedes filtrar por sesión, entregar lo retenido, cambiar el modo y
  escribirle a una sesión o a todas (⌘↩ envía).
- **Quién le habla a quién, en el momento:** cuando se envía un mensaje, la fila del que lo manda
  en la barra lateral muestra `→ fake-b: el comienzo del mensaje` y la del que lo recibe
  `← fake-a: …` durante unos segundos, con el nombre de la otra sesión en su color (⏸ si está
  retenido).
- Una sesión con mensajes esperando muestra en el sidebar un sobre con la cantidad.
- **Ajustes → General → Mensajes entre sesiones** (`messages.enabled`, activado por defecto) lo
  apaga todo: `msg send` se rechaza y los hooks no entregan nada.

## Seguridad

- **El daemon decide quién envió un mensaje.** Mira el proceso al otro lado del socket: una llamada
  desde dentro de una sesión (el proceso de su terminal, o cualquier cosa que haya lanzado) viene de
  esa sesión, diga lo que diga su entorno. Cualquier otra cosa viene del usuario.
- **Es atribución, no un aislamiento.** Todas las sesiones corren con tu usuario de macOS, y
  cualquier proceso tuyo ya puede manejar panal (`panal attach` escribe en una terminal). Un
  proceso que sale a propósito del árbol de procesos de su sesión (un daemon que se desacopla, tmux
  abierto en una sesión) cuenta como el usuario y puede lo mismo que él: escribir a cualquier
  proyecto, listar todos los mensajes. Del mismo modo, panal abierto desde dentro de una sesión
  (una build de desarrollo) se toma por esa sesión.
- **Los mensajes son información, no órdenes.** Los agentes los reciben dentro de elementos
  `<panal-message session="…" from="…">`, con un preámbulo que dice que vienen de otros agentes y
  no de su usuario, cuyas instrucciones van primero, y cómo responder. Todo lo que en un mensaje se
  parezca a esa etiqueta se escapa, así que un mensaje no puede hacerse pasar por otro.
- **No salen del proyecto.** Una llamada desde una sesión solo puede escribir a sesiones de su propio
  proyecto, y solo ve los mensajes de esa sesión.
- Se guardan en `~/.panal/messages.json` (`$PANAL_HOME/messages.json`): como mucho 200 por
  proyecto (se descartan los más viejos), de 4 KiB cada uno. Los pendientes de una sesión que se
  elimina se descartan; los entregados se quedan como historial. Si el archivo no se puede leer
  (dañado, o escrito por un panal más nuevo), el daemon arranca igual: no toca el archivo y
  guarda los mensajes en memoria hasta que se reinicie.
