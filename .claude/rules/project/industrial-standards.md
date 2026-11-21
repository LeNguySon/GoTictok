---
paths:
  - "**/*.go"
  - "**/go.mod"
  - "**/go.sum"
---
# Industrial-Grade Go

> Project rule. It extends the vendored ECC rules in `../ecc/` and wins where
> the two disagree.

"It compiles and the demo works" is not the bar. Code merged here must be fit
to run unattended in production and to be maintained by someone who has never
seen it. Write every change as if it ships tonight and you are not on call.

## Reference standards

When this file does not answer a question, these decide, in this order:

1. [Effective Go](https://go.dev/doc/effective_go) and
   [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)
2. [Google Go Style Guide](https://google.github.io/styleguide/go/)
3. [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
4. [Organizing a Go module](https://go.dev/doc/modules/layout)
5. [The Twelve-Factor App](https://12factor.net) for configuration and process behaviour
6. [OWASP ASVS](https://owasp.org/www-project-application-security-verification-standard/) for security
7. [Semantic Versioning](https://semver.org) and
   [Conventional Commits](https://www.conventionalcommits.org) for releases and history

## Enforced by the gate

These fail `go run ./tools/eccgate verify`. Do not argue with them; fix the code.

| Standard | Check |
|----------|-------|
| gofmt and goimports formatting | `fmt`, `lint` |
| Every error handled, wrapped with context, compared with `errors.Is/As` | errcheck, wrapcheck, errorlint, nilerr, errname |
| `context.Context` first, propagated, never stored in a struct, no context-less I/O | contextcheck, containedctx, fatcontext, noctx, revive |
| Named constants instead of magic numbers | mnd |
| No mutable package state, no `init()`, `os.Exit` and `log.Fatal` only in `main` | gochecknoglobals, gochecknoinits, revive deep-exit |
| Functions under 50 lines, at most 5 parameters and 3 results, complexity under 15 | funlen, gocyclo, revive |
| Files under 800 lines, nesting no deeper than 4 | revive, nestif |
| Interfaces of at most 5 methods (aim for 1 to 3); return concrete types | interfacebloat, ireturn |
| Switches over enums are exhaustive | exhaustive |
| No copy-pasted blocks | dupl |
| Bodies, rows and statements are closed; row errors checked | bodyclose, sqlclosecheck, rowserrcheck |
| No deprecated or superseded packages (`io/ioutil`, `github.com/pkg/errors`) | depguard |
| No `replace` directives pointing at local paths | gomoddirectives |
| Static security analysis | gosec |
| Exported identifiers and packages documented | revive |
| Every package tested, `-race` clean, total coverage at least 80% | `tests-exist`, `test` |
| Every direct dependency has a recorded decision | `deps` |
| No known-vulnerable dependency, tidy module files | `vuln`, `mod` |
| A suppression names its linter and gives a reason | nolintlint |

## Required, verified in review

A machine cannot check these. The reviewer roles (`go-reviewer`,
`code-reviewer`, `security-reviewer`) and the human reviewer do.

### Design

- `cmd/<name>/main.go` only wires things together. Logic lives under
  `internal/`, packaged by domain, not by layer name.
- Dependencies point inward: transport, then service, then storage. Business
  logic never imports a transport or a database driver.
- Interfaces are declared by the package that consumes them. Constructors
  validate their input and return concrete types.
- Make the zero value useful or make construction mandatory; never leave a
  half-initialised value reachable.

### Reliability

- Every outbound call (HTTP, database, RPC, lock) carries a deadline from the
  context. Servers set `ReadHeaderTimeout`, read, write and idle timeouts, and
  a request size limit.
- Retries are bounded, use backoff with jitter, and apply only to idempotent
  operations.
- Every goroutine has an owner, a way to be stopped, and is waited for
  (`errgroup` or `sync.WaitGroup`). Every channel has one clear closer.
- No unbounded queue, map or slice fed by external input.
- Shut down gracefully on SIGINT and SIGTERM: stop accepting work, drain, exit.
- Check `Close` errors on anything that was written to.

### Observability

- Structured logging with `log/slog`: constant message, key-value attributes,
  no formatted strings, no secrets or personal data.
- Log an error once, at the boundary that handles it. Lower layers wrap and
  return.
- Carry a request or trace identifier in the context. Servers expose health
  and readiness endpoints and measure latency, errors and throughput at their
  boundaries.

### Configuration and data

- Configuration comes from the environment, is parsed into a typed struct, and
  is validated at startup. Fail fast with a message that names the variable.
- SQL is parameterised without exception. Schema changes are versioned,
  forward-only migrations. Multi-step writes run in a transaction.
- Validate all external input at the boundary; return errors that do not leak
  internals.

### Testing depth

- Table-driven unit tests, integration tests for every I/O boundary (behind
  the `integration` build tag), fuzz tests for anything that parses untrusted
  input.
- Tests are deterministic: no sleeps, no real network, no wall clock. Inject
  the clock and the transport.
- A bug fix starts with a test that reproduces the bug.

### Performance

- Measure before optimising: a benchmark (`go test -bench`) or a profile
  justifies every optimisation. Preallocate when the size is known.

### Compatibility and documentation

- Keep the exported surface minimal. A breaking change is marked with `!` in
  the commit type and bumps the major version.
- Comments explain why, not what. A decision that is not obvious from the code
  is recorded under `docs/research/` (see `research-first.md`).

## Production-readiness checklist

Before calling a feature done, in addition to `AGENTS.md` section 8:

- [ ] Timeouts on every outbound call; cancellation propagates end to end
- [ ] Failure paths tested, not only the happy path
- [ ] No goroutine can outlive its owner; `-race` is clean
- [ ] Inputs validated and bounded; errors do not leak internals
- [ ] Logs are structured, useful at 3 a.m., and free of secrets
- [ ] Configuration validated at startup
- [ ] No new dependency without an entry in `docs/research/dependencies.md`
