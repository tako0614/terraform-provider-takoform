# Conformance language and classes

## Requirement keywords

The key words **MUST**, **MUST NOT**, **REQUIRED**, **SHALL**, **SHALL NOT**,
**SHOULD**, **SHOULD NOT**, **RECOMMENDED**, **MAY**, and **OPTIONAL** in the
Takoform specification are to be interpreted as described in
[BCP 14](https://www.rfc-editor.org/info/bcp14)
([RFC 2119](https://www.rfc-editor.org/rfc/rfc2119),
[RFC 8174](https://www.rfc-editor.org/rfc/rfc8174)) when, and only when, they
appear in all capitals.

Prose that does not use those keywords is explanatory. Where prose and a
normative schema disagree, **the schema wins**; every schema this
specification declares normative is listed in [`schemas/`](schemas/).

## Conformance classes

Takoform defines four independent roles. An implementation claims one or more
of them, and a claim is only ever about the class it names: a conforming host
does not become a conforming publisher by hosting Forms, and a conforming
provider does not admit anything.

### Form Package (data)

A **conforming Form Package** MUST satisfy
[`form-package/`](form-package/) and [`form-definition/`](form-definition/):
one definition, one exact `FormRef`, RFC 8785 canonical bytes, a closed file
inventory, allowlisted data media types, and no executable, credential,
placement, or commercial content. It MUST NOT depend on any host to be valid.

### Form host (protocol)

A **conforming Form host** MUST implement [`host-api/`](host-api/): versioned
discovery, exact Form availability, reviewed preview and apply, optimistic
concurrency, idempotent lifecycle, and the stable error taxonomy. It MUST
reject a request whose exact Form identity it has not installed, and it MUST
NOT return a Resource whose name, space, or Form identity differs from the one
requested.

A host **MAY** additionally implement the optional read-only interface
declaration surface ([`interface-declaration/`](interface-declaration/)). A host
that does not is still fully conforming.

A host decides placement, capacity, credentials, and commercial terms. Those
decisions are outside this specification, and a host MUST NOT require them to
appear in portable desired state.

### Form provider (client)

A **conforming provider** MUST send only declared desired state, MUST carry the
exact five-field installed Form identity on every mutation, and MUST NOT place
a credential, price, target, or backend selection in its state.

### Form publisher (distribution)

A **conforming publisher** MUST satisfy [`trust/`](trust/): immutable release
bytes, exact digests, provenance, and an append-only revocation chain. A
publisher MUST NOT overwrite or re-sign a released version in place.

## What conformance is not

Passing the checks in this repository proves the local data and schema
contract. It is not, and MUST NOT be presented as, evidence of publication,
host admission, activation, revocation enforcement, or interoperability with
any particular host. Those require signed evidence from the party that actually
performed the operation.
