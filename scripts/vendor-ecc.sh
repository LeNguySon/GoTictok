#!/bin/sh
# Re-vendor the Go subset of ECC (https://github.com/affaan-m/ECC) into this
# repository. Run it by hand when you want to pick up a newer ECC release,
# then review the diff like any other change.
#
#   git clone --depth 1 https://github.com/affaan-m/ECC.git /tmp/ecc
#   sh scripts/vendor-ecc.sh /tmp/ecc
set -eu

ECC="${1:?usage: sh scripts/vendor-ecc.sh <path-to-ECC-checkout>}"
[ -f "$ECC/rules/golang/coding-style.md" ] || { echo "not an ECC checkout: $ECC" >&2; exit 1; }
cd "$(git rev-parse --show-toplevel)"

AGENTS="planner architect tdd-guide code-reviewer security-reviewer go-reviewer go-build-resolver refactor-cleaner silent-failure-hunter code-simplifier"
COMMANDS="plan go-test go-build go-review code-review build-fix refactor-clean feature-dev checkpoint"
SKILLS="golang-patterns golang-testing tdd-workflow verification-loop search-first security-review error-handling api-design"

# Rules keep their directory layout: golang/ files link to ../common/.
rm -rf .claude/rules/ecc/common .claude/rules/ecc/golang
mkdir -p .claude/rules/ecc .claude/agents .claude/commands .claude/skills .cursor/rules
cp -r "$ECC/rules/common" "$ECC/rules/golang" .claude/rules/ecc/
cp "$ECC/LICENSE" .claude/rules/ecc/LICENSE

for a in $AGENTS; do cp "$ECC/agents/$a.md" .claude/agents/; done
for c in $COMMANDS; do cp "$ECC/commands/$c.md" .claude/commands/; done
for s in $SKILLS; do rm -rf ".claude/skills/$s"; cp -r "$ECC/skills/$s" .claude/skills/; done

# ECC ships Cursor-format copies of the rules; Cursor wants the .mdc suffix.
rm -f .cursor/rules/ecc-common-*.mdc .cursor/rules/ecc-golang-*.mdc
for f in "$ECC"/.cursor/rules/common-*.md "$ECC"/.cursor/rules/golang-*.md; do
	cp "$f" ".cursor/rules/ecc-$(basename "${f%.md}").mdc"
done

# Like ECC's own manual installer, drop the plugin-scoped "ecc:" agent prefix:
# here the agents are plain project agents.
grep -rlE '\becc:[a-z]' .claude/rules/ecc .claude/agents .claude/commands .claude/skills .cursor/rules |
	while read -r f; do sed -i -E 's/\becc:([a-z][a-z0-9-]+)/\1/g' "$f"; done

cat > .claude/ecc-vendor.json <<JSON
{
  "source": "https://github.com/affaan-m/ECC",
  "license": "MIT (see .claude/rules/ecc/LICENSE)",
  "version": "$(cat "$ECC/VERSION")",
  "commit": "$(git -C "$ECC" rev-parse HEAD)",
  "vendored": {
    "rules": ["common", "golang"],
    "agents": "$AGENTS",
    "commands": "$COMMANDS",
    "skills": "$SKILLS",
    "cursorRules": ["common-*", "golang-*"]
  },
  "changes": "agent names lose the plugin prefix 'ecc:'; Cursor rules renamed to ecc-*.mdc"
}
JSON
echo "ECC $(cat "$ECC/VERSION") vendored. Review with: git status && git diff"
