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

resource "takoform_compute_instance" "example" {
  name          = "compute-instance"
  machine_class = "general.small"
  image         = "portable-conformance/v1/base-linux"
  boot_disk_gib = 20
}

output "compute_instance_outputs" {
  value = takoform_compute_instance.example.outputs
}
