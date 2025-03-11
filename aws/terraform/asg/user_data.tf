module "amz-tailscale-client" {
  source           = "/Users/lbriggs/src/github/lbrlabs/terraform-cloudinit-tailscale"
  auth_key         = var.tailscale_auth_key
  enable_ssh       = true
  hostname         = var.name
  advertise_tags   = var.advertise_tags
  advertise_routes = [local.vpc_cidr]
  accept_routes    = false
  max_retries      = 10
  retry_delay      = 10
}
