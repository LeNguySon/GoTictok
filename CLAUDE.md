# GoTictok

@AGENTS.md

## Claude Code notes

- The ECC rules in `.claude/rules/ecc/`, the roles in `.claude/agents/` and the
  skills in `.claude/skills/` load automatically. Delegate to them as
  `AGENTS.md` section 7 describes.
- Hooks in `.claude/settings.json` format and vet Go files after each edit,
  block attempts to skip the git hooks, ask a human before the gate's own
  files are changed, and send you back to work if you stop with a red gate.
- The project's own rules are in `.claude/rules/project/`:
  `industrial-standards.md` (what production-grade Go means here) and
  `research-first.md` (what to do when a problem is hard).
- Slash commands: `/research` (hard problems), `/plan`, `/go-test` (TDD),
  `/go-review`, `/go-build`, `/code-review`, `/build-fix`, `/refactor-clean`,
  `/checkpoint`.

## graphify

This project has a knowledge graph at graphify-out/ with god nodes, community structure, and cross-file relationships.

Rules:
- When graphify-out/graph.json exists, run `graphify query "<question>"` before your first grep, search or source-file read for a codebase question, and put this rule in every subagent prompt that explores code. Read raw files only for lines the graph or an earlier read already located. Use `graphify path "<A>" "<B>"` for relationships and `graphify explain "<concept>"` for focused concepts. When the answer is under a Markdown heading, `graphify section "<heading>"` prints that section's text, so the .md need not be opened. These return a scoped subgraph, usually much smaller than GRAPH_REPORT.md or raw grep output.
- If graphify-out/wiki/index.md exists, use it for broad navigation instead of raw source browsing.
- Read graphify-out/GRAPH_REPORT.md only for broad architecture review or when query/path/explain do not surface enough context.
- After modifying code, run `graphify update .` to keep the graph current (AST-only, no API cost).
- A committed graphify-out/graph.json is rewritten whole on every build, so a large diff to it in a pull request is expected. Do not flag it as churn or suggest skipping the graph rebuild or its hooks.
