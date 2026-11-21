# GoTictok: Copilot instructions

The contract for every AI agent in this repository is `AGENTS.md` at the
repository root. Read it in full and follow it; it overrides these notes.

The essentials:

- Follow the ECC workflow: research (query the Graphify knowledge graph
  first), plan, write the failing test first, implement, review, verify.
- Go rules: gofmt, wrap errors with `%w`, never ignore an error, `ctx` first,
  functions under 50 lines, table-driven tests, coverage at least 80%.
- Code must be industrial-grade (`AGENTS.md` section 9,
  `.claude/rules/project/industrial-standards.md`): deadlines on every outbound
  call, owned goroutines, structured logging, validated configuration, named
  constants, small interfaces.
- On a hard problem, stop and research before planning (`AGENTS.md` section 10,
  `.claude/rules/project/research-first.md`): study at least three sources,
  synthesize one recommendation, record it under `docs/research/`. Every new
  direct dependency must be listed in `docs/research/dependencies.md`.
- Before finishing or committing run `go run ./tools/eccgate verify`; it must
  print `RESULT: READY`.
- Never use `--no-verify`, never change `core.hooksPath`, never edit the gate
  (`.golangci.yml`, `.githooks/`, `tools/eccgate/`, CI workflows) to make it pass.
- Commit messages: `<type>: <description>` (feat, fix, refactor, docs, test,
  chore, perf, ci, build, style, revert).
