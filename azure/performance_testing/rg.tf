# rg.tf
resource "azurerm_resource_group" "main" {
  name     = "${var.name}-test"
  location = "East US"
}