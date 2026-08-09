# The official `worker-app` module: one running ES Module Worker, assembled
# from the five raw Forms the Edge Platform Family splits it across.
#
# The raw Forms stay the low-level surface, and the split is deliberate — it is
# what makes a rollback a re-weighting instead of a mutation. What this module
# owns is the part of that split an ordinary author should never have to
# rediscover: a Worker Bundle and a Worker Version are IMMUTABLE revisions, so
# every code change creates new ones beside the old, the deployment is
# re-weighted onto the new version, and only then are the old revisions
# removable.
#
# Two mechanisms carry that, and both are load-bearing:
#
#   - the revisions carry NO `name`. The provider derives
#     `bundle-<manifest digest prefix>-<owner digest prefix>` and
#     `version-<spec digest prefix>-<owner digest prefix>` from the revision's
#     own content and its declared owner, so changed bytes are a different
#     resource rather than a replacement of the same one. The owner is this
#     module's `name`, which is the one name an author chooses and is already
#     unique in the space: without it two instances of this module built from
#     identical output would derive one name, and two Terraform resources would
#     manage one host address.
#   - the revisions declare `create_before_destroy`. Terraform's default order
#     destroys first, and the destroy is refused with `dependency_in_use` (409)
#     while the Worker Version still executes the bundle and the Worker
#     Deployment still weights the version. Creating first, then re-weighting,
#     then destroying is the only order the host's own rules admit.
#
# The Module Worker identity and the Worker Deployment deliberately do NOT
# carry `create_before_destroy`: they are the stable things. The worker's name
# is the author's, the deployment is updated in place, and neither is ever
# replaced by a code change.

locals {
  # Every file under content_dir is one module of the bundle, named by its path
  # relative to that directory — which is exactly how the module graph resolves
  # an import at runtime.
  module_files = fileset(var.content_dir, "**")

  modules = [
    for relative_path in sort(local.module_files) : {
      name         = relative_path
      content_type = lookup(var.module_media_types, lower(reverse(split(".", relative_path))[0]), "application/octet-stream")
      content_file = "${var.content_dir}/${relative_path}"
    }
  ]
}

resource "takoform_module_worker" "this" {
  name  = var.name
  space = var.space

  create_timeout = var.create_timeout
  delete_timeout = var.delete_timeout
}

resource "takoform_worker_bundle" "this" {
  # No name: the provider derives bundle-<manifest digest prefix>-<owner digest
  # prefix>, and this module owns it.
  revision_owner = var.name
  space          = var.space
  main_module    = var.main_module
  modules        = local.modules

  create_timeout = var.create_timeout
  delete_timeout = var.delete_timeout

  lifecycle {
    create_before_destroy = true
  }
}

resource "takoform_worker_version" "this" {
  # No name: the provider derives version-<spec digest prefix>-<owner digest
  # prefix>, and this module owns it.
  revision_owner = var.name
  space          = var.space
  worker         = takoform_module_worker.this.name
  bundle         = takoform_worker_bundle.this.name
  handlers       = var.handlers

  vars_json               = length(var.vars) > 0 ? jsonencode(var.vars) : null
  required_sensitive_vars = var.required_sensitive_vars

  kv_bindings = [
    for binding_name in sort(keys(var.kv_bindings)) : {
      name        = binding_name
      target_name = var.kv_bindings[binding_name]
    }
  ]

  bucket_bindings = [
    for binding_name in sort(keys(var.bucket_bindings)) : {
      name        = binding_name
      target_name = var.bucket_bindings[binding_name]
    }
  ]

  sqlite_bindings = [
    for binding_name in sort(keys(var.sqlite_bindings)) : {
      name        = binding_name
      target_name = var.sqlite_bindings[binding_name]
    }
  ]

  queue_producer_bindings = [
    for binding_name in sort(keys(var.queue_producer_bindings)) : {
      name        = binding_name
      target_name = var.queue_producer_bindings[binding_name]
    }
  ]

  service_bindings = [
    for binding_name in sort(keys(var.service_bindings)) : {
      name        = binding_name
      target_name = var.service_bindings[binding_name]
    }
  ]

  create_timeout = var.create_timeout
  delete_timeout = var.delete_timeout

  lifecycle {
    create_before_destroy = true
  }
}

resource "takoform_worker_deployment" "this" {
  name   = "${var.name}-deployment"
  space  = var.space
  worker = takoform_module_worker.this.name

  versions = [
    {
      worker_version = takoform_worker_version.this.name
      weight         = 10000
    },
  ]

  create_timeout = var.create_timeout
  update_timeout = var.create_timeout
  delete_timeout = var.delete_timeout
}

resource "takoform_worker_endpoint" "this" {
  count = var.endpoint ? 1 : 0

  name   = "${var.name}-endpoint"
  space  = var.space
  worker = takoform_module_worker.this.name

  create_timeout = var.create_timeout
  delete_timeout = var.delete_timeout

  # A worker is reachable only once its deployment serves fetch, and the host
  # refuses the attachment before that (unsupported_capability, 422). The
  # dependency states the order the host already enforces.
  depends_on = [takoform_worker_deployment.this]
}
