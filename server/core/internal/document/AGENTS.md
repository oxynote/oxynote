# AGENTS.md

Core-side document invariants: branches, files, hooks, search, tags.
Cross-service storage rules (Hocuspocus, Yjs) live in
[server/AGENTS.md](../../../AGENTS.md); Go standards in
[server/core/AGENTS.md](../../AGENTS.md).

## Branches

- **The main branch is identified by its `default` flag, not its name**: the
  tree join keys on it, and `Document.AllowsBranchRename` refuses to rename
  it. `NewDocument` and `Duplicate` set `Default: true`; forks must not.
- **Maintainers accumulate.** The `maintainers` field on a branch update is
  who edited in that persist; `UpsertDocumentMaintainers` only adds. There
  is no removal path.
- **Duplication copies, never shares**: `Document.Duplicate` regenerates
  every block uid, rewrites image/file `src` paths to the new document (the
  file id is read off the src), and returns old→new maps for files (copied
  server-side) and block uids (hooks re-anchored; unmapped ones dropped).

## History entries

`document_branch_history_entries` are restorable snapshots: name, icon,
content, hook definitions (type, block, settings; never watcher state)
and the author. Every write goes through
`RecordDocumentBranchHistoryEntry`, which locks the branch row, then
reads it and its hooks, in the transaction that wrote the change.

- Edits in one 30-minute bucket (by `created_at`) update the newest
  entry and its `updated_at`. An entry equal to the newest is skipped. A
  null author (system write, no maintainers) never replaces a set one.
- Listed hooks are those whose block the content holds, soft-deleted or
  not.
- Hook writes go through `hook/manager`: create, update and delete record
  an entry in their transaction, reset does not. Callers flush first.
- Create, duplicate, fork and merge write a **boundary** entry that never
  aggregates, after `CopyHooks` inserted the copies, so it lists them.
- Entries pin the files they reference. Retention keeps each branch's
  newest entry. Entries cascade with their branch.

## Files and hooks

Files live at `organizations/{org}/documents/{doc}/files/{fileId}`. The id
is a nanoid the editor mints per upload, not the block uid, and an id that
already has a row is refused with 409, so a history entry keeps its object
when a block's file is replaced. Rows are `document_files` with name, size
and sniffed content type. `GET
/api/documents/{id}/files/{fileId}-{fileName}` cuts the 21-char nanoid off
the front and ignores the name; `Content-Disposition` is `inline` only for
`file.Viewable`. The key is an S3 object key or a path under the storage
directory (`internal/storage/{s3,fs}`). Reclamation is by sweep, never by a
request handler:

- **The row outlives its owner.** `document_files` and `document_hooks`
  reference documents, branches and organizations with `ON DELETE SET
  NULL`; `document_files.storage_key` stays reachable once FKs are gone.
- `file/manager` ticks every 5 minutes: trims expired history, then reclaims
  files. A file is referenced if its id appears in any branch content,
  retained history entry or comment (`CheckDocumentFileReferenced`, in
  SQL). Unreferenced files are stamped `unreferenced_at` and deleted a day
  later; NULL-FK rows skip the wait; files younger than a day are never
  touched.
- `hook/manager` does the same for hooks, calling `Hook.Delete` before
  dropping a row whose branch, document or org went NULL. A merge detaches
  the target's hooks (`DetachDocumentHooksByBranchID`) rather than
  soft-deleting them.
- **Organization deletion is announced**: auth-realtime's
  `beforeDeleteOrganization` calls `POST /api/x/organizations/{id}/teardown`
  while rows still exist and throws if core fails.
- Create writes the row before the object; delete removes the object before
  the row.
- **A null hook state means not set up.** Copies get one (never the
  source's) and `ProcessBranch` sets them up after commit. Hook writes
  and the sweep go through the manager's per-hook lock.

## Search

- **Jobs apply in order per document**; a failed diff holds back every later
  one for that document, and an org removal holds everything after it.
- **Entries are branch-scoped**: id `<branchId>-<blockUid>`
  (`<branchId>-docname` for the name), carrying `documentId`, `branchId`,
  `branchName`, `branchDefault`. Every branch change queues its diff through
  `search.Jobs` in the same transaction (persist, fork, rename, merge);
  branch deletion queues a `RemovedBranches` removal, document deletion
  clears by `documentId` using the subtree ids `agent.DeleteDocument`
  returns. Index settings are sent only when they differ from the index;
  every write task is awaited.

## Tags

Tags are per branch, visibility per user: `tags` (unique name per org,
`sort_index` for order), `document_branch_tags` (cascading from both),
`user_tag_settings` (tags a user hides). The sidebar tree lists a document
under a tag by its default branch; the header reads the open branch's tags
from `GET /documents/{documentId}/branches/{branchId}/tags`. A fork copies
tags, a merge makes the target carry exactly the source's, a duplicate
copies the default branch's.
