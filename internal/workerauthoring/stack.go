package workerauthoring

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The disposable stack is authored against the reference host's primary
// credential and one ordinary space. Both are harness facts, never portable
// ones.
const (
	harnessToken = "reference-primary-token"
	harnessSpace = "prod"
	workerName   = "counter"
)

// moduleSource renders one deterministic ES module. `revision` changes the
// bytes and therefore the manifest digest, which is what a real code change
// does.
func moduleSource(revision int) string {
	return fmt.Sprintf(`export default {
  async fetch() {
    return new Response("counter revision %d");
  },
};
`, revision)
}

// writeModuleSource lays the worker's build output down beside the stack, the
// way `content_dir` output from a real bundler would be.
func (h *harness) writeModuleSource(revision int) error {
	return h.writeModuleSourceIn("dist", revision)
}

// writeModuleSourceIn lays one owner's build output into its own directory, so
// two owners can be given byte-identical output and then moved independently.
func (h *harness) writeModuleSourceIn(directory string, revision int) error {
	dist := filepath.Join(h.workDir, directory)
	if err := os.MkdirAll(dist, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dist, "index.js"), []byte(moduleSource(revision)), 0o600)
}

func providerBlock(endpoint string) string {
	return fmt.Sprintf(`terraform {
  required_providers {
    takoform = {
      source = %q
    }
  }
}

provider "takoform" {
  endpoint = %q
  token    = %q
  space    = %q
}
`, ProviderAddress, endpoint, harnessToken, harnessSpace)
}

// rawStackOptions selects which authoring mistake the raw stack reproduces.
type rawStackOptions struct {
	// PinnedNames writes an explicit `name` on both immutable revisions, which
	// is what an author does before the provider derives one.
	PinnedNames bool
	// CreateBeforeDestroy adds the lifecycle meta-argument to both revisions.
	CreateBeforeDestroy bool
}

// rawStack renders the five-resource worker aggregate written directly against
// the raw Forms.
func rawStack(endpoint string, options rawStackOptions) string {
	var builder strings.Builder
	builder.WriteString(providerBlock(endpoint))
	builder.WriteString(fmt.Sprintf(`
resource "takoform_module_worker" "app" {
  name = %q
}
`, workerName))

	lifecycle := ""
	if options.CreateBeforeDestroy {
		lifecycle = "\n  lifecycle {\n    create_before_destroy = true\n  }\n"
	}
	// A revision either pins its name or declares whose revision it is: a name
	// derived from content alone would be one address two owners can reach.
	bundleName := fmt.Sprintf("  revision_owner = %q\n", workerName)
	versionName := fmt.Sprintf("  revision_owner = %q\n", workerName)
	if options.PinnedNames {
		bundleName = fmt.Sprintf("  name        = %q\n", workerName+"-bundle")
		versionName = fmt.Sprintf("  name     = %q\n", workerName+"-version")
	}
	builder.WriteString(fmt.Sprintf(`
resource "takoform_worker_bundle" "app" {
%s  main_module = "index.js"

  modules = [
    {
      name         = "index.js"
      content_type = "application/javascript+module"
      content_file = "${path.module}/dist/index.js"
    },
  ]
%s}
`, bundleName, lifecycle))

	builder.WriteString(fmt.Sprintf(`
resource "takoform_worker_version" "app" {
%s  worker   = takoform_module_worker.app.name
  bundle   = takoform_worker_bundle.app.name
  handlers = ["fetch"]
%s}
`, versionName, lifecycle))

	builder.WriteString(fmt.Sprintf(`
resource "takoform_worker_deployment" "app" {
  name   = %q
  worker = takoform_module_worker.app.name

  versions = [
    {
      worker_version = takoform_worker_version.app.name
      weight         = 10000
    },
  ]
}
`, workerName+"-deployment"))
	return builder.String()
}

// bindingStack renders a worker that uses the `edge.kv` binding, and
// optionally an object bucket as well. It is the configuration the host-support
// scenario plans: the first shape is what a KV-only host implements, the second
// asks that host for something it does not have.
func bindingStack(endpoint string, withBucket bool) string {
	var builder strings.Builder
	builder.WriteString(providerBlock(endpoint))
	builder.WriteString(fmt.Sprintf(`
resource "takoform_module_worker" "app" {
  name = %q
}

resource "takoform_edge_kv_namespace" "state" {
  name = "counter-state"
}

resource "takoform_worker_bundle" "app" {
  revision_owner = %q
  main_module    = "index.js"

  modules = [
    {
      name         = "index.js"
      content_type = "application/javascript+module"
      content_file = "${path.module}/dist/index.js"
    },
  ]

  lifecycle {
    create_before_destroy = true
  }
}
`, workerName, workerName))
	bucketResource := ""
	bucketBinding := ""
	if withBucket {
		bucketResource = `
resource "takoform_edge_object_bucket" "media" {
  name = "counter-media"
}
`
		bucketBinding = `
  bucket_bindings = [
    {
      name        = "MEDIA"
      target_name = takoform_edge_object_bucket.media.name
    },
  ]
`
	}
	builder.WriteString(bucketResource)
	builder.WriteString(fmt.Sprintf(`
resource "takoform_worker_version" "app" {
  revision_owner = %q
  worker         = takoform_module_worker.app.name
  bundle         = takoform_worker_bundle.app.name
  handlers       = ["fetch"]

  kv_bindings = [
    {
      name        = "COUNTER"
      target_name = takoform_edge_kv_namespace.state.name
    },
  ]
%s
  lifecycle {
    create_before_destroy = true
  }
}

resource "takoform_worker_deployment" "app" {
  name   = %q
  worker = takoform_module_worker.app.name

  versions = [
    {
      worker_version = takoform_worker_version.app.name
      weight         = 10000
    },
  ]
}
`, workerName, bucketBinding, workerName+"-deployment"))
	return builder.String()
}

// moduleStackOptions selects which shape of the official module one scenario
// authors.
type moduleStackOptions struct {
	// Owners are the Module Worker names, one module instance each. Empty means
	// the single default worker.
	Owners []string
	// Endpoint attaches a Worker Endpoint to every instance.
	Endpoint bool
	// Vars is the literal HCL body of the module's `vars` argument, or empty for
	// none. It is written verbatim so a scenario can author values whose JSON
	// types differ from each other.
	Vars string
}

func (options moduleStackOptions) owners() []string {
	if len(options.Owners) == 0 {
		return []string{workerName}
	}
	return options.Owners
}

// moduleInstanceLabel is the Terraform label of one module instance. It is
// derived from the owner name so an owner's outputs are addressable.
func moduleInstanceLabel(owner string) string {
	return "worker_app_" + strings.ReplaceAll(owner, "-", "_")
}

// contentDir is the build-output directory one owner's module reads. A single
// owner reads the shared `dist`; several owners each get their own, so a
// scenario can hand them byte-identical output and then move one alone.
func (options moduleStackOptions) contentDir(owner string) string {
	if len(options.Owners) <= 1 {
		return "dist"
	}
	return "dist-" + owner
}

// moduleStack renders the worker through the official module, once per owner.
//
// Every instance points at the SAME content directory, so two owners in one
// stack are two independent Terraform owners of byte-identical build output —
// which is exactly the state a derived name has to keep distinct.
func moduleStack(endpoint, modulePath string, options moduleStackOptions) string {
	var builder strings.Builder
	builder.WriteString(providerBlock(endpoint))
	for _, owner := range options.owners() {
		label := moduleInstanceLabel(owner)
		fmt.Fprintf(&builder, `
module %q {
  source = %q

  name        = %q
  main_module = "index.js"
  content_dir = "${path.module}/%s"
  endpoint    = %t
%s}
`, label, modulePath, owner, options.contentDir(owner), options.Endpoint, options.Vars)
		for _, name := range []string{"url", "bundle_name", "version_name", "worker_name"} {
			fmt.Fprintf(&builder, `
output "%s_%s" {
  value = module.%s.%s
}
`, label, name, label, name)
		}
	}
	return builder.String()
}

// defaultModuleStack renders the one-owner module stack the roll-forward
// scenario drives.
func defaultModuleStack(endpoint, modulePath string, endpointAttachment bool) string {
	return moduleStack(endpoint, modulePath, moduleStackOptions{Endpoint: endpointAttachment})
}

// heterogeneousVars is the `vars` argument whose three values have three
// different JSON types. A collection type that infers ONE element type cannot
// carry it: the values unify to strings before they are ever encoded.
const heterogeneousVars = `
  vars = {
    enabled = true
    retries = 3
    label   = "prod"
  }
`
