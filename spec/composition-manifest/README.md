# Capsule Composition Manifest v1alpha1

A Capsule Composition Manifest is portable, data-only selection metadata for
one or more ordinary Git-hosted OpenTofu/Terraform Capsules. It gives users a
repeatable starting composition such as a standalone app or an app plus a
separate storage Capsule.

It is deliberately **not** a Form Package. A Form Package contains exactly one
portable Service Form definition; a Composition Manifest contains Git source
coordinates and optional requested Interface relationships.

```json
{
  "apiVersion": "compositions.takoform.com/v1alpha1",
  "kind": "CapsuleComposition",
  "metadata": {
    "name": "office-with-storage",
    "version": "1.0.0",
    "title": "Takos Office with storage"
  },
  "components": [
    {
      "id": "office",
      "title": "Takos Office",
      "source": {
        "url": "https://github.com/tako0614/takos-office.git",
        "ref": "<immutable git commit>",
        "path": "."
      }
    }
  ]
}
```

The manifest may contain only a closed document with `metadata`, `components`,
and optional `connections`. It cannot carry commands, artifacts, provider
configuration, credentials, secrets, targets, capacity, pricing, billing,
quota, account data, authorization, host InstallConfigs, or lifecycle policy.
Every `source.ref` is a full lowercase 40- or 64-hex Git commit object ID;
branches, tags, abbreviated IDs, and other movable refs are rejected so the
manifest digest selects the exact reviewed Capsule source.

A host treats every component as a normal Source/Capsule flow: it applies its
own source policy, asks for ProviderConnection and CredentialRecipe choices,
materializes Interface records only through its service-side configuration,
requires InterfaceBinding authorization, and records reviewed Runs. A declared
connection is a request to present and validate; it never grants access or
creates a binding by itself.

`go test ./composition` verifies the closed parser and RFC 8785 digest. The
digest identifies the exact manifest bytes after canonicalization and is the
value a URL-selected host should pin before showing its install review.
