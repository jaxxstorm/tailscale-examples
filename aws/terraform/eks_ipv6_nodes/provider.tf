terraform {
  required_version = ">= 1.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.30"
    }
    tailscale = {
        source = "tailscale/tailscale"
        version = ">= 0.16.0"
    }
  }
}


provider "aws" {
  region = "us-west-2"
}