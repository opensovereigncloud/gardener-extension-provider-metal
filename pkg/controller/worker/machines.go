// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gardener/gardener/extensions/pkg/controller/worker"
	genericworkeractuator "github.com/gardener/gardener/extensions/pkg/controller/worker/genericactuator"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	machinecontrollerv1alpha1 "github.com/gardener/machine-controller-manager/pkg/apis/machine/v1alpha1"
	"github.com/imdario/mergo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	metalv1alpha1 "github.com/ironcore-dev/gardener-extension-provider-metal/pkg/apis/metal/v1alpha1"
	"github.com/ironcore-dev/gardener-extension-provider-metal/pkg/metal"
)

// DeployMachineClasses generates and creates the metal specific machine classes.
func (w *workerDelegate) DeployMachineClasses(ctx context.Context) error {
	machineClasses, machineClassSecrets, err := w.generateMachineClassAndSecrets(ctx)
	if err != nil {
		return fmt.Errorf("failed to generate machine classes and machine class secrets: %w", err)
	}

	// apply machine classes and machine secrets
	for _, class := range machineClasses {
		if err := w.client.Patch(ctx, class, client.Apply, client.ForceOwnership, metal.FieldOwner); err != nil {
			return fmt.Errorf("failed to create/patch machineclass %s: %w", client.ObjectKeyFromObject(class), err)
		}
	}
	for _, secret := range machineClassSecrets {
		if err := w.client.Patch(ctx, secret, client.Apply, client.ForceOwnership, metal.FieldOwner); err != nil {
			return fmt.Errorf("failed to create/patch machineclass secret %s: %w", client.ObjectKeyFromObject(secret), err)
		}
	}

	return nil
}

// GenerateMachineDeployments generates the configuration for the desired machine deployments.
func (w *workerDelegate) GenerateMachineDeployments(ctx context.Context) (worker.MachineDeployments, error) {
	var (
		machineDeployments = worker.MachineDeployments{}
		updateStrategy     = gardencorev1beta1.RollingUpdate
	)

	for _, pool := range w.worker.Spec.Pools {

		if pool.UpdateStrategy != nil {
			updateStrategy = *pool.UpdateStrategy
		}

		zoneLen := int32(len(pool.Zones))
		for zoneIndex := range pool.Zones {
			workerPoolHash, err := w.generateHashForWorkerPool(pool)
			if err != nil {
				return nil, err
			}
			var (
				deploymentName = fmt.Sprintf("%s-%s-z%d", w.worker.Namespace, pool.Name, zoneIndex+1)
				className      = fmt.Sprintf("%s-%s", deploymentName, workerPoolHash)
			)
			zoneIdx := int32(zoneIndex)

			machineDeployments = append(machineDeployments, worker.MachineDeployment{
				Name:                 deploymentName,
				ClassName:            className,
				SecretName:           className,
				Minimum:              worker.DistributeOverZones(zoneIdx, pool.Minimum, zoneLen),
				Maximum:              worker.DistributeOverZones(zoneIdx, pool.Maximum, zoneLen),
				MaxSurge:             worker.DistributePositiveIntOrPercent(zoneIdx, pool.MaxSurge, zoneLen, pool.Maximum),
				MaxUnavailable:       worker.DistributePositiveIntOrPercent(zoneIdx, pool.MaxUnavailable, zoneLen, pool.Minimum),
				Labels:               pool.Labels,
				Annotations:          pool.Annotations,
				Taints:               pool.Taints,
				UpdateStrategy:       updateStrategy,
				MachineConfiguration: genericworkeractuator.ReadMachineConfiguration(pool),
			})
		}
	}

	return machineDeployments, nil
}

func (w *workerDelegate) generateMachineClassAndSecrets(ctx context.Context) ([]*machinecontrollerv1alpha1.MachineClass, []*corev1.Secret, error) {
	var (
		machineClasses      []*machinecontrollerv1alpha1.MachineClass
		machineClassSecrets []*corev1.Secret
	)

	for _, pool := range w.worker.Spec.Pools {

		workerConfig := &metalv1alpha1.WorkerConfig{}
		if pool.ProviderConfig != nil && pool.ProviderConfig.Raw != nil {
			if _, _, err := w.decoder.Decode(pool.ProviderConfig.Raw, nil, workerConfig); err != nil {
				return nil, nil, fmt.Errorf("could not decode provider config: %+v", err)
			}
		}

		workerPoolHash, err := w.generateHashForWorkerPool(pool)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to generate hash for worker pool %s: %w", pool.Name, err)
		}

		arch := ptr.Deref(pool.Architecture, v1beta1constants.ArchitectureAMD64)
		machineImage, err := w.findMachineImage(pool.MachineImage.Name, pool.MachineImage.Version, &arch)
		if err != nil {
			return nil, nil, err
		}

		serverLabels, err := w.getServerLabelsForMachine(pool.MachineType, workerConfig)
		if err != nil {
			return nil, nil, err
		}

		machineClassProviderSpec := map[string]any{
			metal.ImageFieldName:        machineImage,
			metal.ServerLabelsFieldName: serverLabels,
		}

		if workerConfig.ExtraIgnition != nil {
			if mergedIgnition, err := w.mergeIgnitionConfig(ctx, workerConfig); err != nil {
				return nil, nil, err
			} else if mergedIgnition != "" {
				machineClassProviderSpec[metal.IgnitionFieldName] = mergedIgnition
				machineClassProviderSpec[metal.IgnitionOverrideFieldName] = workerConfig.ExtraIgnition.Override
			}
		}

		for zoneIndex, zone := range pool.Zones {
			var (
				deploymentName = fmt.Sprintf("%s-%s-z%d", w.worker.Namespace, pool.Name, zoneIndex+1)
				className      = fmt.Sprintf("%s-%s", deploymentName, workerPoolHash)
			)

			// Here we are going to create the necessary objects:
			// 1. construct a MachineClass per zone containing the ProviderSpec needed by the MCM
			// 2. construct a Secret for each MachineClass containing the user-data

			nodeTemplate := &machinecontrollerv1alpha1.NodeTemplate{}
			if pool.NodeTemplate != nil {
				nodeTemplate = &machinecontrollerv1alpha1.NodeTemplate{
					Capacity:     pool.NodeTemplate.Capacity,
					InstanceType: pool.MachineType,
					Region:       w.worker.Spec.Region,
					Zone:         zone,
				}
			}

			machineClassProviderSpec[metal.LabelsFieldName] = map[string]string{
				metal.ClusterNameLabel: w.cluster.ObjectMeta.Name,
			}

			machineClassProviderSpecJSON, err := json.Marshal(machineClassProviderSpec)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to marshal machine class for machine pool %s: %w", pool.Name, err)
			}

			machineClass := &machinecontrollerv1alpha1.MachineClass{
				TypeMeta: metav1.TypeMeta{
					Kind:       "MachineClass",
					APIVersion: machinecontrollerv1alpha1.SchemeGroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      className,
					Namespace: w.worker.Namespace,
				},
				NodeTemplate: nodeTemplate,
				CredentialsSecretRef: &corev1.SecretReference{
					Name:      w.worker.Spec.SecretRef.Name,
					Namespace: w.worker.Spec.SecretRef.Namespace,
				},
				ProviderSpec: runtime.RawExtension{
					Raw: machineClassProviderSpecJSON,
				},
				Provider: metal.Type,
				SecretRef: &corev1.SecretReference{
					Name:      className,
					Namespace: w.worker.Namespace,
				},
			}

			userData, err := worker.FetchUserData(ctx, w.client, w.worker.Namespace, pool)
			if err != nil {
				return nil, nil, err
			}

			machineClassSecret := &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Secret",
					APIVersion: corev1.SchemeGroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      className,
					Namespace: w.worker.Namespace,
					Labels:    map[string]string{v1beta1constants.GardenerPurpose: v1beta1constants.GardenPurposeMachineClass},
				},
				Data: map[string][]byte{
					metal.UserDataFieldName: userData,
				},
			}

			machineClasses = append(machineClasses, machineClass)
			machineClassSecrets = append(machineClassSecrets, machineClassSecret)
		}
	}

	return machineClasses, machineClassSecrets, nil
}

func (w *workerDelegate) generateHashForWorkerPool(pool v1alpha1.WorkerPool) (string, error) {
	// Generate the worker pool hash.
	return worker.WorkerPoolHash(pool, w.cluster, nil, nil)
}

func (w *workerDelegate) getServerLabelsForMachine(machineType string, workerConfig *metalv1alpha1.WorkerConfig) (map[string]string, error) {
	combinedLabels := make(map[string]string)
	for _, t := range w.cloudProfileConfig.MachineTypes {
		if t.Name == machineType {
			for key, value := range t.ServerLabels {
				combinedLabels[key] = value
			}
			break
		}
	}
	for key, value := range workerConfig.ExtraServerLabels {
		combinedLabels[key] = value
	}
	if len(combinedLabels) == 0 {
		return nil, fmt.Errorf("no server labels found for machine type %s or worker config", machineType)
	}
	return combinedLabels, nil
}

func (w *workerDelegate) mergeIgnitionConfig(ctx context.Context, workerConfig *metalv1alpha1.WorkerConfig) (string, error) {
	rawIgnition := &map[string]interface{}{}

	if workerConfig.ExtraIgnition.Raw != "" {
		if err := yaml.Unmarshal([]byte(workerConfig.ExtraIgnition.Raw), rawIgnition); err != nil {
			return "", err
		}
	}

	if workerConfig.ExtraIgnition.SecretRef != nil {
		secret := &corev1.Secret{}
		secretKey := client.ObjectKey{Namespace: w.worker.Namespace, Name: workerConfig.ExtraIgnition.SecretRef.Name}
		if err := w.client.Get(ctx, secretKey, secret); err != nil {
			return "", fmt.Errorf("failed to get ignition secret %s: %w", workerConfig.ExtraIgnition.SecretRef, err)
		}

		secretContent, ok := secret.Data[metal.IgnitionFieldName]
		if !ok {
			return "", fmt.Errorf("ignition key not found in secret %s", workerConfig.ExtraIgnition.SecretRef)
		}

		ignitionSecret := map[string]interface{}{}

		if err := yaml.Unmarshal(secretContent, &ignitionSecret); err != nil {
			return "", err
		}

		// append ignition
		opt := mergo.WithAppendSlice

		// merge both ignitions
		err := mergo.Merge(rawIgnition, ignitionSecret, opt)
		if err != nil {
			return "", err
		}
	}

	// avoid converting empty string to an empty map with non-zero length
	if len(*rawIgnition) == 0 {
		return "", nil
	}

	mergedIgnition, err := yaml.Marshal(rawIgnition)
	if err != nil {
		return "", err
	}

	return string(mergedIgnition), nil
}
