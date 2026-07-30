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

resource "takoform_compute_instance" "example" {
  name                = "compute-instance"
  artifact_media_type = "application/x-qemu-disk"
  artifact_sha256     = "a6cbb7e295a8dd89b98f3b8c731047e0e62a4312b8b43c590a8c8662df59e913"
  artifact_url        = "https://artifacts.portable-conformance.invalid/base-linux.qcow2"
  machine_class       = "general.small"
  boot_disk_gib       = 20
  instance_count      = 1
  configuration       = { "LOG_LEVEL" = "info" }
}

output "compute_instance_outputs" {
  value = takoform_compute_instance.example.outputs
}
