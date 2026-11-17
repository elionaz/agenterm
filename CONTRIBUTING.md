# Contributing to agenterm

**English** · [Español](CONTRIBUTING.es.md)

Thanks for your interest! These are the rules for contributing. Issues, PRs and comments in English
or Spanish are welcome.

## Workflow

1. **Open an issue first** for non-trivial changes (features, refactors, protocol changes), so we
   can agree on the approach before you invest time.
2. **Fork** the repo and create a branch from `main`: `feat/<topic>`, `fix/<topic>`, `docs/<topic>`.
3. **Open a pull request against `main`.** Nobody can push directly to `main`.
4. Every PR needs:
   - the **maintainer's review** (@elionaz, requested automatically via `CODEOWNERS`): only the
     maintainer can merge PRs;
   - **green CI** (`go vet`, `go test -race`, frontend check);
   - the branch **up to date with `main`** and all conversations resolved.
5. PRs are merged with **squash merge**, so the PR title becomes the commit message.

## Commit messages / PR titles

We use [Conventional Commits](https://www.conventionalcommits.org/):
`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`. Example: `fix: avoid leaking the socket when the daemon shuts down`.

## Before opening the PR

```bash
gofmt -l .                     # must print nothing
go vet ./... && go test -race ./...
cd app/frontend && npm ci && npm run check && npm test && npm run build   # if you touch the frontend
```

- Add or update tests for the behavior you change.
- Update the README and `docs/` if you change commands, flags, paths or `settings.yaml`. The main
  documentation is in English (`README.md`, `docs/`) and the Spanish translation lives in
  `README.es.md` and `docs/es/`: keep both up to date (if you can't write in one of the two
  languages, say so in the PR).
- Keep PRs small and focused on one thing.

## Reporting bugs and proposing ideas

Use the issue templates. For security vulnerabilities **don't open a public issue**: follow
[SECURITY.md](SECURITY.md).

## License

By contributing you agree that your contribution is published under the project's
[MIT license](LICENSE).
