# Contributing

Contributions are welcome. Open an issue before changing the public Service Form contract or adding a resource kind.

## Local checks

Use Go 1.23 or later and run:

```console
gofmt -w .
go vet ./...
go test ./...
```

A change is ready for review only when formatting, vet, and tests pass and any schema change includes documentation and an example.

## Provider boundary

Keep this provider thin and host-neutral. It may expose only the ten documented typed Service Forms unless the public specification and conformance suite are updated first. Do not add target-pool, backend, credential, secret, pricing, billing, quota, account, or operator-policy resources. Never log bearer tokens or returned secret material.

Do not add release automation until its signing-key custody, GitHub permissions, and Terraform Registry publication flow are documented and fail closed.
