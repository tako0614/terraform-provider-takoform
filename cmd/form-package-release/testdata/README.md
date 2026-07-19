# Cosign v3 bundle fixture

`cosign-v3.0.6-message-signature.sigstore.json` is an actual public bundle
generated and immediately verified by pinned Cosign v3.0.6 in
[GitHub Actions run 29668147384](https://github.com/tako0614/terraform-provider-takoform/actions/runs/29668147384).
The signed subject was the UTF-8 line `Takoform Cosign v3 bundle shape probe`.

The fixture contains only public verification material: the ephemeral Fulcio
certificate, blob signature, Rekor transparency-log entry and inclusion proof,
and timestamp verification data. It contains no private key, OIDC token, or
repository secret.

Cosign v3.0.6 depends on `sigstore/protobuf-specs` v0.5.0. The pinned
[`Bundle` protobuf](https://github.com/sigstore/protobuf-specs/blob/f8d009ede80474a9e257788207a00693e4693168/protos/sigstore_bundle.proto)
and the pinned
[Cosign bundle specification](https://github.com/sigstore/cosign/blob/f1ad3ee952313be5d74a49d67ba0aa8d0d5e351f/specs/BUNDLE_SPEC.md)
both encode the bundle content oneof as a top-level `messageSignature` or
`dsseEnvelope`. They do not define a nested `content.messageSignature` object.
