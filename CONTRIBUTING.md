# Contributing to Fuze AI PaaS

Thanks for your interest in contributing to **Fuze AI PaaS**! This
document describes how to get involved, the conventions we follow, and
how to submit changes.

## Code of Conduct

Be respectful and constructive. We aim to keep the project welcoming to
contributors of all backgrounds and experience levels. Harassment or
abusive behavior will not be tolerated.

## How to Contribute

1. **Open an issue first** for non-trivial changes (new features,
   breaking changes, large refactors). This lets maintainers give early
   feedback and avoids wasted effort.
2. **Fork** the repository and create a feature branch from `main`:
   ```bash
   git checkout -b feat/my-change
   ```
3. **Make your change** following the project conventions below.
4. **Add or update tests** that cover your change.
5. **Run the checks** (see [Local Development](#local-development)).
6. **Submit a pull request** with a clear description of the *why* and
   *what*. Link the related issue if any.

## Project Layout

- `backend/` — Go backend (hexagonal architecture): `internal/domain`,
  `internal/app`, `internal/api`, `internal/storage`.
- `frontend/` — React (Vite) frontend.
- `sdk/` — Python SDK (`fuze-ai-sdk`).
- `k8s/` — Kubernetes manifests.
- `docs/` — Design documents.

## Development Conventions

### Backend (Go)

- Follow standard Go formatting: `gofmt` / `goimports`.
- Architecture is **Domain-Driven + Hexagonal**:
  - `domain/<subdomain>`: entities, ports (interfaces), and services.
  - `app/<subdomain>`: application-layer orchestration.
  - `api`: REST transport (Gin handlers + routes).
  - `storage`: port implementations on a single `*Storage`.
- New features add: a `domain` package (ports + entity + service), an
  app service, a storage repo, API handler methods, `routes.go` entries,
  and `bootstrap.go` wiring.
- API handlers must be **nil-safe**: degrade to `501 Not Implemented`
  when a dependency is not injected.
- **Write tests beside the code** (`*_test.go`). Run `go test ./backend/...`.
- Keep tenant isolation: all write operations are scoped by
  `principalTenant`; platform admins may cross tenants.
- Record audit logs for critical operations.

### Frontend (React)

- Use functional components with hooks.
- Co-locate pages under `frontend/src/pages` and shared components under
  `frontend/src/components`.
- Keep API calls through the existing REST client.

### SDK (Python)

- Keep the public API stable and documented in `sdk/README.md`.
- Pin versions in `sdk/python/pyproject.toml`.

## Local Development

Common commands (see `Makefile`):

```bash
make build         # build backend binary + frontend assets
make run           # run backend locally (default SQLite, port 8080)
make test          # run backend unit tests
make frontend-dev  # run frontend dev server (Vite, port 5173)
make test-e2e      # run end-to-end scripts
```

The backend defaults to SQLite (zero-dependency). To use Postgres:

```bash
make db-up
export DB_DRIVER=postgres
export DB_DSN="postgres://fuze:fuze@localhost:5432/fuze?sslmode=disable"
```

Before submitting a PR, please ensure:

- `make test` passes.
- `gofmt` reports no diffs.
- The frontend builds (`make frontend-build`) if you changed it.

## Commit Messages

- Use clear, imperative subject lines (e.g. `fix: handle empty dataset
  in ETL pipeline`).
- Keep the subject under ~72 characters.
- Add a body explaining *why* when the change is non-obvious.

## License

By contributing, you agree that your contributions will be licensed
under the **GNU Affero General Public License v3 (or later)**, the same
license that covers this project. See the `LICENSE` and `NOTICE` files
at the repository root.

You retain copyright on your contributions; we ask only that they be
licensed under the project's license.
