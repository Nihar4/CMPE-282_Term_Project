# Contributing

Thanks for your interest in the Enterprise Knowledge Portal project. This is
primarily a CMPE-282 academic deliverable, but contributions and feedback are
welcome.

## Workflow

1. Fork the repo and create a feature branch:
   ```bash
   git checkout -b feature/<short-name>
   ```
2. Run the local stack to verify your change:
   ```bash
   make up
   make test
   ```
3. Commit with a descriptive, signed message:
   ```bash
   git commit -S -m "feat(file-service): add bulk delete endpoint"
   ```
4. Push and open a pull request against `main`.

## Conventions

- **Go**: `gofmt`, `go vet`, `golangci-lint` clean before pushing.
- **TypeScript**: `eslint` clean. No `dangerouslySetInnerHTML`.
- **Commits**: Conventional Commits (`feat:`, `fix:`, `docs:`, `chore:` …).
- **PRs**: keep them small (<400 LOC diff where possible). Link the
  documentation file you updated.
- **Tests**: every new endpoint should ship with at least one `go test` or
  `pytest` case.

## Reporting Issues

Open a GitHub issue with:

- a short title,
- reproduction steps,
- expected vs. actual behavior,
- relevant logs (`docker compose logs <svc>` or `kubectl logs`).

For security findings, please use the private "Report a vulnerability" flow
in GitHub Security instead of a public issue.

## Code of Conduct

This project follows the
[Contributor Covenant v2.1](https://www.contributor-covenant.org/version/2/1/code_of_conduct/).
Be kind, be inclusive, be technical.
