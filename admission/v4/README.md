# Standard admission v4 candidate lane

`admission/v4` is the fail-closed successor lane for the exact mixed-version
`ga-core-v2` ten-Form subset selected by
`forms/admission-candidate-set.json`.

The generation replaces `HttpService@1.0.0` with the provider-neutral
`EdgeWorker@2.0.0`. It does not rewrite the immutable `ga-core-v1` package
publication snapshot under `admission/v3`.

This directory intentionally contains only reviewed trust and conforming-host
policy before publication. Package readback, signed provider/host/Registry
candidates, and signed admission evidence are added only by their protected
workflows after the exact Form Packages and provider `v1.0.0` are published.
Until then `current-published-package-check` and
`current-admission-closure-check` fail closed.
