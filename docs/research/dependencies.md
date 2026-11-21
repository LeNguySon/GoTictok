# Dependency ledger

Every **direct** dependency in `go.mod` needs a row in this table. The gate's
`deps` check fails for a direct dependency whose module path does not appear
below as a code span (in backticks).

Add a row only after following
[research-first](../../.claude/rules/project/research-first.md): compare the
candidates, check the licence and the maintenance record, and write the
research note the row links to.

| Module | Purpose | Alternatives considered | Licence | Research note |
|--------|---------|-------------------------|---------|---------------|

_No direct dependencies yet: the project uses only the standard library._
