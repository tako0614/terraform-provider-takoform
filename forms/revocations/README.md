# Append-only Form Package revocations

Each `<statementVersion>.json` is one immutable security revocation for an
exact `(FormRef, packageDigest)`. It must satisfy
`formpackage/schemas/form-package-revocation.schema.json` and is delivered by
the matching `forms/revocations/v<statementVersion>` tag.

Only new statement files may be added. Never edit, rename, delete, supersede in
place, or reuse a statement version. A corrected decision is a new signed
statement. Revocation blocks new create/update and activation but retains the
referenced bytes for observe/delete and operator evacuation. Ordinary
deprecation belongs in the Form Definition status instead.

No revocation statement has been published yet.
