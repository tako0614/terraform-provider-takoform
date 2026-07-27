terraform {
  required_providers {
    takoform = {
      source = "registry.terraform.io/tako0614/takoform"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_tls_certificate" "example" {
  name          = "tls-certificate"
  domains       = ["portable-conformance.invalid"]
  key_algorithm = "ecdsa_p256"
}

output "tls_certificate_outputs" {
  value = takoform_tls_certificate.example.outputs
}
