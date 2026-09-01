terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "~> 4.0"
    }
    aws = {
      source  = "registry.terraform.io/hashicorp/aws"
      version = "~> 6.0"
    }
  }
}

provider "takoform" {
  endpoint = var.takoform_endpoint
  space    = var.takoform_space
}

provider "aws" {
  region = var.aws_region
}

variable "takoform_endpoint" {
  type = string
}

variable "takoform_space" {
  type    = string
  default = "prod"
}

variable "aws_region" {
  type    = string
  default = "ap-northeast-1"
}

variable "artifact_bucket_name" {
  type = string
}

# Industry-standard/provider-native infrastructure remains owned by its own
# provider. Takoform does not wrap S3 as a Form or proxy AWS credentials.
resource "aws_s3_bucket" "artifacts" {
  bucket = var.artifact_bucket_name
}

# tako0614 Forms participate in the same OpenTofu dependency graph.
resource "takoform_edge_kv_namespace" "sessions" {
  name = "sessions"

  depends_on = [aws_s3_bucket.artifacts]
}
