# ADR 0042: A consumed module may declare standalone development

Status: Accepted

Supersedes: ADR 0027 (in part)

Date: 2026-10-15

## Context

ADR 0027 decided that a repository consumed through a `git-submodule-consumer`
relationship is materialized only as the superproject's gitlink and gets no
standalone checkout. That decision matched the estates it described: the
affected repositories were developed inside their consuming superproject, and
every second checkout was an accident that forked the repository identity into
two drifting Git stores.

A second, deliberate shape exists and is now common. Some modules keep their
own development checkout — the standalone clone where working commits are
made, reviewed and pushed — while consumers pin the same repository as a
gitlink at a reviewed commit. The estate that consumes them even forbids
editing a public module through its submodule worktree, so the standalone
checkout is not redundant: it is the only place the module can be changed.
Embedded and standalone HEADs differ on purpose; the pin is an ancestor of the
development line, not a competing line.

Under the unamended rule both checkouts are flagged: `GDS_WORKSPACE_EMBEDDED_ONLY`
for the standalone placement and `GDS_IDENTITY_INDEX_ID_CONFLICT` for the two
claims on one stable identity. Nothing in the observed set was drift; the audit
could not express the difference between an accidental duplicate and a
declared development checkout.

## Decision

1. The repository-owned `module` block gains an optional `development` field
   with values `embedded` and `standalone`. Absent or `embedded` preserves the
   ADR 0027 contract exactly: development happens inside the consuming
   superproject and no standalone checkout may exist.
2. `development: standalone` declares that the module is developed in its own
   checkout. Its standalone checkout is audited as an ordinary standalone
   placement, and its embedded materializations remain the consumers'
   gitlinks. This is declared intent, not a suppression flag: the anchor is
   the repository's own statement of where it is worked on.
3. A `GDS_IDENTITY_INDEX_ID_CONFLICT` pair is suppressed only when the claimed
   repository declares `development: standalone` and at least one side of the
   pair is an embedded checkout. Two standalone checkouts claiming one
   identity, and any dual claim by a module without the declaration, remain
   conflicts.
4. ADR 0027's guardrail still applies in full to the undeclared case: an
   undeclared standalone checkout of a submodule-consumed repository is a
   defect, and removing one still requires its commits to be preserved into
   the module's own Git store first.

## Consequences

- A device root can once more be read as intent: a standalone checkout of a
  consumed module is either declared development or a defect, and the audit
  states which.
- Publishing work stays the same: commits are pushed from the module's own
  checkout to its own remote before any consumer gitlink advances.
- The declaration is one boolean choice, not a placement list; a second
  undeclared standalone clone of the same module still conflicts.

## Rollback

Remove `development` from the affected anchors. The module then validates and
audits under the unchanged ADR 0027 semantics because the field is optional.
