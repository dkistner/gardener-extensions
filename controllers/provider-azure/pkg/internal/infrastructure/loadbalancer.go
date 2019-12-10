package infrastructure

import (
	"context"
	"fmt"
	"net/http"

	azurev1alpha1 "github.com/gardener/gardener-extensions/controllers/provider-azure/pkg/apis/azure/v1alpha1"
	"github.com/gardener/gardener-extensions/controllers/provider-azure/pkg/internal"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"

	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2019-06-01/network"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
)

var outboundName = "outbound"

// EnsureLoadBalancer ...
func EnsureLoadBalancer(ctx context.Context, clientAuth *internal.ClientAuth, infra *extensionsv1alpha1.Infrastructure, infraStatus *azurev1alpha1.InfrastructureStatus) error {
	lbClient, pubipClient, err := newAzureClients(clientAuth)
	if err != nil {
		return err
	}

	publicIP, err := ensureOutboundPublicIP(ctx, pubipClient, infra, infraStatus)
	if err != nil {
		return err
	}

	if err = ensureLoadBalancer(ctx, clientAuth, lbClient, infra, infraStatus, publicIP); err != nil {
		return err
	}

	return nil
}

// DeleteLoadBalancer ...
func DeleteLoadBalancer(ctx context.Context, clientAuth *internal.ClientAuth, infra *extensionsv1alpha1.Infrastructure) error {
	fmt.Println("DELETE LB")
	fmt.Println("Not implemented yet")
	return nil
}

func ensureOutboundPublicIP(ctx context.Context, pubipClient *network.PublicIPAddressesClient, infra *extensionsv1alpha1.Infrastructure, infraStatus *azurev1alpha1.InfrastructureStatus) (*network.PublicIPAddress, error) {
	fmt.Println("ENSURE OUTBOUND IP")
	exists, publicIP, err := getPublicIP(ctx, pubipClient, &infraStatus.ResourceGroup.Name)
	if err != nil {
		return nil, err
	}
	if exists {
		fmt.Println("ip already exists")
		infraStatus.OutboundIPs = append(infraStatus.OutboundIPs, azurev1alpha1.PublicIP{
			Name:          *publicIP.Name,
			ResourceGroup: &infraStatus.ResourceGroup.Name,
			IP:            publicIP.IPAddress,
		})
		return publicIP, nil
	}

	fmt.Println("create ip")
	parameters := network.PublicIPAddress{
		Location: &infra.Spec.Region,
		Sku: &network.PublicIPAddressSku{
			Name: network.PublicIPAddressSkuNameStandard,
		},
		PublicIPAddressPropertiesFormat: &network.PublicIPAddressPropertiesFormat{
			PublicIPAllocationMethod: network.Static,
		},
	}
	pubIPResult, err := pubipClient.CreateOrUpdate(ctx, infraStatus.ResourceGroup.Name, outboundName, parameters)
	if err != nil {
		return nil, err
	}
	err = pubIPResult.WaitForCompletionRef(ctx, pubipClient.Client)
	if err != nil {
		return nil, err
	}

	exists, publicIP, err = getPublicIP(ctx, pubipClient, &infraStatus.ResourceGroup.Name)
	if err != nil {
		return nil, err
	}
	if exists {
		infraStatus.OutboundIPs = append(infraStatus.OutboundIPs, azurev1alpha1.PublicIP{
			Name:          *publicIP.Name,
			ResourceGroup: &infraStatus.ResourceGroup.Name,
			IP:            publicIP.IPAddress,
		})
		return publicIP, nil
	}
	return nil, fmt.Errorf("public IP does not exists after creation")
}

func createVanillaLoadBalancer(ctx context.Context, clientAuth *internal.ClientAuth, lbClient *network.LoadBalancersClient, infra *extensionsv1alpha1.Infrastructure, infraStatus *azurev1alpha1.InfrastructureStatus, publicIP *network.PublicIPAddress) error {
	var (
		frontentIPConfigID = getFrontentIPConfigID(clientAuth.SubscriptionID, infraStatus.ResourceGroup.Name, infraStatus.ResourceGroup.Name, outboundName)
		backendPoolID      = getBackendPoolID(clientAuth.SubscriptionID, infraStatus.ResourceGroup.Name, infraStatus.ResourceGroup.Name, infraStatus.ResourceGroup.Name)
		parameters         = network.LoadBalancer{
			Location: &infra.Spec.Region,
			Sku: &network.LoadBalancerSku{
				Name: network.LoadBalancerSkuNameStandard,
			},
			LoadBalancerPropertiesFormat: &network.LoadBalancerPropertiesFormat{
				FrontendIPConfigurations: &[]network.FrontendIPConfiguration{
					{
						FrontendIPConfigurationPropertiesFormat: &network.FrontendIPConfigurationPropertiesFormat{
							PublicIPAddress: publicIP,
						},
						Name: &outboundName,
					},
				},
				BackendAddressPools: &[]network.BackendAddressPool{
					{
						Name: &infraStatus.ResourceGroup.Name,
					},
				},
				OutboundRules: &[]network.OutboundRule{
					{
						Name: &outboundName,
						OutboundRulePropertiesFormat: &network.OutboundRulePropertiesFormat{
							FrontendIPConfigurations: &[]network.SubResource{{
								ID: &frontentIPConfigID,
							}},
							BackendAddressPool: &network.SubResource{
								ID: &backendPoolID,
							},
						},
					},
				},
			},
		}
	)
	operation, err := lbClient.CreateOrUpdate(ctx, infraStatus.ResourceGroup.Name, infraStatus.ResourceGroup.Name, parameters)
	if err != nil {
		return err
	}
	err = operation.WaitForCompletionRef(ctx, lbClient.Client)
	if err != nil {
		return err
	}

	infraStatus.LoadBalancer = &azurev1alpha1.LoadBalancer{
		Name: infraStatus.ResourceGroup.Name,
	}
	return nil
}

func ensureLoadBalancer(ctx context.Context, clientAuth *internal.ClientAuth, lbClient *network.LoadBalancersClient, infra *extensionsv1alpha1.Infrastructure, infraStatus *azurev1alpha1.InfrastructureStatus, publicIP *network.PublicIPAddress) error {
	fmt.Println("ENSURE LB")
	loadBalancer, exists, err := getLoadBalancer(ctx, lbClient, &infraStatus.ResourceGroup.Name, &infraStatus.ResourceGroup.Name)
	if err != nil {
		return err
	}

	if !exists {
		fmt.Println("Create new load balancer")
		if err := createVanillaLoadBalancer(ctx, clientAuth, lbClient, infra, infraStatus, publicIP); err != nil {
			return err
		}
		return nil
	}
	fmt.Println("Check existing lb")

	// Check if the Load Balancer has a backend pool.
	hasBackendPool := false
	for _, pool := range *loadBalancer.BackendAddressPools {
		if *pool.Name == infraStatus.ResourceGroup.Name {
			hasBackendPool = true
			break
		}
	}
	if !hasBackendPool {
		fmt.Println("Need to create backend pool")
		loadBalancer.LoadBalancerPropertiesFormat.BackendAddressPools = &[]network.BackendAddressPool{
			{
				Name: &infraStatus.ResourceGroup.Name,
			},
		}
	}

	// Check if the LoadBalancer has a frontend ip configuration for outbound.
	var frontendIPConfigExists, publicIPAssigned bool
	for _, fcfg := range *loadBalancer.FrontendIPConfigurations {
		if *fcfg.Name == outboundName {
			frontendIPConfigExists = true
			// Check if the public ip is assigned to the frontendIPConfiguration.
			if fcfg.PublicIPAddress != nil && *fcfg.PublicIPAddress.ID == *publicIP.ID {
				publicIPAssigned = true
			}
			break
		}
	}

	if !frontendIPConfigExists {
		fmt.Println("Need to assign frontend ip config")
		frontendIPConfigs := append(*loadBalancer.FrontendIPConfigurations, network.FrontendIPConfiguration{
			FrontendIPConfigurationPropertiesFormat: &network.FrontendIPConfigurationPropertiesFormat{
				PublicIPAddress: publicIP,
			},
			Name: &outboundName,
		})
		loadBalancer.FrontendIPConfigurations = &frontendIPConfigs
	} else if !publicIPAssigned {
		fmt.Println("Need to assign public ip")
		for _, fcfg := range *loadBalancer.FrontendIPConfigurations {
			if *fcfg.Name == outboundName {
				fcfg.PublicIPAddress = publicIP
				break
			}
		}
	}

	var hasOutboundRule bool
	for _, outboundRule := range *loadBalancer.OutboundRules {
		if *outboundRule.Name == outboundName {
			hasOutboundRule = true
			break
		}
	}
	if !hasOutboundRule {
		fmt.Println("Need to assign outbound rule")
		frontentIPConfigID := getFrontentIPConfigID(clientAuth.SubscriptionID, infraStatus.ResourceGroup.Name, infraStatus.ResourceGroup.Name, outboundName)
		backendPoolID := getBackendPoolID(clientAuth.SubscriptionID, infraStatus.ResourceGroup.Name, infraStatus.ResourceGroup.Name, infraStatus.ResourceGroup.Name)
		outboundRules := append(*loadBalancer.OutboundRules, network.OutboundRule{
			Name: &outboundName,
			OutboundRulePropertiesFormat: &network.OutboundRulePropertiesFormat{
				FrontendIPConfigurations: &[]network.SubResource{{
					ID: &frontentIPConfigID,
				}},
				BackendAddressPool: &network.SubResource{
					ID: &backendPoolID,
				},
			},
		})
		loadBalancer.OutboundRules = &outboundRules
	}
	// TODO Reconcile outbound rule reference to frontendipconfig and backend pool

	// Update the LoadBalancer if required.
	if !hasBackendPool || !frontendIPConfigExists || !publicIPAssigned || !hasOutboundRule {
		fmt.Println("Need to update load balancer")
		operation, err := lbClient.CreateOrUpdate(ctx, infraStatus.ResourceGroup.Name, infraStatus.ResourceGroup.Name, *loadBalancer)
		if err != nil {
			return err
		}
		err = operation.WaitForCompletionRef(ctx, lbClient.Client)
		if err != nil {
			return err
		}
	}
	infraStatus.LoadBalancer = &azurev1alpha1.LoadBalancer{
		Name: infraStatus.ResourceGroup.Name,
	}

	return nil
}

func getLoadBalancer(ctx context.Context, lbClient *network.LoadBalancersClient, resourceGroup, loadBalancerName *string) (*network.LoadBalancer, bool, error) {
	request, err := lbClient.GetPreparer(ctx, *resourceGroup, *loadBalancerName, "")
	if err != nil {
		return nil, false, err
	}
	response, err := lbClient.GetSender(request)
	if err != nil {
		return nil, false, err
	}
	if response.StatusCode == http.StatusNotFound {
		return nil, false, nil
	}
	if response.StatusCode != http.StatusOK {
		return nil, true, fmt.Errorf("unknown loadbalancer status code: %d", response.StatusCode)
	}
	result, err := lbClient.GetResponder(response)
	if err != nil {
		return nil, true, err
	}
	return &result, true, nil
}

func getPublicIP(ctx context.Context, pubIPClient *network.PublicIPAddressesClient, resourceGroup *string) (bool, *network.PublicIPAddress, error) {
	request, err := pubIPClient.GetPreparer(ctx, *resourceGroup, outboundName, "")
	if err != nil {
		return false, nil, err
	}

	response, err := pubIPClient.GetSender(request)
	if err != nil {
		return false, nil, err
	}

	if response.StatusCode == http.StatusNotFound {
		return false, nil, nil
	}
	if response.StatusCode != http.StatusOK {
		return false, nil, fmt.Errorf("unknown ip status code: %d", response.StatusCode)
	}

	result, err := pubIPClient.GetResponder(response)
	if err != nil {
		return true, nil, err
	}
	if result.ID == nil {
		return true, nil, fmt.Errorf("public ip exists, but id is unknown. (rg:%s publicipname:%s)", *resourceGroup, outboundName)
	}
	return true, &result, nil
}

func newAzureClients(clientAuth *internal.ClientAuth) (*network.LoadBalancersClient, *network.PublicIPAddressesClient, error) {
	oAuthConfig, err := adal.NewOAuthConfig(azure.PublicCloud.ActiveDirectoryEndpoint, clientAuth.TenantID)
	if err != nil {
		return nil, nil, err
	}

	servicePrincipalToken, err := adal.NewServicePrincipalToken(*oAuthConfig, clientAuth.ClientID, clientAuth.ClientSecret, azure.PublicCloud.ResourceManagerEndpoint)
	if err != nil {
		return nil, nil, err
	}

	authorizer := autorest.NewBearerAuthorizer(servicePrincipalToken)
	lbClient := network.NewLoadBalancersClient(clientAuth.SubscriptionID)
	lbClient.Authorizer = authorizer

	pubipClient := network.NewPublicIPAddressesClient(clientAuth.SubscriptionID)
	pubipClient.Authorizer = authorizer

	return &lbClient, &pubipClient, nil
}

func getFrontentIPConfigID(subscription, resourceGroup, lb, frontendIPConfig string) string {
	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/frontendIPConfigurations/%s", subscription, resourceGroup, lb, frontendIPConfig)
}

func getBackendPoolID(subscription, resourceGroup, lb, backendPool string) string {
	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/backendAddressPools/%s", subscription, resourceGroup, lb, backendPool)
}
