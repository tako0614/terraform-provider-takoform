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
	dist := filepath.Join(h.workDir, "dist")
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
	bundleName := ""
	versionName := ""
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
  main_module = "index.js"

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
`, workerName))
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
  worker   = takoform_module_worker.app.name
  bundle   = takoform_worker_bundle.app.name
  handlers = ["fetch"]

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
`, bucketBinding, workerName+"-deployment"))
	return builder.String()
}

// moduleStack renders the same worker through the official module.
func moduleStack(endpoint, modulePath string, endpointAttachment bool) string {
	return providerBlock(endpoint) + fmt.Sprintf(`
module "worker_app" {
  source = %q

  name        = %q
  main_module = "index.js"
  content_dir = "${path.module}/dist"
  endpoint    = %t
}

output "worker_app_url" {
  value = module.worker_app.url
}
`, modulePath, workerName, endpointAttachment)
}
