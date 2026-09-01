variable "name" {
  type        = string
  description = "Portable name of the Module Worker identity. Every revision this module creates is named from its own content and from THIS name as its owner, so this is the ONE name an author chooses, the one that stays stable across deploys, and what keeps two instances of this module apart when they are built from identical output."

  # The canonical portable resource-name grammar, exactly as the provider
  # enforces it (internal/currentformmodel.PatternResourceName). A module that
  # narrows the provider's own grammar rejects names `takoform_module_worker`
  # accepts and sends the author back to the raw resources for no reason.
  validation {
    condition     = can(regex("^[a-z][a-z0-9-]{0,62}$", var.name))
    error_message = "name must be a portable resource name: 1 to 63 characters, starting with a lowercase letter, then lowercase letters, digits, or hyphens."
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

variable "apply_idempotency_key" {
  type        = string
  default     = null
  description = "Optional Provider-only opaque Host API Idempotency-Key for the immutable Worker Version apply. It never enters the portable desired spec. Changing it replaces the Worker Version."
}

variable "vars" {
  # `any`, not `map(any)`. A collection type infers ONE element type for the
  # whole collection, so `{ enabled = true, retries = 3 }` is coerced to a
  # common type — usually strings — before anything encodes it, and the worker
  # receives "true" and "3" where the Form admits a boolean and a number.
  # `any` keeps each value's own type, and the shape is checked below instead.
  type        = any
  default     = {}
  description = "Non-secret configuration projected into the worker environment, as an object of names to JSON values. Each value keeps its own JSON type — boolean, number, string, or a bounded nested object or array — exactly as the Form admits it. Never put a secret value here: it is portable desired state and it is persisted in Terraform state."

  # The SHAPE is what this module is entitled to require: an object rather than
  # a list or a scalar. Which values the Form admits, and which key grammar, are
  # the provider's and the host's to decide, and a module re-stating them would
  # start rejecting what they accept. `jsonencode` preserves each value's own
  # type, so this check cannot itself coerce anything.
  validation {
    condition     = startswith(jsonencode(var.vars), "{")
    error_message = "vars must be an object of names to JSON values, for example { enabled = true, retries = 3, label = \"prod\" }."
  }
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

variable "external_services" {
  type = list(object({
    name     = string
    protocol = string
    required = optional(bool, true)
  }))
  default     = []
  description = "Opaque standard-service slots. Each slot carries only its runtime name, normalized reverse-DNS protocol, and whether the host must satisfy it; endpoints, credentials, and vendor configuration never enter module state."
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
