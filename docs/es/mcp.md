# Herramientas de panal para los agentes (MCP)

Las sesiones de agente reciben **el servidor MCP propio de panal**, llamado `panal`. Sus
herramientas le permiten a un agente coordinarse con las otras sesiones de su proyecto y leer el
estado de panal, sin correr comandos `panal` en su shell ni conocer su sintaxis.

| herramienta | qué hace |
|---|---|
| `peers` | las otras sesiones del proyecto: id, nombre, agente, estado, qué están haciendo, rama o carpeta |
| `msg_send` | manda un mensaje a una sesión (`to`: su id, su nombre o `all`); avisa cuando el mensaje queda esperándote (proyectos en modo manual) |
| `msg_read` | tus mensajes nuevos, marcados como entregados (y cuántos esperan a tu usuario) |
| `wait` | espera hasta que una sesión termina su turno, necesita a su usuario o sale (`timeoutSec`: 300 por defecto, 600 como máximo) |
| `session_status` | el estado y los cambios de una sesión: rama, archivos, commits por delante, sin commitear y si ya está mergeada; por defecto, la tuya |
| `history_search` | busca en las sesiones que terminaron (en este proyecto, salvo con `allProjects`) |
| `history_get` | lee una sesión terminada: sus prompts y el final de su terminal |

Solo lee y manda mensajes: nunca arranca, detiene ni cambia sesiones. Los mensajes siguen las
mismas reglas que `panal msg` (ver [messages.md](messages.md)): automáticos o manuales por
proyecto, y todos se ven en la app.

## Qué agentes lo reciben

panal agrega el servidor **solo para esa sesión**. Tu configuración propia del agente no se toca:

| agente | cómo |
|---|---|
| Claude Code | `--mcp-config ~/.panal/hooks/mcp.json`, junto al servidor del navegador |
| codex | `-c mcp_servers.panal.command=…` / `-c mcp_servers.panal.args=…` |
| Gemini CLI | `mcpServers.panal` en los ajustes de sistema que panal le pasa (los servidores de tus ajustes de sistema reales se quedan) |
| opencode | `mcp.panal` en su configuración en línea (`OPENCODE_CONFIG_CONTENT`) |
| Cursor CLI | `mcp.json` en el plugin que panal carga con `--plugin-dir` (Cursor puede pedirte que apruebes el servidor la primera vez) |

Otros agentes, y cualquier cosa que corras tú, pueden arrancarlo con `panal mcp`: MCP por stdio,
ligado al daemon de la sesión donde corre. El daemon sabe qué sesión llama por el árbol de
procesos (el servidor es hijo del agente), igual que con `panal msg`.

El primer mensaje de los agentes (el que les presenta las otras sesiones) apunta a estas
herramientas cuando la sesión las tiene, y a `panal msg` si no.

## Apagarlo

**Ajustes → General → Dar a los agentes las herramientas de panal (MCP)**, o en
`settings.yaml`:

```yaml
mcp:
  enabled: false
```

Aplica a las sesiones que arrancan después del cambio.
