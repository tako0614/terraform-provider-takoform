terraform {
  required_providers {
    aws = {
      source  = "registry.terraform.io/hashicorp/aws"
      version = "~> 6.0"
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

variable "aws_region" {
  type        = string
  description = "AWS region selected for this configuration."
}

variable "artifact_bucket_name" {
  type        = string
  description = "S3 bucket name selected for this configuration."
}

provider "takoform" {
  endpoint = var.takoform_endpoint
  space    = var.takoform_space
  # Supply the Host bearer token through TAKOFORM_TOKEN, not HCL.
}

provider "aws" {
  region = var.aws_region
  # AWS authentication remains with the AWS provider's normal credential chain.
}

resource "aws_s3_bucket" "artifacts" {
  bucket = var.artifact_bucket_name
}

resource "takoform_edge_kv_namespace" "sessions" {
  # Choose a name not already used in the selected Space.
  name = "sessions"

  # This is an ordinary OpenTofu dependency edge between peer providers.
  depends_on = [aws_s3_bucket.artifacts]
}
