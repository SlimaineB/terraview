terraform {
  required_version = ">= 1.4"
}

resource "terraform_data" "network" {
  input = {
    name = "acme-network"
    cidr = "10.0.0.0/16"
  }
}

resource "terraform_data" "subnet" {
  input = {
    network = terraform_data.network.id
    cidr    = "10.0.1.0/24"
  }
}

resource "terraform_data" "cluster" {
  input = {
    subnet = terraform_data.subnet.id
    name   = "acme-eks"
  }
}
