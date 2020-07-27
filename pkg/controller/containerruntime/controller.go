// Copyright (c) 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package containerruntime

import (
	"time"

	extensionshandler "github.com/gardener/gardener-extensions/pkg/handler"
	extensionspredicate "github.com/gardener/gardener-extensions/pkg/predicate"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	// FinalizerPrefix is the prefix name of the finalizer written by this controller.
	FinalizerName = "extensions.gardener.cloud/containerruntime"
	// ControllerName is the name of the controller.
	ControllerName = "containerruntime_controller"
)

// AddArgs are arguments for adding an ContainerRuntime resources controller to a manager.
type AddArgs struct {
	// Actuator is an ContainerRuntime resource actuator.
	Actuator Actuator
	// FinalizerSuffix is the suffix for the finalizer name.
	FinalizerSuffix string
	// ControllerOptions are the controller options used for creating a controller.
	// The options.Reconciler is always overridden with a reconciler created from the
	// given actuator.
	ControllerOptions controller.Options
	// Predicates are the predicates to use.
	Predicates []predicate.Predicate
	// Resync determines the requeue interval.
	Resync time.Duration
	// Type is the type of the resource considered for reconciliation.
	Type string
}

// Add adds an ContainerRuntime controller to the given manager using the given AddArgs.
func Add(mgr manager.Manager, args AddArgs) error {
	args.ControllerOptions.Reconciler = NewReconciler(mgr, args.Actuator)
	return add(mgr, args)
}

// DefaultPredicates returns the default predicates for an containerruntime reconciler.
func DefaultPredicates(ignoreOperationAnnotation bool) []predicate.Predicate {
	if ignoreOperationAnnotation {
		return []predicate.Predicate{
			predicate.GenerationChangedPredicate{},
		}
	}
	return []predicate.Predicate{
		extensionspredicate.Or(
			extensionspredicate.HasOperationAnnotation(),
			extensionspredicate.LastOperationNotSuccessful(),
			extensionspredicate.IsDeleting(),
		),
		extensionspredicate.ShootNotFailed(),
		extensionspredicate.Or(
			extensionspredicate.HasOperationAnnotation(),
			predicate.GenerationChangedPredicate{},
		),
	}
}

func add(mgr manager.Manager, args AddArgs) error {
	ctrl, err := controller.New(ControllerName, mgr, args.ControllerOptions)
	if err != nil {
		return err
	}

	predicates := extensionspredicate.AddTypePredicate(args.Predicates, args.Type)

	if err := ctrl.Watch(&source.Kind{Type: &extensionsv1alpha1.ContainerRuntime{}}, &handler.EnqueueRequestForObject{}, predicates...); err != nil {
		return err
	}

	return ctrl.Watch(&source.Kind{Type: &extensionsv1alpha1.Cluster{}}, &extensionshandler.EnqueueRequestsFromMapFunc{
		ToRequests: extensionshandler.SimpleMapper(ClusterToContainerResourceMapper(predicates...), extensionshandler.UpdateWithNew),
	})
}