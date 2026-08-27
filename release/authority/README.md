# Authority transition evidence

`transition-evidence.json` is a standalone, content-addressed readback of the
two Git commits named by an active
`release/specification-schema-authority-tombstone.json`. It contains the
commit/tree objects needed to prove that `successorAuthorityCommit` is the
direct child of `successorPreparedCommit` and that the child changes only the
successor Core canonical authority record at
`release/specification-authority.json`. The predecessor tombstone at
`release/specification-schema-authority-tombstone.json` is a separate record;
it is never used as the Core P0/P blob. The readback also validates the exact
Core `prepared-writer-disabled` JSON semantics and the P0-to-P field delta.
The tombstone pins both the readback digest and a digest of its own fields
(excluding the pointer, to avoid a hash cycle).

Validation reads these bytes and Git history locally. It never fetches the
predecessor or successor repository. A shallow checkout, missing cutoff
ancestor, missing object, parent mismatch, extra changed path, or digest
mismatch fails closed before deploy credentials or source gates are reached.

The local check prevents an `active` record from becoming `pending` on a
descendant commit. Rewriting or deleting published branch history is outside
this repository's authority and must remain protected by the hosting
repository's branch policy; the local check cannot make rewritten history
recoverable.

Keep the tracked record `pending` with null transition fields until the real
`P0` and `P` commits exist. The deterministic helper is:

```text
bun scripts/authority-tombstone.mjs --activate \
  --prepared=<P0> --authority=<P> --disabled-at=<RFC3339>
```

When the successor objects are held in a separate local checkout, pass
`--objects-from=<checkout>` while generating the readback. Runtime validation
still reads only the committed readback bytes; it does not inspect that
checkout or use a network.
