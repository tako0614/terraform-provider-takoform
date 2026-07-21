# Capsule Composition catalog

These are portable selection manifests, not application repositories and not
Takosumi execution configuration. They describe Git Capsule coordinates and
requested non-secret Interface relationships only.

Verify one before publishing or linking it:

```console
go run ./cmd/composition-manifest verify compositions/yurucommu-standalone.json
```

A host link pins the canonical digest returned by that command:

```text
https://app.takosumi.com/install?composition=<https-URL-to-manifest>&digest=sha256:<canonical-digest>
```

Current catalog entries:

| Manifest | Canonical digest |
| --- | --- |
| `yurucommu-standalone.json` | `sha256:ee5a6856a560167d7a548ffc3c7802f19473135ff0f6b03010f20df6ae1308db` |
| `yurumeet-standalone.json` | `sha256:db0ce0418747c9da242b38f4c1f63ce5d1446c28d6814f507da94cd0a63e06e2` |
| `takos-office-with-storage.json` | `sha256:03609cef9cdabff08d5d1f546615cb68440f018f5fe2acca073a121901f3cbea` |

The dashboard displays the chosen composition and sends each selected component
through its ordinary Git Source/Capsule review. A composition never grants a
ProviderConnection, InterfaceBinding, target, or automatic apply.
