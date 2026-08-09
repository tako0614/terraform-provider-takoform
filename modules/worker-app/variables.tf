variable "name" {
  type        = string
  description = "Portable name of the Module Worker identity. Every revision this module creates is named from its own content, so this is the ONE name an author chooses and the one that stays stable across deploys."

  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{0,61}[a-z0-9]$", var.name))
    error_message = "name must be a portable resource name: lowercase letters, digits, and hyphens."
  }
}

variable "space" {
  type        = string
  default     = null
  description = "Exact opaque SpaceID for every resource this module creates. Null uses the provider default."
}

variable "content_dir" {
  type        = string
  description = "Directory holding the worker's build output. Every file under it becomes one module of the bundle, at its path relative to this directory."
}

variable "main_module" {
  type        = string
  default     = "index.js"
  description = "Relative path, inside content_dir, of the ES module the runtime instantiates first."
}

variable "module_media_types" {
  type = map(string)
  default = {
    "js"   = "application/javascript+module"
    "mjs"  = "application/javascript+module"
    "wasm" = "application/wasm"
    "txt"  = "text/plain"
    "map"  = "application/source-map+json"
    "bin"  = "application/octet-stream"
  }
  description = "Media type per file extension. The values are the closed Worker Bundle module media-type set; a file whose extension is absent from this map is carried as application/octet-stream."
}

variable "handlers" {
  type        = set(string)
  default     = ["fetch"]
  description = "Handlers the main module exports and this worker activates. The vocabulary is the worker.runtime contract's, and a host refuses a handler the module does not export."
}

variable "vars" {
  type        = map(any)
  default     = {}
  description = "Non-secret configuration projected into the worker environment. Never put a secret value here: it is portable desired state and it is persisted in Terraform state."
}

variable "required_sensitive_vars" {
  type        = set(string)
  default     = []
  description = "Names of secret slots the worker requires. Only the NAMES travel; the values are host-owned and never enter desired state or Terraform state."
}

variable "kv_bindings" {
  type        = map(string)
  default     = {}
  description = "Binding name to Edge KV namespace resource name."
}

variable "bucket_bindings" {
  type        = map(string)
  default     = {}
  description = "Binding name to Object Bucket resource name."
}

variable "sqlite_bindings" {
  type        = map(string)
  default     = {}
  description = "Binding name to SQLite Database resource name."
}

variable "queue_producer_bindings" {
  type        = map(string)
  default     = {}
  description = "Binding name to At-Least-Once Queue resource name."
}

variable "service_bindings" {
  type        = map(string)
  default     = {}
  description = "Binding name to another Module Worker's resource name."
}

variable "endpoint" {
  type        = bool
  default     = false
  description = "Attach a Worker Endpoint, so the host assigns a reachable HTTPS address for this worker's active deployment."
}

variable "create_timeout" {
  type        = string
  default     = null
  description = "Overall create deadline for every resource this module creates, as a Go duration."
}

variable "delete_timeout" {
  type        = string
  default     = null
  description = "Overall delete deadline for every resource this module creates, as a Go duration."
}
