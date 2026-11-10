# Rules catalog

The binary embeds the core pack `tripcord/core@2026.11`
([`rules/pack.yaml`](../../rules/pack.yaml)). Each rule is one
`rules/<ID>.yaml` file: a CEL `when` expression over the items the
[adapters](../adapters/README.md) normalize, a catalog `severity`, the
harnesses it covers, and a fix. The per-rule pages below give file
lists, examples, and limits. The pack's own index, with a longer
"Detects" summary per rule, is
[`rules/README.md`](../../rules/README.md).

Catalog severity is the `severity` field of each rule file and the floor
a policy lock uses. [TC-CFG-004](TC-CFG-004.md),
[TC-CFG-005](TC-CFG-005.md), [TC-CFG-006](TC-CFG-006.md),
[TC-CFG-007](TC-CFG-007.md), [TC-CFG-016](TC-CFG-016.md), and
[TC-CFG-023](TC-CFG-023.md) also set `severity_when`, which is tried in
order once `when` is true: the first true clause is the severity of that
match, and an item no clause accepts is not a match. A match of
TC-CFG-004 through TC-CFG-007 can be medium, and a match of TC-CFG-023
critical; every TC-CFG-016 clause is high except the embed written by
another Tripcord version (`outdated` or `unverified`), which is low. A policy severity override
replaces the per-match result ([Policy](../policy.md)).

On the wiki, the tables below are generated from `rules/*.yaml` and
`rules/ADVISORIES.json` by `cmd/wikigen`, so they always match the pack
on `main`.

<!-- wiki:rules-catalog -->

## Known gaps

Two documented ids have no `rules/<ID>.yaml`:

- [TC-CFG-013](TC-CFG-013.md), an AI coding-agent workflow triggered by
  an event a non-collaborator can produce, is a known gap: it is
  documentation-only in v0. Coverage is meant to come from zizmor's
  `agentic-actions` audit once it is released
  ([#41](https://github.com/vgmartinez/tripcord/issues/41)).
- [TC-CFG-014](TC-CFG-014.md), an agent-config change in an
  agent-authored pull request with no human approval, is not a CEL rule.
  [`tripcord pr-context`](../pr-context.md#tc-cfg-014) evaluates it.

Ids TC-CFG-017 through TC-CFG-022 are not allocated. Every allocated id
is in [`rules/REGISTRY`](../../rules/REGISTRY), which is append-only.

## Writing a rule

The rule format, the id registry, and fixtures are described in
[CONTRIBUTING](../../CONTRIBUTING.md#rules). Run one rule's fixtures
with:

```sh
make test-rule ID=TC-CFG-001
```
