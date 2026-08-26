// Package providerdiagnostics owns the Provider diagnostic codes consumed by
// automation outside the provider package. Provider-internal codes remain
// private; a code enters this package only when another production component
// branches on its exact value.
package providerdiagnostics

const (
	ImmutableRevisionSameName = "takoform.provider/immutable-revision-same-name"
	HostDoesNotSupportValue   = "takoform.provider/host-does-not-support-value"
)
