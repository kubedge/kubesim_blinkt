# Tasks — adopt-go-ci

## 1. Class-M delivery (done in #18)
- [x] 1.1 Apply the claude-meta delivery (`/alemax:complete-update`, alemax 0.3.3, #18).
- [x] 1.2 Confirm the class-M set landed: `.editorconfig`, `.gitattributes`, `.github/*`, `dependabot.yml`, `.pre-commit-config.yaml`, `bin/set-secret.sh`.
- [x] 1.3 `citrim`: `ci.yml` reduced to `secret-scan` (no root `go.mod` / `pyproject.toml`).

## 2. Code workflow
- [x] 2.1 Add `.github/workflows/code.yml`: triggers (PR + push to `main`), path filters, concurrency, 30 min job timeouts.
- [x] 2.2 `go` job, matrix `go-blinkt` / `dra-driver`: `setup-go` from the module's `go.mod`, `make <root>-test <root>-lint`.
- [x] 2.3 `operator` job: `make operator-test operator-lint`; cache `blinkt-operator/bin` (setup-envtest + assets).
- [x] 2.4 `rust` job: buildx + GHA cache, `make rust-test rust-lint`; fail on a non-zero cargo exit (check `hack/cargo.sh` keeps it through the `sed` pipe).
- [x] 2.5 `interop` job: `make interop`, then assert the output contains `INTEROP OK`.
- [x] 2.6 `images` job: `make docker-buildx-check` (go), `make docker-buildx-check IMPL=rust`, `make dra-buildx-check`, `make operator-buildx-check`.

## 3. Verify
- [x] 3.1 Push the branch; all code jobs and `secret-scan` green.
- [x] 3.2 Negative check: a throwaway commit breaking one Go test and one clippy lint turns its job red (then drop it).
- [ ] 3.3 Docs-only commit skips the code jobs. (A PR is filtered against its base, so this PR always counts as code; verify on the next docs- or spec-only PR, e.g. the archive of this change.)
- [x] 3.4 README "Develop": one line naming the CI jobs; ask the operator to mark them required on `main`.
