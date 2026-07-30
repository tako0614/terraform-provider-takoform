terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 1.0.1"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_dns_record" "example" {
  name        = "dns-record"
  record_name = "api"
  record_type = "CNAME"
  values      = ["service.portable-conformance.invalid"]
  ttl_seconds = 300

  connections = [
    {
      name        = "parent"
      resource    = "DnsZone/primary"
      permissions = ["administer"]
      projection  = "dns.zone.v1"
    },
  ]
}

output "dns_record_outputs" {
  value = takoform_dns_record.example.outputs
}
