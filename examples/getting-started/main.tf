terraform {
  required_providers {
    random = {
      source  = "registry.terraform.io/hashicorp/random"
      version = "~> 3.7"
    }
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "~> 4.0"
    }
  }
}

variable "takoform_endpoint" {
  type        = string
  description = "Endpoint supplied by the operator of the compatible Takoform Host."
}

variable "takoform_space" {
  type        = string
  description = "Exact Space ID supplied for the selected Host."
}

provider "takoform" {
  endpoint = var.takoform_endpoint
  space    = var.takoform_space
  # Supply the Host bearer token through TAKOFORM_TOKEN, not HCL.
}

resource "random_id" "namespace_name" {
  byte_length = 8
}

resource "takoform_edge_kv_namespace" "sessions" {
  # The peer Random provider supplies a suffix; the Takoform provider owns
  # the actual resource in the configured Host.
  name = "sessions-${random_id.namespace_name.hex}"
}
