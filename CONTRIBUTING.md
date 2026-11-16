# Contribuir a agenterm

¡Gracias por tu interés! Estas son las reglas para colaborar.

## Flujo de trabajo

1. **Abre un issue primero** para cambios no triviales (features, refactors, cambios de protocolo),
   así acordamos el enfoque antes de que inviertas tiempo.
2. **Haz fork** del repo y crea una rama desde `main`: `feat/<algo>`, `fix/<algo>`, `docs/<algo>`.
3. **Abre un pull request contra `main`.** Nadie puede hacer push directo a `main`.
4. Todo PR necesita:
   - la **aprobación del mantenedor** (@elionaz, vía `CODEOWNERS`);
   - el **CI en verde** (`go vet`, `go test -race`, chequeo del frontend);
   - la rama **al día con `main`** y todas las conversaciones resueltas.
   Un push nuevo invalida las aprobaciones previas.
5. Los PRs se integran con **squash merge**, así que el título del PR será el mensaje del commit.

## Mensajes de commit / títulos de PR

Usamos [Conventional Commits](https://www.conventionalcommits.org/):
`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`. Ejemplo: `fix: evitar fuga del socket al cerrar el daemon`.

## Antes de abrir el PR

```bash
gofmt -l .                     # no debe listar nada
go vet ./... && go test -race ./...
cd app/frontend && npm ci && npm run check && npm run build   # si tocas el frontend
```

- Añade o actualiza tests para el comportamiento que cambies.
- Actualiza el `README.md` si cambias comandos, flags, rutas o `settings.yaml`.
- Mantén los PRs pequeños y enfocados en una sola cosa.

## Reportar bugs y proponer ideas

Usa las plantillas de issue. Para vulnerabilidades de seguridad **no abras un issue público**:
sigue [SECURITY.md](SECURITY.md).

## Licencia

Al contribuir aceptas que tu aportación se publique bajo la [licencia MIT](LICENSE) del proyecto.
