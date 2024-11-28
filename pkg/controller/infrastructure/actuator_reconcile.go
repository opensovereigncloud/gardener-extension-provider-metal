// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gardener/gardener/extensions/pkg/controller"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"github.com/go-logr/logr"

	metalv1alpha1 "github.com/ironcore-dev/gardener-extension-provider-metal/pkg/apis/metal/v1alpha1"
)

var (
	infrastructureConfig metalv1alpha1.InfrastructureConfig
)

// Reconcile implements infrastructure actuator reconciliation
func (a *actuator) Reconcile(ctx context.Context, log logr.Logger, infra *extensionsv1alpha1.Infrastructure, cluster *controller.Cluster) error {
	err := json.Unmarshal(cluster.Shoot.Spec.Provider.InfrastructureConfig.Raw, &infrastructureConfig)
	if err != nil {
		return fmt.Errorf("failed to unmarshal infrastructure config: %w", err)
	}

	var newNodes []string
	if infrastructureConfig.Networks != nil {
		for _, network := range infrastructureConfig.Networks {
			if network.Workers == "" {
				return fmt.Errorf("network name is required")
			}
			newNodes = append(newNodes, network.Workers)
		}
	}

	if !equalStringSlices(infra.Status.Networking.Nodes, newNodes) {
		infra.Status.Networking.Nodes = newNodes
	}

	return a.reconcile(ctx, log, infra, cluster)
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (a *actuator) reconcile(ctx context.Context, log logr.Logger, infra *extensionsv1alpha1.Infrastructure, cluster *controller.Cluster) error {
	return nil
}
