resource "azurerm_resource_group" "main" {
  name     = "lbr"
  location = "East US"
}

resource "azurerm_virtual_network" "main" {
  name                = "lbr-vnet"
  address_space       = ["10.0.0.0/16"]
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name

  tags = {
    environment = "dev"
  }
}

# Azure Bastion Subnet (must be named exactly "AzureBastionSubnet")
resource "azurerm_subnet" "bastion" {
  name                 = "AzureBastionSubnet"
  resource_group_name  = azurerm_resource_group.main.name
  virtual_network_name = azurerm_virtual_network.main.name
  address_prefixes     = ["10.0.1.0/24"]
}

# Private subnet for VMs
resource "azurerm_subnet" "private" {
  name                 = "private"
  resource_group_name  = azurerm_resource_group.main.name
  virtual_network_name = azurerm_virtual_network.main.name
  address_prefixes     = ["10.0.2.0/24"]
}

resource "azurerm_network_security_group" "vm_nsg" {
  name                = "lbr-nsg"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name

  security_rule {
    name                       = "SSH"
    priority                   = 1001
    direction                  = "Inbound"
    access                     = "Allow"
    protocol                   = "Tcp"
    source_port_range          = "*"
    destination_port_range     = "22"
    source_address_prefix      = "*"
    destination_address_prefix = "*"
  }
}

# Associate NSG with subnet
resource "azurerm_subnet_network_security_group_association" "vm_nsg_association" {
  subnet_id                 = azurerm_subnet.private.id  # private
  network_security_group_id = azurerm_network_security_group.vm_nsg.id
}

# Create public IP for NAT Gateway (for outbound internet access)
resource "azurerm_public_ip" "nat_gateway_pip" {
  name                = "nat-gateway-pip"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  allocation_method   = "Static"
  sku                 = "Standard"
}

# Create NAT Gateway for outbound internet access
resource "azurerm_nat_gateway" "nat_gateway" {
  name                    = "nat-gateway"
  location                = azurerm_resource_group.main.location
  resource_group_name     = azurerm_resource_group.main.name
  sku_name                = "Standard"
  idle_timeout_in_minutes = 10
}

# Associate NAT Gateway with public IP
resource "azurerm_nat_gateway_public_ip_association" "nat_gateway_pip_association" {
  nat_gateway_id       = azurerm_nat_gateway.nat_gateway.id
  public_ip_address_id = azurerm_public_ip.nat_gateway_pip.id
  depends_on = [
    azurerm_nat_gateway.nat_gateway,
    azurerm_public_ip.nat_gateway_pip
  ]
}

# Associate NAT Gateway with subnet
resource "azurerm_subnet_nat_gateway_association" "nat_gateway_association" {
  subnet_id      = azurerm_subnet.private.id
  nat_gateway_id = azurerm_nat_gateway.nat_gateway.id
  depends_on = [
    azurerm_nat_gateway.nat_gateway,
    azurerm_subnet.private,
    azurerm_nat_gateway_public_ip_association.nat_gateway_pip_association
  ]
}


module "azure-tailscale" {
  source      = "lbrlabs/tailscale/cloudinit"
  version     = "0.0.7"
  auth_key    = var.tailscale_auth_key
  enable_ssh  = true
  hostname    = "azure"
  max_retries = 10
  retry_delay = 10
}

resource "azurerm_network_interface" "vm_nic" {
  count               = 2
  name                = "lbr-vm-${count.index}-nic"
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
  name                = "lbr-ppg"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
}

resource "azurerm_linux_virtual_machine" "vm" {
  count               = 1
  name                = "lbr-${count.index}-vm-mc"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name
  size                = "Standard_D2als_v6"
  admin_username      = "azureuser"
  zone = 1
  
  # Disable password authentication and use SSH keys
  disable_password_authentication = true

  proximity_placement_group_id = azurerm_proximity_placement_group.vm_ppg.id
  
  # Pass Tailscale cloud-init user data

  network_interface_ids = [
    azurerm_network_interface.vm_nic[count.index].id,
  ]

  user_data = module.azure-tailscale.rendered

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