# Form Package release sources

Each child directory is one exact data-only release source:

```text
<release-id>/<packageVersion>/package-index.json
<release-id>/<packageVersion>/<listed payloads>
```

The directory must pass `go run ./cmd/form-package verify`. Its release ID is
`k-` followed by the lowercase, unpadded base32 encoding of the exact ASCII
FormRef Kind bytes. This encoding is reversible and preserves distinctions such
as `SQLDatabase` versus `SqlDatabase`. Its version must equal `packageVersion`
and the `forms/<release-id>/v<packageVersion>` tag. Compatibility and test fixtures stay
under `conformance/`; copying one here is an explicit reviewed release
decision, not automatic standardization.

The retired `1.0.0` directories are the release-owned source copies of the
historical first structural-candidate set. They are no longer regenerated from
their generation of the `conformance/form-package-v1/positive/standard/` tree.
Their immutable tags and retained release manifests remain historical evidence;
fixture regeneration never updates a release directory automatically.

The retired `1.0.1` directories are also immutable published release sources.
They coexist with `1.0.0` and never replace its bytes or tags.
`forms/retired-package-set.json` and
`admission/v1/published-package-set.json` select the exact `1.0.1` generation,
and `standard-form-conformance published-package-check` authenticates that
selected set against its retained release assets. The retained `1.0.0` history
is not the selected input to that current check.

Publication of either generation does not change
`forms/standard-package-set.json` from `structural-candidate` and does not
satisfy direct Registry readback, host/provider lifecycle, signed admission, or
revocation-chain requirements.

`k-kniuyrdborqweyltmu/2.0.0` is the reviewed source for the independent
`SQLDatabase@2.0.0` bounded indexed successor. It coexists with both SQLDatabase
1.x identities and is generated from the exact local successor package. Its
presence is not a release, Registry publication, admission, or activation
claim.
