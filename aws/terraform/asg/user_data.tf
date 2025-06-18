module "amz-tailscale-client" {
  source           = "lbrlabs/tailscale/cloudinit"
  version          = "0.0.7"
  auth_key         = var.tailscale_auth_key
  enable_ssh       = true
  hostname         = var.name
  advertise_tags   = var.advertise_tags
  advertise_routes = [local.vpc_cidr]
  accept_routes    = false
  max_retries      = 10
  retry_delay      = 10
  additional_parts = [
    {
      filename     = "attach_eni.sh"
      content_type = "text/x-shellscript"
      content = templatefile(
        "${path.module}/templates/attach_eni.sh.tmpl",
        {
          ENI_ID   = aws_network_interface.main.id
          HOSTNAME = var.hostname
        }
      )
    }
  ]
}
