terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 1.0.3"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_cache_cluster" "example" {
  name                = "cache-cluster"
  size_class          = "cache.small"
  eviction_policy     = "least_recently_used"
  default_ttl_seconds = 300
}

output "cache_cluster_outputs" {
  value = takoform_cache_cluster.example.outputs
}
