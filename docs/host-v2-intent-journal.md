# Host v2 intent journal prerequisite

`internal/intentv2` is an unmounted prerequisite for a future Host API v2
resource lifecycle. It stores mutation intent; it does not send HTTP, decide
whether a request may be replayed, perform retries, or establish Host support.
The caller remains responsible for deciding whether to send the exact saved
request with the saved key after an interruption.

## Custody and identity

One journal belongs to one Provider state-management scope. Configure one
initialized local path and a fixed store ID for that scope. Do not share or
adopt a journal across unrelated states, and do not infer ownership from a
matching resource name. `Initialize` is an explicit operator action that
creates a new store and refuses an existing path. `Open` only verifies an
existing store and exact store ID; it never creates, repairs, truncates, or
replaces one. A missing, corrupt, partial, locked, or unavailable store is a
hard stop before a caller may send a mutation.

The record identity includes the exact Host API root, a caller-declared
principal-scope label, Space, and resource name. The label is stable binding
metadata, not proof that credentials still resolve to the same authenticated
principal. The caller must establish principal continuity independently.
Credentials and private-input values are not accepted by the journal API.
Only private-input names may be retained to match a request; values and hashes
are never stored. Public JSON must not contain a `privateInputs` member.

The store and files must be owned by the current OS user and private to that
user. The POSIX backend is build-tagged for Linux, macOS, FreeBSD, OpenBSD, and
NetBSD, but this implementation run exercised runtime behavior only on Linux.
Windows was compile-checked and its non-POSIX backend fails closed at runtime;
other OS builds and runtimes remain unverified. This does not change the v1
Provider's platform support. Network filesystems are unsupported. File and
directory sync plus atomic replacement reduce local-filesystem loss windows,
but power-loss guarantees depend on filesystem and device behavior and have not
been qualified.

## Mutation claims and recovery

`Begin` accepts only the Host mutation wire shapes: Create is `POST
/resources` with its original envelope; Update is `PUT /resources/{UID}` with a
spec body; Delete is `DELETE /resources/{UID}` with no body. Update/Delete
must use the immutable Form and exact UID/generation established by this
journal; this prerequisite does not import or adopt existing Host resources.
It durably records a random idempotency key, exact method/path/Form, public
JSON bytes, private-input name set, target UID/generation fence where
applicable, and a first pre-send timestamp plus the currently advertised
replay window. It returns only after the claim has been written and synced.
An identical request finds the original claim and returns the original key,
timestamp, and window. A changed request for the same action fence fails
closed. Public JSON matching is byte-exact, so semantically equivalent JSON
with different whitespace or member order may be conservatively refused.

The timestamp and advertised window are evidence captured before the first
send attempt; they do not prove the Host accepted the request within that
window. The package does not authorize a resend, reinterpret current limits,
or mint a replacement key for an unknown outcome. `RecordAccepted` binds a
Host Operation ID, UID, action, advanced generation, status/effect, and initial
nonzero `retainUntil`. `Complete` requires matching Host Operation evidence
and exact-UID Resource evidence: successful Create/Update requires the
accepted generation and `phase:idle`; accepted failed mutations remain
reserved and require the accepted generation and `phase:error`. A failed
reserved intent allows a new explicit Update/Delete only after that evidence
has been recorded; it never frees the name. Delete confirmation requires a
succeeded/complete Operation plus exact-UID Resource absence. Later Operation
evidence may extend `retainUntil`, never shorten it. A confirmed Delete
tombstone permits a new Create incarnation, while preserving old
UID/generation action records so an old Delete retry cannot be mistaken for an
action on the recreated resource.

An accepted operation record is not proof that Terraform successfully
committed resource state. The caller must separately observe and commit the
result, and later state must retain the journal's store identity/custody. The
journal does not automatically discover or adopt Host resources by name.
Pre-existing/imported state needs a later explicit binding flow using
validated exact-UID Host state; a synthetic Create record or name match is not
such proof. A recorded accepted/applied operation also does not prove that
Terraform state was durably committed.

`Initialize` syncs the newly created store directory's parent, each initial
file, and the store directory before returning. This is a local filesystem
durability measure, not a claim of power-loss safety on every filesystem or
storage device.

Focused subprocess tests kill a separate OS process after its claim is durable
and after an accepted Operation is recorded, then reopen the same store and
verify retained identity and continuation. A separate two-process fixture
contends on one name with different Forms and expects only one unresolved
winner. These tests cover this journal's local process boundary only; they do
not establish OpenTofu/Provider state durability, Host behavior, backend
recovery, or power-loss guarantees.

## Bounds and failure handling

The current implementation bounds a snapshot to 16 MiB, history to 4096
records, public JSON to 1 MiB, private-input names to 256, and waits at most
five seconds for the cross-process lock. Directory/file ownership, links,
lock inode, store identity, and directory identity are checked around access.
An unexpected entry (including an interrupted temporary snapshot), malformed
record, failed sync, or exceeded bound stops the operation. The package never
silently rebuilds or discards history; repair and backup custody remain an
operator procedure outside this prerequisite.
