# Takoform portable specification

This directory is the portable specification surface for Takoform, an
**Experimental specification and tooling project**. Takoform defines a small
desired-state boundary between infrastructure-as-code clients and resource
hosts; it is not currently an industry standard, certification authority, or
guarantee of backend portability.

Requirement keywords, conformance classes, and what a passing check does and
does not prove are defined in [`conformance.md`](conformance.md). How the API
group, Forms, packages, and the provider are versioned is in
[`versioning.md`](versioning.md). Current project positioning and the
Proposal → Experimental → Stable → Legacy lifecycle are defined in
[`project-lifecycle.md`](project-lifecycle.md). The exact boundary between
portable workload semantics and host/profile/operator concerns is
[`portability-boundary.md`](portability-boundary.md).

## Product contract map

Takoform has five public contract interfaces:

1. **Exact Form and Package data.**
   [`form-definition/`](form-definition/) defines immutable `FormRef` and the
   desired/observed/output shape. [`form-package/`](form-package/) binds one
   exact definition and its data-only fixtures into immutable package bytes.
2. **Desired Resource lifecycle.**
   [`host-api/`](host-api/) defines discovery, exact Form availability,
   preview/apply, read/import/observe/refresh/delete, fencing, and portable
   errors. The host chooses implementation and placement.
3. **Read-only Form-derived Interface projection.**
   [`interface-declaration/`](interface-declaration/) defines open
   `(name, version)` descriptors embedded in Forms and their read-only host
   projection. Focused contracts such as the Legacy
   [`data.indexed@1`](data-indexed/) only define the descriptor data the
   declaring Form actually declares.
4. **Current Beta (Edge Platform Family) contract surfaces.**
   [`host-api/v1beta1.md`](host-api/v1beta1.md) defines the current Host API
   channel with UID/generation/revision resource identity, long-running
   Operations, and Host Support Profiles.
   [`form-families.md`](form-families.md) defines namespaced Form Family
   groups. [`interface-contract/`](interface-contract/) defines exact
   digest-bound Interface contracts,
   [`binding-contract/`](binding-contract/) defines typed Binding contracts,
   and [`artifact-transport/`](artifact-transport/) defines content-addressed
   artifact upload.
5. **Trust, lifecycle, version, and release identity.**
   [`trust/`](trust/) defines immutable publisher evidence and revocation;
   [`project-lifecycle.md`](project-lifecycle.md) separates Form maturity from
   Host Support and availability; [`versioning.md`](versioning.md) keeps
   provider, API, Form, and package compatibility independent.
   [`release/`](../release/index.md) binds artifacts to those exact identities
   without changing the contracts above.

[`schemas/`](schemas/), [`conformance.md`](conformance.md), and
[`decisions/`](decisions/) support those interfaces with structural minima,
executable evidence language, and decision rationale. They are not additional
product interfaces. The generated current inventory is
[`../forms/README.md`](../forms/index.md), host discovery validation is
[`../schemas/host-discovery.schema.json`](/schemas/host-discovery.schema.json),
and the local evidence map is
[`../conformance/README.md`](../conformance/index.md).

## Current status

The FormRef, Form Definition, package-index, revocation, and cumulative
revocation-checkpoint schemas, the RFC 8785/I-JSON library, the closed local
verifier, the positive/negative corpus, the protected keyless Sigstore release
lane, and the signed append-only checkpoint delivery lane are implemented.

Current Form design work uses namespaced Form Family groups
([`form-families.md`](form-families.md)); the first family is
`edge.forms.takoform.com/v1beta1`. Its exact 15 `0.1.0` definitions are
Experimental and its packages use `packages.forms.takoform.com/v1alpha4`.
Package publication and Form maturity are independent. The
`forms.takoform.com/v1alpha2`
epoch and its nine `0.1.0` source candidates are retained provider-v2 preview
source under the `packages.forms.takoform.com/v1alpha3` envelope
([decision 0035](decisions/0035-beta-contracts-ship-in-stable-provider-v2-1.md));
they are not the basis for new specification work. The published v1alpha3
schema, operation, documentation, and public-mirror identities are retained
unchanged history rather than rewritten as Beta. A repository
implementation or local passing gate is not Form publication, Host Support,
activation, or live Cloud availability.

Decision [`0004`](decisions/0004-takoform-is-an-experimental-specification.md)
made the previously published `forms.takoform.com/v1alpha1` line Legacy after
it was labelled `standard` without sufficient independent implementation and
operational evidence. Decision
[`0006`](decisions/0006-v1alpha2-restarts-form-lines.md) restarts selected kinds
in the distinct v1alpha2 epoch through mutable Proposals and Experimental
`0.x` Forms. Decision
[`0007`](decisions/0007-current-forms-exclude-substrate-operation.md) requires
those candidates to be independently authored and excludes substrate
operation from portable desired state. Historical `standard` and
`portable-standard` fields remain
readable in immutable documents; they do not define a current approved subset.
The lifecycle projection and Legacy verification tooling are implemented and
fail closed against
[`../forms/lifecycle.json`](../forms/lifecycle.json). Generated compatibility
inventory pages classify every retained entry as Legacy and MUST NOT
reinterpret historical package or admission fields as current project status.

Published generations are retained, not erased. Their immutable releases and
admission evidence stay verifiable offline through
[`../forms/retired-package-set.json`](../forms/retired-package-set.json), and
the current retained release sets. This build refuses to overwrite or restamp
their proofs with a new provider or maturity identity.

The frozen Legacy Host API wire remains `forms.takoform.com/v1alpha1`, and
the retained provider-v2 wire remains `forms.takoform.com/v1alpha2` at
`/.well-known/takoform/v1alpha2`. The v1alpha3 public identities are retained
unchanged. The current Host API wire is `forms.takoform.com/v1beta1`, reached
through `/.well-known/takoform/v1beta1` with API root
`/apis/forms.takoform.com/v1beta1`, so retained clients cannot select it
accidentally. The Host API group is a protocol compatibility identity
independent of any nested Form group. The current package envelope is
`packages.forms.takoform.com/v1alpha4`; Interface and Binding refs remain
`interfaces.takoform.com/v1alpha1` and `bindings.takoform.com/v1alpha1`. The
Terraform provider identity is `registry.terraform.io/tako0614/takoform`; its
stable `v2.1.0` release target is independent from all of these API identities
and remains `candidate-only` until the release owner publishes it.

## Normative consistency audit

`go test ./spec` is the cross-specification contradiction gate. It does not
repeat the Form Package verifier, provider schema tests, or portable-host
runner. Instead, it joins their machine-readable inputs and fails when:

- host operations, mutation fences, idempotency, or the stable error taxonomy
  disagree with the portable-host conformance contract;
- the optional Interface projection stops being read-only, same-origin, and
  materialized only from Form-declared descriptors;
- the portable API identity, provider candidate version, or canonical provider
  FQN diverges between release, schema, trust, and conformance locks; or
- a normative active Form, package, schema, or host contract leaks a concrete
  backend vocabulary such as Cloudflare/Workers configuration.

The complete repository gate, `bun run check`, runs this audit together with
the deeper package-byte, provider-schema, generated-surface, and lifecycle
verifiers. Passing it remains local evidence only; it does not prove Registry
publication, Host Support, Form maturity, production activation, or
interoperability.
