# Using the Azure provider extension with Gardener as end-user

The [`core.gardener.cloud/v1alpha1.Shoot` resource](https://github.com/gardener/gardener/blob/master/example/90-shoot.yaml) declares a few fields that are meant to contain provider-specific configuration.

In this document we are describing how this configuration looks like for Azure and provide an example `Shoot` manifest with minimal configuration that you can use to create an Azure cluster (modulo the landscape-specific information like cloud profile names, secret binding names, etc.).

## Provider secret data

Every shoot cluster references a `SecretBinding` which itself references a `Secret`, and this `Secret` contains the provider credentials of your Azure subscription.
This `Secret` must look as follows:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: core-azure
  namespace: garden-dev
type: Opaque
data:
  clientID: base64(client-id)
  clientSecret: base64(client-secret)
  subscriptionID: base64(subscription-id)
  tenantID: base64(tenant-id)
```

Please look up https://docs.microsoft.com/en-us/azure/active-directory/develop/howto-create-service-principal-portal as well.

## `InfrastructureConfig`

The infrastructure configuration mainly describes how the network layout looks like in order to create the shoot worker nodes in a latter step, thus, prepares everything relevant to create VMs, load balancers, volumes, etc.

An example `InfrastructureConfig` for the Azure extension looks as follows:

```yaml
apiVersion: azure.provider.extensions.gardener.cloud/v1alpha1
kind: InfrastructureConfig
networks:
  vnet: # specify either 'name' and 'resourceGroup' or 'cidr'
  # name: my-vnet
  # resourceGroup: my-vnet-resource-group
    cidr: 10.250.0.0/16
  workers: 10.250.0.0/19
# serviceEndpoints:
# - entry1
  zoned: false
```

The `networks.vnet` section describes whether you want to create the shoot cluster in an already existing VNet or whether to create a new one:

* If `networks.vnet.name` and `networks.vnet.resourceGroup` are given then you have to specify the VNet name and resource group name of the existing VNet that was created by other means (manually, other tooling, ...).
* If `networks.vnet.cidr` is given then you have to specify the VNet CIDR of a new VNet that will be created during shoot creation.
You can freely choose a private CIDR range.
* Either `networks.vnet.name` or `networks.vnet.cidr` must be present, but not both at the same time.

The `networks.workers` section describes the CIDR for a subnet that is used for all shoot worker nodes, i.e., VMs which later run your applications.
The specified CIDR range must be contained in the VNet CIDR specified above, or the VNet CIDR of your already existing VNet.
You can freely choose this CIDR and it is your responsibility to properly design the network layout to suit your needs.

In the `networks.serviceEndpoints[]` list you can specify the list of Azure service endpoints which shall be associated with the worker subnet.

Via the `.zoned` boolean you can tell whether you want to use Azure availability zones or not.
If you don't use zones then an availability set will be created and only basic load balancers will be used.
Zoned clusters use standard load balancers.

Apart from the VNet and the worker subnet the Azure extension will also create a dedicated resource group, route tables, security groups, and an availability set (if not using zoned clusters).

## `ControlPlaneConfig`

The control plane configuration mainly contains values for the Azure-specific control plane components.
Today, the only component deployed by the Azure extension is the `cloud-controller-manager`.

An example `ControlPlaneConfig` for the Azure extension looks as follows:

```yaml
apiVersion: azure.provider.extensions.gardener.cloud/v1alpha1
kind: ControlPlaneConfig
cloudControllerManager:
  featureGates:
    CustomResourceValidation: true
```

The `cloudControllerManager.featureGates` contains a map of explicitly enabled or disabled feature gates.
For production usage it's not recommend to use this field at all as you can enable alpha features or disable beta/stable features, potentially impacting the cluster stability.
If you don't want to configure anything for the `cloudControllerManager` simply omit the key in the YAML specification.

## Example `Shoot` manifest (non-zoned)

Please find below an example `Shoot` manifest for a non-zoned cluster:

```yaml
apiVersion: core.gardener.cloud/v1alpha1
kind: Shoot
metadata:
  name: johndoe-azure
  namespace: garden-dev
spec:
  cloudProfileName: azure
  region: westeurope
  secretBindingName: core-azure
  provider:
    type: azure
    infrastructureConfig:
      apiVersion: azure.provider.extensions.gardener.cloud/v1alpha1
      kind: InfrastructureConfig
      networks:
        vnet:
          cidr: 10.250.0.0/16
        workers: 10.250.0.0/19
      zoned: false
    controlPlaneConfig:
      apiVersion: azure.provider.extensions.gardener.cloud/v1alpha1
      kind: ControlPlaneConfig
    workers:
    - name: worker-xoluy
      machine:
        type: Standard_D4_v3
      minimum: 2
      maximum: 2
      volume:
        size: 50Gi
        type: standard
  networking:
    nodes: 10.250.0.0/16
    type: calico
  kubernetes:
    version: 1.16.1
  maintenance:
    autoUpdate:
      kubernetesVersion: true
      machineImageVersion: true
  addons:
    kubernetes-dashboard:
      enabled: true
    nginx-ingress:
      enabled: true
```

## Example `Shoot` manifest (zoned)

Please find below an example `Shoot` manifest for a zoned cluster:

```yaml
apiVersion: core.gardener.cloud/v1alpha1
kind: Shoot
metadata:
  name: johndoe-azure
  namespace: garden-dev
spec:
  cloudProfileName: azure
  region: westeurope
  secretBindingName: core-azure
  provider:
    type: azure
    infrastructureConfig:
      apiVersion: azure.provider.extensions.gardener.cloud/v1alpha1
      kind: InfrastructureConfig
      networks:
        vnet:
          cidr: 10.250.0.0/16
        workers: 10.250.0.0/19
      zoned: true
    controlPlaneConfig:
      apiVersion: azure.provider.extensions.gardener.cloud/v1alpha1
      kind: ControlPlaneConfig
    workers:
    - name: worker-xoluy
      machine:
        type: Standard_D4_v3
      minimum: 2
      maximum: 2
      volume:
        size: 50Gi
        type: standard
      zones:
      - "1"
      - "2"
  networking:
    nodes: 10.250.0.0/16
    type: calico
  kubernetes:
    version: 1.16.1
  maintenance:
    autoUpdate:
      kubernetesVersion: true
      machineImageVersion: true
  addons:
    kubernetes-dashboard:
      enabled: true
    nginx-ingress:
      enabled: true
```
