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

resource "takoform_dns_zone" "example" {
  name                = "dns-zone"
  domain              = "portable-conformance.invalid"
  default_ttl_seconds = 3600
}

output "dns_zone_outputs" {
  value = takoform_dns_zone.example.outputs
}
