// Copyright (c) 2018 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package infrastructure

import (
	"context"
	"time"

	"github.com/gardener/gardener-extensions/controllers/provider-azure/pkg/internal"
	"github.com/gardener/gardener-extensions/controllers/provider-azure/pkg/internal/infrastructure"
	infrainternal "github.com/gardener/gardener-extensions/controllers/provider-azure/pkg/internal/infrastructure"
	"github.com/gardener/gardener-extensions/pkg/controller"
	controllererrors "github.com/gardener/gardener-extensions/pkg/controller/error"
	"github.com/gardener/gardener-extensions/pkg/terraformer"

	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
)

// Reconcile implements infrastructure.Actuator.
func (a *actuator) Reconcile(ctx context.Context, infra *extensionsv1alpha1.Infrastructure, cluster *controller.Cluster) error {
	config, err := internal.InfrastructureConfigFromInfrastructure(infra)
	if err != nil {
		return err
	}

	clientAuth, err := infrastructure.GetClientAuthFromInfrastructure(ctx, a.client, infra)
	if err != nil {
		return err
	}

	terraformFiles, err := infrastructure.RenderTerraformerChart(a.chartRenderer, infra, clientAuth, config, cluster)
	if err != nil {
		return err
	}

	tf, err := internal.NewTerraformer(a.restConfig, clientAuth, infrastructure.TerraformerPurpose, infra.Namespace, infra.Name)
	if err != nil {
		return err
	}

	if err := tf.
		InitializeWith(terraformer.DefaultInitializer(a.client, terraformFiles.Main, terraformFiles.Variables, terraformFiles.TFVars)).
		Apply(); err != nil {

		a.logger.Error(err, "failed to apply the terraform config", "infrastructure", infra.Name)
		return &controllererrors.RequeueAfterError{
			Cause:        err,
			RequeueAfter: 30 * time.Second,
		}
	}

	status, err := infrainternal.ComputeStatus(tf, config)
	if err != nil {
		return err
	}

	// Handle the load balancer lifecycle independently.
	if config.OutboundConnectivity != nil && config.OutboundConnectivity.StableEgressIP && config.Zoned {
		if err := infrastructure.EnsureLoadBalancer(ctx, clientAuth, infra, status); err != nil {
			return err
		}
	}

	return a.updateProviderStatus(ctx, infra, status)
}
