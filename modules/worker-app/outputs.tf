output "worker_name" {
  value       = takoform_module_worker.this.name
  description = "Portable name of the Module Worker identity. It is stable across every code change."
}

output "worker_ready" {
  value       = takoform_module_worker.this.ready
  description = "True when the worker has an active deployment whose every weighted version exports fetch."
}

output "bundle_name" {
  value       = takoform_worker_bundle.this.name
  description = "Derived name of the current Worker Bundle revision: bundle-<manifest digest prefix>."
}

output "manifest_digest" {
  value       = takoform_worker_bundle.this.manifest_digest
  description = "Immutable content address of the artifact manifest this deploy committed."
}

output "version_name" {
  value       = takoform_worker_version.this.name
  description = "Derived name of the current Worker Version revision: version-<spec digest prefix>."
}

output "deployment_name" {
  value       = takoform_worker_deployment.this.name
  description = "Name of the one Worker Deployment that governs this worker's traffic."
}

output "hostname" {
  value       = var.endpoint ? takoform_worker_endpoint.this[0].hostname : null
  description = "DNS name the host assigned, when an endpoint was requested. Never parse it, assert a suffix on it, or reconstruct it: its shape is host detail."
}

output "url" {
  value       = var.endpoint ? takoform_worker_endpoint.this[0].url : null
  description = "Complete HTTPS address of the worker's active deployment, when an endpoint was requested."
}
