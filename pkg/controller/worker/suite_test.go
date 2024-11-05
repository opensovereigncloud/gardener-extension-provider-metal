// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package worker

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	extensionscontroller "github.com/gardener/gardener/extensions/pkg/controller"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	gardenerextensionv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	gardener "github.com/gardener/gardener/pkg/client/kubernetes"
	machinescheme "github.com/gardener/machine-controller-manager/pkg/client/clientset/versioned/scheme"
	"github.com/ironcore-dev/controller-utils/modutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	apiextensionsscheme "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/scheme"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	apiv1alpha1 "github.com/ironcore-dev/gardener-extension-provider-metal/pkg/apis/metal/v1alpha1"
)

const (
	pollingInterval      = 50 * time.Millisecond
	eventuallyTimeout    = 10 * time.Second
	consistentlyDuration = 1 * time.Second
)

var (
	testEnv   *envtest.Environment
	cfg       *rest.Config
	k8sClient client.Client
)

// global Gardener resources used by delegates
var (
	shootVersionMajorMinor = "1.2"
	shootVersion           = shootVersionMajorMinor + ".3"

	pool               gardenerextensionv1alpha1.WorkerPool
	cloudProfileConfig *apiv1alpha1.CloudProfileConfig

	clusterWithoutImages   *extensionscontroller.Cluster
	testCluster            *extensionscontroller.Cluster
	cloudProfileConfigJSON []byte

	workerConfig     *apiv1alpha1.WorkerConfig
	workerConfigJSON []byte

	w *gardenerextensionv1alpha1.Worker
)

func TestAPIs(t *testing.T) {
	SetDefaultConsistentlyPollingInterval(pollingInterval)
	SetDefaultEventuallyPollingInterval(pollingInterval)
	SetDefaultEventuallyTimeout(eventuallyTimeout)
	SetDefaultConsistentlyDuration(consistentlyDuration)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Worker Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true), zap.Level(zapcore.InfoLevel)))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			modutils.Dir("github.com/gardener/machine-controller-manager", "kubernetes", "crds", "machine.sapcloud.io_machineclasses.yaml"),
			modutils.Dir("github.com/gardener/machine-controller-manager", "kubernetes", "crds", "machine.sapcloud.io_machinedeployments.yaml"),
			modutils.Dir("github.com/gardener/machine-controller-manager", "kubernetes", "crds", "machine.sapcloud.io_machines.yaml"),
			modutils.Dir("github.com/gardener/machine-controller-manager", "kubernetes", "crds", "machine.sapcloud.io_machinesets.yaml"),
			modutils.Dir("github.com/gardener/machine-controller-manager", "kubernetes", "crds", "machine.sapcloud.io_scales.yaml"),
			filepath.Join("..", "..", "..", "example", "20-crd-extensions.gardener.cloud_controlplanes.yaml"),
			filepath.Join("..", "..", "..", "example", "20-crd-extensions.gardener.cloud_workers.yaml"),
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	// cfg is defined in this file globally.
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	Expect(apiextensionsscheme.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(machinescheme.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(gardenerextensionv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(apiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

	// Init package-level k8sClient
	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	komega.SetClient(k8sClient)
})

func SetupTest() (*corev1.Namespace, *gardener.ChartApplier) {
	var chartApplier gardener.ChartApplier
	ns := &corev1.Namespace{}

	BeforeEach(func(ctx SpecContext) {
		var err error
		*ns = corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "testns-",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed(), "failed to create test namespace")
		DeferCleanup(k8sClient.Delete, ns)

		chartApplier, err = gardener.NewChartApplierForConfig(cfg)
		Expect(err).NotTo(HaveOccurred())

		volumeName := "test-volume"
		volumeType := "fast"

		workerConfig = &apiv1alpha1.WorkerConfig{
			ServerLabels: map[string]string{
				"foo": "bar",
			},
			ExtraIgnition: &apiv1alpha1.IgnitionConfig{
				Raw:      "abc",
				Override: true,
			},
		}
		workerConfigJSON, _ = json.Marshal(workerConfig)

		// define test resources
		pool = gardenerextensionv1alpha1.WorkerPool{
			MachineType:    "large",
			Maximum:        10,
			MaxSurge:       intstr.IntOrString{IntVal: 5},
			MaxUnavailable: intstr.IntOrString{IntVal: 2},
			Annotations:    map[string]string{"foo": "bar"},
			Labels:         map[string]string{"foo": "bar"},
			MachineImage: gardenerextensionv1alpha1.MachineImage{
				Name:    "my-os",
				Version: "1.0",
			},
			Minimum:  0,
			Name:     "pool",
			UserData: []byte("some-data"),
			Volume: &gardenerextensionv1alpha1.Volume{
				Name: &volumeName,
				Type: &volumeType,
				Size: "10Gi",
			},
			Zones:        []string{"zone1", "zone2"},
			Architecture: ptr.To[string]("amd64"),
			NodeTemplate: &gardenerextensionv1alpha1.NodeTemplate{
				Capacity: map[corev1.ResourceName]resource.Quantity{
					corev1.ResourceCPU: resource.MustParse("100m"),
				},
			},
			ProviderConfig: &runtime.RawExtension{
				Raw: workerConfigJSON,
			},
		}
		cloudProfileConfig = &apiv1alpha1.CloudProfileConfig{
			TypeMeta: metav1.TypeMeta{
				APIVersion: apiv1alpha1.SchemeGroupVersion.String(),
				Kind:       "CloudProfileConfig",
			},
			MachineImages: []apiv1alpha1.MachineImages{
				{
					Name: "my-os",
					Versions: []apiv1alpha1.MachineImageVersion{
						{
							Version:      "1.0",
							Image:        "registry/my-os",
							Architecture: ptr.To[string]("amd64"),
						},
					},
				},
			},
		}
		shootVersionMajorMinor = "1.2"
		shootVersion = shootVersionMajorMinor + ".3"
		clusterWithoutImages = &extensionscontroller.Cluster{
			Shoot: &gardencorev1beta1.Shoot{
				Spec: gardencorev1beta1.ShootSpec{
					Kubernetes: gardencorev1beta1.Kubernetes{
						Version: shootVersion,
					},
					Provider: gardencorev1beta1.Provider{
						InfrastructureConfig: &runtime.RawExtension{
							Raw: []byte("{}"),
						},
					},
				},
			},
		}
		cloudProfileConfigJSON, _ = json.Marshal(cloudProfileConfig)
		testCluster = &extensionscontroller.Cluster{
			CloudProfile: &gardencorev1beta1.CloudProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "metal",
				},
				Spec: gardencorev1beta1.CloudProfileSpec{
					ProviderConfig: &runtime.RawExtension{
						Raw: cloudProfileConfigJSON,
					},
				},
			},
			Shoot: clusterWithoutImages.Shoot,
		}
		w = &gardenerextensionv1alpha1.Worker{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pool",
				Namespace: ns.Name,
			},
			Spec: gardenerextensionv1alpha1.WorkerSpec{
				DefaultSpec: gardenerextensionv1alpha1.DefaultSpec{},
				Region:      "foo",
				SecretRef: corev1.SecretReference{
					Name: "my-secret",
				},
				SSHPublicKey: nil,
				Pools: []gardenerextensionv1alpha1.WorkerPool{
					pool,
				},
			},
		}
	})

	return ns, &chartApplier
}
