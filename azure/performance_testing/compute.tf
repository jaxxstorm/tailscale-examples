# compute.tf
module "tailscale" {
  source      = "lbrlabs/tailscale/cloudinit"
  version     = "0.0.7"
  auth_key    = var.tailscale_auth_key
  enable_ssh  = true
  hostname    = var.name
  max_retries = 10
  retry_delay = 10
}

resource "azurerm_network_interface" "vm_nic" {
  count               = 2
  name                = "${var.name}-vm-${count.index}-nic"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  accelerated_networking_enabled = true

  ip_configuration {
    name                          = "internal"
    subnet_id                     = azurerm_subnet.private.id  # private subnet
    private_ip_address_allocation = "Dynamic"
  }
}

resource "azurerm_proximity_placement_group" "vm_ppg" {
  name                = "${var.name}-ppg"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
}

resource "azurerm_linux_virtual_machine" "vm" {
  count               = 2
  name                = "${var.name}-${count.index}-vm"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  size                = var.instance_size
  admin_username      = "azureuser"
  zone = 1
  
  # Disable password authentication and use SSH keys
  disable_password_authentication = true

  proximity_placement_group_id = azurerm_proximity_placement_group.vm_ppg.id
  
  # Pass Tailscale cloud-init user data

  network_interface_ids = [
    azurerm_network_interface.vm_nic[count.index].id,
  ]

  user_data = module.tailscale.rendered

  admin_ssh_key {
    username   = "azureuser"
    public_key = file(var.ssh_public_key_path)
  }

  os_disk {
    caching              = "ReadWrite"
    storage_account_type = "Premium_LRS"
  }

  source_image_reference {
    publisher = "Canonical"
    offer     = "ubuntu-24_04-lts"
    sku       = "server"
    version   = "latest"
  }

  identity {
    type = "SystemAssigned"
  }

  depends_on = [azurerm_subnet.private]
}

# output "connection_info" {
#   value = <<EOT
# VMs Created:
# ${join("\n", [for i, vm in azurerm_linux_virtual_machine.vm : "  ${vm.name}: ${azurerm_network_interface.vm_nic[i].private_ip_address}"])}

# Ready-to-use CLI Commands:

# 1. Connect to VM-0 via Bastion SSH tunnel:
#    az network bastion tunnel --name ${azurerm_bastion_host.bastion.name} --resource-group ${azurerm_resource_group.main.name} --target-resource-id ${azurerm_linux_virtual_machine.vm[0].id} --resource-port 22 --port 2222
   
#    Then in another terminal: ssh azureuser@localhost -p 2222

# 2. Connect to VM-1 via Bastion SSH tunnel:
#    az network bastion tunnel --name ${azurerm_bastion_host.bastion.name} --resource-group ${azurerm_resource_group.main.name} --target-resource-id ${azurerm_linux_virtual_machine.vm[1].id} --resource-port 22 --port 2223
   
#    Then in another terminal: ssh azureuser@localhost -p 2223

# 3. Direct SSH (Azure CLI 2.49+) to VM-0:
#    az network bastion ssh --name ${azurerm_bastion_host.bastion.name} --resource-group ${azurerm_resource_group.main.name} --target-resource-id ${azurerm_linux_virtual_machine.vm[0].id} --auth-type ssh-key --username azureuser

# 4. Run commands remotely on VM-0:
#    az vm run-command invoke --resource-group ${azurerm_resource_group.main.name} --name ${azurerm_linux_virtual_machine.vm[0].name} --command-id RunShellScript --scripts "tailscale status"

# 5. Via Tailscale (after installation):
#    - VMs will automatically join your Tailscale network
#    - SSH directly: ssh azureuser@<tailscale-ip>
# EOT
# }