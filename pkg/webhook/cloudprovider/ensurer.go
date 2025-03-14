// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package cloudprovider

import (
	"context"
	"fmt"

	"github.com/gardener/gardener/extensions/pkg/webhook/cloudprovider"
	gcontext "github.com/gardener/gardener/extensions/pkg/webhook/context"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"
	clientcmdv1 "k8s.io/client-go/tools/clientcmd/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	apismetal "github.com/ironcore-dev/gardener-extension-provider-ironcore-metal/pkg/apis/metal"
	"github.com/ironcore-dev/gardener-extension-provider-ironcore-metal/pkg/metal"
)

// NewEnsurer creates cloudprovider ensurer.
func NewEnsurer(logger logr.Logger, mgr manager.Manager) cloudprovider.Ensurer {
	return &ensurer{
		logger:  logger,
		client:  mgr.GetClient(),
		decoder: serializer.NewCodecFactory(mgr.GetScheme(), serializer.EnableStrict).UniversalDecoder(),
	}
}

type ensurer struct {
	logger  logr.Logger
	client  client.Client
	decoder runtime.Decoder
}

// EnsureCloudProviderSecret ensures that cloudprovider secret contains
// the shared credentials file.
func (e *ensurer) EnsureCloudProviderSecret(ctx context.Context, gctx gcontext.GardenContext, newCloudProviderSecret, _ *corev1.Secret) error {
	token, ok := newCloudProviderSecret.Data[metal.TokenFieldName]
	if !ok {
		return fmt.Errorf("could not mutate cloudprovider secret as %q field is missing", metal.TokenFieldName)
	}
	namespace, ok := newCloudProviderSecret.Data[metal.NamespaceFieldName]
	if !ok {
		return fmt.Errorf("could not mutate cloudprovider secret as %q field is missing", metal.NamespaceFieldName)
	}
	username, ok := newCloudProviderSecret.Data[metal.UsernameFieldName]
	if !ok {
		return fmt.Errorf("could not mutate cloud provider secret as %q fied is missing", metal.UsernameFieldName)
	}

	cluster, err := gctx.GetCluster(ctx)
	if err != nil {
		return fmt.Errorf("failed to get cluster: %w", err)
	}

	cloudProfileConfig := &apismetal.CloudProfileConfig{}
	raw, err := cluster.CloudProfile.Spec.ProviderConfig.MarshalJSON()
	if err != nil {
		return fmt.Errorf("could not decode cluster object's providerConfig: %w", err)
	}
	if _, _, err := e.decoder.Decode(raw, nil, cloudProfileConfig); err != nil {
		return fmt.Errorf("could not decode cluster object's providerConfig: %w", err)
	}

	kubeconfig := &clientcmdv1.Config{
		CurrentContext: cluster.Shoot.Spec.Region,
		Clusters: []clientcmdv1.NamedCluster{{
			Name: cluster.Shoot.Spec.Region,
		}},
		AuthInfos: []clientcmdv1.NamedAuthInfo{{
			Name: string(username),
			AuthInfo: clientcmdv1.AuthInfo{
				Token: string(token),
			},
		}},
		Contexts: []clientcmdv1.NamedContext{{
			Name: cluster.Shoot.Spec.Region,
			Context: clientcmdv1.Context{
				Cluster:   cluster.Shoot.Spec.Region,
				AuthInfo:  string(username),
				Namespace: string(namespace),
			},
		}},
	}

	var regionFound bool
	for _, region := range cloudProfileConfig.RegionConfigs {
		if region.Name == cluster.Shoot.Spec.Region {
			kubeconfig.Clusters[0].Cluster.Server = region.Server
			kubeconfig.Clusters[0].Cluster.CertificateAuthorityData = region.CertificateAuthorityData
			regionFound = true
			break
		}
	}
	if !regionFound {
		return fmt.Errorf("faild to find region %s in cloudprofile", cluster.Shoot.Spec.Region)
	}

	raw, err = runtime.Encode(clientcmdlatest.Codec, kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to encode kubeconfig: %w", err)
	}

	newCloudProviderSecret.Data[metal.KubeConfigFieldName] = raw
	return nil
}
