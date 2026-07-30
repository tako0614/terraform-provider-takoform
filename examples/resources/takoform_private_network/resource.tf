terraform {
  required_providers {
    takoform = {
      source  = "registry.terraform.io/tako0614/takoform"
      version = "= 1.0.2"
    }
  }
}

provider "takoform" {
  endpoint = "https://takoform.example.com"
  space    = "prod"
}

resource "takoform_private_network" "example" {
  name          = "private-network"
  address_space = "10.32.0.0/16"
  public_egress = false
}

output "private_network_outputs" {
  value = takoform_private_network.example.outputs
}
