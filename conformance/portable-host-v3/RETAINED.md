# Retained: the `forms.takoform.com/v1alpha3` host corpus

These bytes are the portable host corpus for the **retained**
`forms.takoform.com/v1alpha3` lane: 114 required checks, discovered at
`/.well-known/takoform/v1alpha3`. They are retained history, published at
`https://takoform.com/conformance/portable-host-v3/`, and they do not move.

## Why this directory exists under this name

The corpus directories were numbered by generation — `portable-host-v1` for
`v1alpha1`, `-v2` for `v1alpha2`, `-v3` for `v1alpha3` — and for three
generations the generation counter and the lane identity agreed. When the
current lane became `forms.takoform.com/v1beta1` the agreement ended, and this
directory was rewritten in place to carry the new lane. That made one published
URL answer about a different contract than it had answered about the day
before, which
[decision 0035](../../spec/decisions/0035-beta-contracts-ship-in-stable-provider-v2-1.md)
forbids: every `v1alpha3` schema, specification, operation table, public URL,
and byte remains retained history.

The v1alpha3 bytes are restored here, at the address that always named them.
The current lane's corpus is [`../portable-host-v1beta1/`](../portable-host-v1beta1/),
named for the lane it measures rather than for its place in a sequence.

## This corpus is retained, not runnable

No runner in this repository loads it. The v1alpha3 runner became the v1beta1
runner rather than being copied, so the code that once executed these checks no
longer exists, and reviving it would prove nothing about a lane no host is being
measured against. `internal/portableconformancev3` now verifies
`takoform.portable-host-conformance-manifest@v1beta1`, so pointing it at this
manifest fails closed on identity rather than half-executing.

What is guaranteed here is not that these checks run. It is that these bytes do
not change: `bun run check` compares them against the commit that published
them, so a future edit to a retained corpus fails the gate by name.
