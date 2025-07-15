# # bastion.tf

# # Public IP for Azure Bastion
# resource "azurerm_public_ip" "bastion_pip" {
#   name                = "${var.name}-bastion-pip"
#   location            = azurerm_resource_group.main.location
#   resource_group_name = azurerm_resource_group.main.name
#   allocation_method   = "Static"
#   sku                 = "Standard"
# }

# # Azure Bastion Host
# resource "azurerm_bastion_host" "bastion" {
#   name                = "${var.name}-bastion"
#   location            = azurerm_resource_group.main.location
#   resource_group_name = azurerm_resource_group.main.name
#   sku                 = "Standard"
  
#   # Enable features for SSH and RDP tunneling
#   tunneling_enabled = true
  
#   ip_configuration {
#     name                 = "configuration"
#     subnet_id            = azurerm_subnet.bastion.id  # AzureBastionSubnet
#     public_ip_address_id = azurerm_public_ip.bastion_pip.id
#   }

#   depends_on = [azurerm_subnet.bastion]
# }

