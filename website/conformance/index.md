# Conformance evidence

The current `forms.takoform.com/v1` suite is the primary conformance surface.
Its manifest composes the generic Host contract, all eight current family
corpora, and cross-family composition checks. It exercises exact FormRef
lifecycle behavior, schema and default handling, runtime support, relations,
state/import semantics, artifacts, tenant boundaries, and asynchronous
operations. It is executable implementation evidence; it does not publish a
Form, grant Host support, or establish Form maturity.

## Run the current suite

From the repository root, run the manifest first:

```console
go run ./cmd/portable-host-conformance suite --manifest conformance/takoform-v1/manifest.json
```

The command starts the deterministic reference host and emits a
`takoform.reference-host-suite-report@v1` report. The committed manifest is
the authority for the suite inputs and required checks.

## Additional checks

The following checks validate the provider mapping, generated schema, examples,
and the historical compatibility corpus after the current suite has run:

```console
go test ./...
go vet ./...
tofu fmt -check -recursive examples
go run ./cmd/standard-form-conformance verify
go run ./cmd/worker-authoring-conformance matrix --opentofu tofu --terraform terraform
```

The current suite and retained corpus are committed under
[`conformance/`](https://github.com/tako0614/terraform-provider-takoform/tree/main/conformance).
For current resource mappings, see the [Provider reference](../docs/) and
[Provider mapping inventory](../forms/).

## Retained Provider 2.1.1 corpus

The historical [`portable-host-v1beta1` corpus](portable-host-v1beta1/contract.json)
is retained for Provider 2.1.1 compatibility. Its identities and upgrade path
are documented in the [release guide](../release/index.md) and the
[v2-to-v3 migration guide](../release/migrations/v2-to-v3.md).
