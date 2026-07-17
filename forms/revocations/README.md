# Append-only Form Package revocations

Each `<statementVersion>.json` is one immutable, consecutively sequenced
security revocation for an exact `(FormRef, packageDigest)`. It must satisfy
`formpackage/schemas/form-package-revocation.schema.json` and is delivered by
the matching `forms/revocations/v<statementVersion>` tag. Each matching
`checkpoints/<statementVersion>.json` is a cumulative index of every statement
from sequence 1 through the current sequence and includes the previous
checkpoint's canonical SHA-256 digest.

Only new statement and checkpoint files may be added. Never edit, rename,
delete, supersede in place, or reuse a version or sequence. A corrected
decision is a new statement plus cumulative checkpoint. The workflow signs the
checkpoint, and hosts retain its `(sequence, checkpoint digest, cumulative
entries digest)` pin before accepting the next checkpoint. Revocation blocks new create/update and activation but retains the
referenced bytes for observe/delete and operator evacuation. Ordinary
deprecation belongs in the Form Definition status instead.

No revocation statement has been published yet.
