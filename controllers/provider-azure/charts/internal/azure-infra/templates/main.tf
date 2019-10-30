provider "azurerm" {
  subscription_id = "{{ required "azure.subscriptionID is required" .Values.azure.subscriptionID }}"
  tenant_id       = "{{ required "azure.tenantID is required" .Values.azure.tenantID }}"
  client_id       = "${var.CLIENT_ID}"
  client_secret   = "${var.CLIENT_SECRET}"
}

resource "azurerm_resource_group" "rg" {
  name     = "{{ required "clusterName is required" .Values.clusterName }}"
  location = "{{ required "azure.region is required" .Values.azure.region }}"
}

#=====================================================================
#= VNet, Subnets, Route Table, Security Groups
#=====================================================================

{{ if .Values.create.vnet -}}
resource "azurerm_virtual_network" "vnet" {
  name                = "{{ required "vnet.name is required" .Values.vnet.name }}"
  resource_group_name = "${azurerm_resource_group.rg.name}"
  location            = "{{ required "azure.region is required" .Values.azure.region }}"
  address_space       = ["{{ required "vnet.cidr is required" .Values.vnet.cidr }}"]
}
{{- end }}

resource "azurerm_subnet" "workers" {
  name                      = "{{ required "clusterName is required" .Values.clusterName }}-nodes"
  {{ if .Values.create.vnet -}}
  resource_group_name       = "${azurerm_resource_group.rg.name}"
  {{ else -}}
  resource_group_name       = "{{ required "vnet.resourceGroup is required" .Values.vnet.resourceGroup }}"
  {{ end -}}
  virtual_network_name      = "{{ required "vnet.name is required" .Values.vnet.name }}"
  address_prefix            = "{{ required "vnet.subnet.cidr is required" .Values.vnet.subnet.cidr }}"
  service_endpoints         = [{{range $index, $serviceEndpoint := .Values.vnet.subnet.serviceEndpoints}}{{if $index}},{{end}}"{{$serviceEndpoint}}"{{end}}]
  route_table_id            = "${azurerm_route_table.workers.id}"
  network_security_group_id = "${azurerm_network_security_group.workers.id}"
}

resource "azurerm_route_table" "workers" {
  name                = "worker_route_table"
  location            = "{{ required "azure.region is required" .Values.azure.region }}"
  resource_group_name = "${azurerm_resource_group.rg.name}"
}

resource "azurerm_network_security_group" "workers" {
  name                = "{{ required "clusterName is required" .Values.clusterName }}-workers"
  location            = "{{ required "azure.region is required" .Values.azure.region }}"
  resource_group_name = "${azurerm_resource_group.rg.name}"
}

{{ if .Values.create.availabilitySet -}}
#=====================================================================
#= Availability Set
#=====================================================================

resource "azurerm_availability_set" "workers" {
  name                         = "{{ required "clusterName is required" .Values.clusterName }}-avset-workers"
  resource_group_name          = "${azurerm_resource_group.rg.name}"
  location                     = "{{ required "azure.region is required" .Values.azure.region }}"
  platform_update_domain_count = "{{ required "azure.countUpdateDomains is required" .Values.azure.countUpdateDomains }}"
  platform_fault_domain_count  = "{{ required "azure.countFaultDomains is required" .Values.azure.countFaultDomains }}"
  managed                      = true
}
{{- end}}

//=====================================================================
//= Output variables
//=====================================================================

output "{{ .Values.outputKeys.resourceGroupName }}" {
  value = "${azurerm_resource_group.rg.name}"
}

output "{{ .Values.outputKeys.vnetName }}" {
  value = "{{ required "vnet.name is required" .Values.vnet.name }}"
}

{{ if not .Values.create.vnet -}}
output "{{ .Values.outputKeys.vnetResourceGroup }}" {
  value = "{{ required "vnet.resourceGroup is required" .Values.vnet.resourceGroup }}"
}
{{- end}}

output "{{ .Values.outputKeys.subnetName }}" {
  value = "${azurerm_subnet.workers.name}"
}

output "{{ .Values.outputKeys.routeTableName }}" {
  value = "${azurerm_route_table.workers.name}"
}

output "{{ .Values.outputKeys.securityGroupName }}" {
  value = "${azurerm_network_security_group.workers.name}"
}

{{ if .Values.create.availabilitySet -}}
output "{{ .Values.outputKeys.availabilitySetID }}" {
  value = "${azurerm_availability_set.workers.id}"
}

output "{{ .Values.outputKeys.availabilitySetName }}" {
  value = "${azurerm_availability_set.workers.name}"
}
{{- end}}