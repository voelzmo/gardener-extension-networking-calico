// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package charts_test

import (
	"fmt"

	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"github.com/gardener/gardener/pkg/chartrenderer"
	mockchartrenderer "github.com/gardener/gardener/pkg/chartrenderer/mock"
	"github.com/gardener/gardener/pkg/utils"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/helm/pkg/manifest"

	calicov1alpha1 "github.com/gardener/gardener-extension-networking-calico/pkg/apis/calico/v1alpha1"
	"github.com/gardener/gardener-extension-networking-calico/pkg/calico"
	"github.com/gardener/gardener-extension-networking-calico/pkg/charts"
	"github.com/gardener/gardener-extension-networking-calico/pkg/imagevector"
)

var (
	trueVar    = true
	falseVar   = false
	mtuVar     = "1430"
	defaultMtu = "1440"
)

var _ = Describe("Chart package test", func() {
	var (
		kubernetesVersion                               = "1.20.0"
		podCIDR                                         = calicov1alpha1.CIDR("12.0.0.0/8")
		nodeCIDR                                        = calicov1alpha1.CIDR("10.250.0.0/8")
		crossSubnet                                     = calicov1alpha1.CrossSubnet
		always                                          = calicov1alpha1.Always
		never                                           = calicov1alpha1.Never
		invalid             calicov1alpha1.IPv4PoolMode = "invalid"
		autodetectionMethod                             = "interface=eth1"
		backendNone                                     = calicov1alpha1.None
		backendVXLan                                    = calicov1alpha1.VXLan
		backendBird                                     = calicov1alpha1.Bird
		backendInvalid                                  = calicov1alpha1.Backend("invalid")
		poolIPIP                                        = calicov1alpha1.PoolIPIP
		poolVXlan                                       = calicov1alpha1.PoolVXLan

		network                       *extensionsv1alpha1.Network
		networkConfigNil              *calicov1alpha1.NetworkConfig
		networkConfigBackendNone      *calicov1alpha1.NetworkConfig
		networkConfigAll              *calicov1alpha1.NetworkConfig
		networkConfigAllMTU           *calicov1alpha1.NetworkConfig
		networkConfigAllEBPFDataplane *calicov1alpha1.NetworkConfig
		networkConfigDeprecated       *calicov1alpha1.NetworkConfig
		networkConfigInvalid          *calicov1alpha1.NetworkConfig
		networkConfigOverlayDisabled  *calicov1alpha1.NetworkConfig

		objectMeta = metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "bar",
		}
	)

	BeforeEach(func() {
		network = &extensionsv1alpha1.Network{
			ObjectMeta: objectMeta,
			Spec: extensionsv1alpha1.NetworkSpec{
				ServiceCIDR: "10.0.0.0/8",
				PodCIDR:     string(podCIDR),
			},
		}
		networkConfigNil = nil
		networkConfigBackendNone = &calicov1alpha1.NetworkConfig{
			Backend: &backendNone,
			IPAM: &calicov1alpha1.IPAM{
				CIDR: &podCIDR,
				Type: "host-local",
			},
		}
		networkConfigAll = &calicov1alpha1.NetworkConfig{
			Backend: &backendVXLan,
			IPAM: &calicov1alpha1.IPAM{
				CIDR: &podCIDR,
				Type: "host-local",
			},
			IPv4: &calicov1alpha1.IPv4{
				Pool:                &poolVXlan,
				Mode:                &crossSubnet,
				AutoDetectionMethod: &autodetectionMethod,
			},
		}
		networkConfigAllMTU = &calicov1alpha1.NetworkConfig{
			Backend: &backendVXLan,
			IPAM: &calicov1alpha1.IPAM{
				CIDR: &podCIDR,
				Type: "host-local",
			},
			IPv4: &calicov1alpha1.IPv4{
				Pool:                &poolVXlan,
				Mode:                &crossSubnet,
				AutoDetectionMethod: &autodetectionMethod,
			},
			VethMTU: &mtuVar,
		}
		networkConfigAllEBPFDataplane = &calicov1alpha1.NetworkConfig{
			Backend: &backendVXLan,
			IPAM: &calicov1alpha1.IPAM{
				CIDR: &podCIDR,
				Type: "host-local",
			},
			IPv4: &calicov1alpha1.IPv4{
				Pool:                &poolVXlan,
				Mode:                &crossSubnet,
				AutoDetectionMethod: &autodetectionMethod,
			},
			EbpfDataplane: &calicov1alpha1.EbpfDataplane{
				Enabled: true,
			},
		}
		networkConfigDeprecated = &calicov1alpha1.NetworkConfig{
			Backend: &backendBird,
			IPAM: &calicov1alpha1.IPAM{
				CIDR: &podCIDR,
				Type: "host-local",
			},
			IPIP:                  &crossSubnet,
			IPAutoDetectionMethod: &autodetectionMethod,
		}
		networkConfigInvalid = &calicov1alpha1.NetworkConfig{
			Backend: &backendInvalid,
			IPAM: &calicov1alpha1.IPAM{
				CIDR: &podCIDR,
				Type: "host-local",
			},
			IPv4: &calicov1alpha1.IPv4{
				Mode:                &invalid,
				AutoDetectionMethod: &autodetectionMethod,
			},
		}
		networkConfigOverlayDisabled = &calicov1alpha1.NetworkConfig{
			Overlay: &calicov1alpha1.Overlay{Enabled: false},
			Backend: &backendNone,
			IPAM: &calicov1alpha1.IPAM{
				CIDR: &podCIDR,
				Type: "host-local",
			},
			IPv4: &calicov1alpha1.IPv4{
				Mode:                &never,
				AutoDetectionMethod: &autodetectionMethod,
			},
			IPAutoDetectionMethod: &autodetectionMethod,
		}
	})

	Describe("#ComputeCalicoChartValues", func() {
		It("empty network config should properly render calico chart values", func() {
			values, err := charts.ComputeCalicoChartValues(network, networkConfigNil, kubernetesVersion, false, true, true, false, string(nodeCIDR))
			Expect(err).To(BeNil())
			Expect(values).To(Equal(map[string]interface{}{
				"images": map[string]interface{}{
					"calico-cni":              imagevector.CalicoCNIImage(kubernetesVersion),
					"calico-typha":            imagevector.CalicoTyphaImage(kubernetesVersion),
					"calico-kube-controllers": imagevector.CalicoKubeControllersImage(kubernetesVersion),
					"calico-node":             imagevector.CalicoNodeImage(kubernetesVersion),
					"calico-podtodaemon-flex": imagevector.CalicoFlexVolumeDriverImage(kubernetesVersion),
					"calico-cpa":              imagevector.ClusterProportionalAutoscalerImage(kubernetesVersion),
					"calico-cpva":             imagevector.ClusterProportionalVerticalAutoscalerImage(kubernetesVersion),
				},
				"global": map[string]string{
					"podCIDR":  network.Spec.PodCIDR,
					"nodeCIDR": string(nodeCIDR),
				},
				"vpa": map[string]interface{}{
					"enabled": false,
				},
				"config": map[string]interface{}{
					"backend": string(calicov1alpha1.Bird),
					"ipam": map[string]interface{}{
						"type":   "host-local",
						"subnet": "usePodCidr",
					},
					"typha": map[string]interface{}{
						"enabled": trueVar,
					},
					"kubeControllers": map[string]interface{}{
						"enabled": trueVar,
					},
					"veth_mtu": defaultMtu,
					"monitoring": map[string]interface{}{
						"enabled":          true,
						"typhaMetricsPort": "9093",
						"felixMetricsPort": "9091",
					},
					"nonPrivileged": false,
					"felix": map[string]interface{}{
						"ipinip": map[string]interface{}{
							"enabled": true,
						},
						"bpf": map[string]interface{}{
							"enabled": false,
						},
						"bpfKubeProxyIPTablesCleanup": map[string]interface{}{
							"enabled": false,
						},
					},
					"ipv4": map[string]interface{}{
						"pool":                string(poolIPIP),
						"mode":                string(always),
						"autoDetectionMethod": nil,
					},
				},
				"pspDisabled": true,
			}))

		})

		It("should disable felix ip in ip and set pool mode to never when setting backend to none", func() {
			values, err := charts.ComputeCalicoChartValues(network, networkConfigBackendNone, kubernetesVersion, false, true, false, false, string(nodeCIDR))
			Expect(err).To(BeNil())
			Expect(values).To(Equal(map[string]interface{}{
				"images": map[string]interface{}{
					"calico-cni":              imagevector.CalicoCNIImage(kubernetesVersion),
					"calico-typha":            imagevector.CalicoTyphaImage(kubernetesVersion),
					"calico-kube-controllers": imagevector.CalicoKubeControllersImage(kubernetesVersion),
					"calico-node":             imagevector.CalicoNodeImage(kubernetesVersion),
					"calico-podtodaemon-flex": imagevector.CalicoFlexVolumeDriverImage(kubernetesVersion),
					"calico-cpa":              imagevector.ClusterProportionalAutoscalerImage(kubernetesVersion),
					"calico-cpva":             imagevector.ClusterProportionalVerticalAutoscalerImage(kubernetesVersion),
				},
				"global": map[string]string{
					"podCIDR":  network.Spec.PodCIDR,
					"nodeCIDR": string(nodeCIDR),
				},
				"vpa": map[string]interface{}{
					"enabled": false,
				},
				"config": map[string]interface{}{
					"backend": string(*networkConfigBackendNone.Backend),
					"ipam": map[string]interface{}{
						"type":   networkConfigBackendNone.IPAM.Type,
						"subnet": string(*networkConfigBackendNone.IPAM.CIDR),
					},
					"typha": map[string]interface{}{
						"enabled": trueVar,
					},
					"kubeControllers": map[string]interface{}{
						"enabled": falseVar,
					},
					"veth_mtu": defaultMtu,
					"monitoring": map[string]interface{}{
						"enabled":          true,
						"typhaMetricsPort": "9093",
						"felixMetricsPort": "9091",
					},
					"nonPrivileged": false,
					"felix": map[string]interface{}{
						"ipinip": map[string]interface{}{
							"enabled": false,
						},
						"bpf": map[string]interface{}{
							"enabled": false,
						},
						"bpfKubeProxyIPTablesCleanup": map[string]interface{}{
							"enabled": false,
						},
					},
					"ipv4": map[string]interface{}{
						"pool":                string(poolIPIP),
						"mode":                string(never),
						"autoDetectionMethod": nil,
					},
				},
				"pspDisabled": false,
			}))
		})

		It("should correctly compute all of the calico chart values", func() {
			values, err := charts.ComputeCalicoChartValues(network, networkConfigAll, kubernetesVersion, true, true, false, false, string(nodeCIDR))
			Expect(err).To(BeNil())
			Expect(values).To(Equal(map[string]interface{}{
				"images": map[string]interface{}{
					"calico-cni":              imagevector.CalicoCNIImage(kubernetesVersion),
					"calico-typha":            imagevector.CalicoTyphaImage(kubernetesVersion),
					"calico-kube-controllers": imagevector.CalicoKubeControllersImage(kubernetesVersion),
					"calico-node":             imagevector.CalicoNodeImage(kubernetesVersion),
					"calico-podtodaemon-flex": imagevector.CalicoFlexVolumeDriverImage(kubernetesVersion),
					"calico-cpa":              imagevector.ClusterProportionalAutoscalerImage(kubernetesVersion),
					"calico-cpva":             imagevector.ClusterProportionalVerticalAutoscalerImage(kubernetesVersion),
				},
				"global": map[string]string{
					"podCIDR":  network.Spec.PodCIDR,
					"nodeCIDR": string(nodeCIDR),
				},
				"vpa": map[string]interface{}{
					"enabled": true,
				},
				"config": map[string]interface{}{
					"backend": string(*networkConfigAll.Backend),
					"ipam": map[string]interface{}{
						"type":   networkConfigAll.IPAM.Type,
						"subnet": string(*networkConfigAll.IPAM.CIDR),
					},
					"typha": map[string]interface{}{
						"enabled": trueVar,
					},
					"kubeControllers": map[string]interface{}{
						"enabled": trueVar,
					},
					"veth_mtu": defaultMtu,
					"monitoring": map[string]interface{}{
						"enabled":          true,
						"typhaMetricsPort": "9093",
						"felixMetricsPort": "9091",
					},
					"nonPrivileged": false,
					"felix": map[string]interface{}{
						"ipinip": map[string]interface{}{
							"enabled": true,
						},
						"bpf": map[string]interface{}{
							"enabled": false,
						},
						"bpfKubeProxyIPTablesCleanup": map[string]interface{}{
							"enabled": false,
						},
					},
					"ipv4": map[string]interface{}{
						"pool":                string(poolVXlan),
						"mode":                string(*networkConfigAll.IPv4.Mode),
						"autoDetectionMethod": *networkConfigAll.IPv4.AutoDetectionMethod,
					},
				},
				"pspDisabled": false,
			}))
		})

		It("should correctly compute all of the calico chart values with mtu", func() {
			values, err := charts.ComputeCalicoChartValues(network, networkConfigAllMTU, kubernetesVersion, false, true, false, false, string(nodeCIDR))
			Expect(err).To(BeNil())
			Expect(values).To(Equal(map[string]interface{}{
				"images": map[string]interface{}{
					"calico-cni":              imagevector.CalicoCNIImage(kubernetesVersion),
					"calico-typha":            imagevector.CalicoTyphaImage(kubernetesVersion),
					"calico-kube-controllers": imagevector.CalicoKubeControllersImage(kubernetesVersion),
					"calico-node":             imagevector.CalicoNodeImage(kubernetesVersion),
					"calico-podtodaemon-flex": imagevector.CalicoFlexVolumeDriverImage(kubernetesVersion),
					"calico-cpa":              imagevector.ClusterProportionalAutoscalerImage(kubernetesVersion),
					"calico-cpva":             imagevector.ClusterProportionalVerticalAutoscalerImage(kubernetesVersion),
				},
				"global": map[string]string{
					"podCIDR":  network.Spec.PodCIDR,
					"nodeCIDR": string(nodeCIDR),
				},
				"vpa": map[string]interface{}{
					"enabled": false,
				},
				"config": map[string]interface{}{
					"backend": string(*networkConfigAll.Backend),
					"ipam": map[string]interface{}{
						"type":   networkConfigAll.IPAM.Type,
						"subnet": string(*networkConfigAll.IPAM.CIDR),
					},
					"typha": map[string]interface{}{
						"enabled": trueVar,
					},
					"kubeControllers": map[string]interface{}{
						"enabled": trueVar,
					},
					"veth_mtu": mtuVar,
					"monitoring": map[string]interface{}{
						"enabled":          true,
						"typhaMetricsPort": "9093",
						"felixMetricsPort": "9091",
					},
					"nonPrivileged": false,
					"felix": map[string]interface{}{
						"ipinip": map[string]interface{}{
							"enabled": true,
						},
						"bpf": map[string]interface{}{
							"enabled": false,
						},
						"bpfKubeProxyIPTablesCleanup": map[string]interface{}{
							"enabled": false,
						},
					},
					"ipv4": map[string]interface{}{
						"pool":                string(poolVXlan),
						"mode":                string(*networkConfigAll.IPv4.Mode),
						"autoDetectionMethod": *networkConfigAll.IPv4.AutoDetectionMethod,
					},
				},
				"pspDisabled": false,
			}))
		})

		It("should correctly compute all of the calico chart values with ebpf dataplane enabled and kube-proxy disabled", func() {
			values, err := charts.ComputeCalicoChartValues(network, networkConfigAllEBPFDataplane, kubernetesVersion, false, false, false, false, string(nodeCIDR))
			Expect(err).To(BeNil())
			Expect(values).To(Equal(map[string]interface{}{
				"images": map[string]interface{}{
					"calico-cni":              imagevector.CalicoCNIImage(kubernetesVersion),
					"calico-typha":            imagevector.CalicoTyphaImage(kubernetesVersion),
					"calico-kube-controllers": imagevector.CalicoKubeControllersImage(kubernetesVersion),
					"calico-node":             imagevector.CalicoNodeImage(kubernetesVersion),
					"calico-podtodaemon-flex": imagevector.CalicoFlexVolumeDriverImage(kubernetesVersion),
					"calico-cpa":              imagevector.ClusterProportionalAutoscalerImage(kubernetesVersion),
					"calico-cpva":             imagevector.ClusterProportionalVerticalAutoscalerImage(kubernetesVersion),
				},
				"global": map[string]string{
					"podCIDR":  network.Spec.PodCIDR,
					"nodeCIDR": string(nodeCIDR),
				},
				"vpa": map[string]interface{}{
					"enabled": false,
				},
				"config": map[string]interface{}{
					"backend": string(*networkConfigAll.Backend),
					"ipam": map[string]interface{}{
						"type":   networkConfigAll.IPAM.Type,
						"subnet": string(*networkConfigAll.IPAM.CIDR),
					},
					"typha": map[string]interface{}{
						"enabled": trueVar,
					},
					"kubeControllers": map[string]interface{}{
						"enabled": trueVar,
					},
					"veth_mtu": defaultMtu,
					"monitoring": map[string]interface{}{
						"enabled":          true,
						"typhaMetricsPort": "9093",
						"felixMetricsPort": "9091",
					},
					"nonPrivileged": false,
					"felix": map[string]interface{}{
						"ipinip": map[string]interface{}{
							"enabled": true,
						},
						"bpf": map[string]interface{}{
							"enabled": true,
						},
						"bpfKubeProxyIPTablesCleanup": map[string]interface{}{
							"enabled": true,
						},
					},
					"ipv4": map[string]interface{}{
						"pool":                string(poolVXlan),
						"mode":                string(*networkConfigAll.IPv4.Mode),
						"autoDetectionMethod": *networkConfigAll.IPv4.AutoDetectionMethod,
					},
				},
				"pspDisabled": false,
			}))
		})

		It("should correctly compute all of the calico chart values with overlay disabled", func() {
			values, err := charts.ComputeCalicoChartValues(network, networkConfigOverlayDisabled, kubernetesVersion, true, true, false, false, string(nodeCIDR))
			Expect(err).To(BeNil())
			Expect(values).To(Equal(map[string]interface{}{
				"images": map[string]interface{}{
					"calico-cni":              imagevector.CalicoCNIImage(kubernetesVersion),
					"calico-typha":            imagevector.CalicoTyphaImage(kubernetesVersion),
					"calico-kube-controllers": imagevector.CalicoKubeControllersImage(kubernetesVersion),
					"calico-node":             imagevector.CalicoNodeImage(kubernetesVersion),
					"calico-podtodaemon-flex": imagevector.CalicoFlexVolumeDriverImage(kubernetesVersion),
					"calico-cpa":              imagevector.ClusterProportionalAutoscalerImage(kubernetesVersion),
					"calico-cpva":             imagevector.ClusterProportionalVerticalAutoscalerImage(kubernetesVersion),
				},
				"global": map[string]string{
					"podCIDR":                  network.Spec.PodCIDR,
					"nodeCIDR":                 string(nodeCIDR),
					"overlayEnabled":           "false",
					"snatToUpstreamDNSEnabled": "true",
				},
				"vpa": map[string]interface{}{
					"enabled": true,
				},
				"config": map[string]interface{}{
					"backend": string(backendNone),
					"ipam": map[string]interface{}{
						"type":   networkConfigOverlayDisabled.IPAM.Type,
						"subnet": string(*networkConfigOverlayDisabled.IPAM.CIDR),
					},
					"typha": map[string]interface{}{
						"enabled": trueVar,
					},
					"kubeControllers": map[string]interface{}{
						"enabled": falseVar,
					},
					"veth_mtu": defaultMtu,
					"monitoring": map[string]interface{}{
						"enabled":          true,
						"typhaMetricsPort": "9093",
						"felixMetricsPort": "9091",
					},
					"nonPrivileged": false,
					"felix": map[string]interface{}{
						"ipinip": map[string]interface{}{
							"enabled": false,
						},
						"bpf": map[string]interface{}{
							"enabled": false,
						},
						"bpfKubeProxyIPTablesCleanup": map[string]interface{}{
							"enabled": false,
						},
					},
					"ipv4": map[string]interface{}{
						"pool":                string(calicov1alpha1.PoolIPIP),
						"mode":                string(*networkConfigOverlayDisabled.IPv4.Mode),
						"autoDetectionMethod": *networkConfigOverlayDisabled.IPv4.AutoDetectionMethod,
					},
				},
				"pspDisabled": false,
			}))
		})

		It("should respect deprecated fields in order to keep backwards compatibility", func() {
			values, err := charts.ComputeCalicoChartValues(network, networkConfigDeprecated, kubernetesVersion, true, true, false, false, string(nodeCIDR))
			Expect(err).To(BeNil())
			Expect(values).To(Equal(map[string]interface{}{
				"images": map[string]interface{}{
					"calico-cni":              imagevector.CalicoCNIImage(kubernetesVersion),
					"calico-typha":            imagevector.CalicoTyphaImage(kubernetesVersion),
					"calico-kube-controllers": imagevector.CalicoKubeControllersImage(kubernetesVersion),
					"calico-node":             imagevector.CalicoNodeImage(kubernetesVersion),
					"calico-podtodaemon-flex": imagevector.CalicoFlexVolumeDriverImage(kubernetesVersion),
					"calico-cpa":              imagevector.ClusterProportionalAutoscalerImage(kubernetesVersion),
					"calico-cpva":             imagevector.ClusterProportionalVerticalAutoscalerImage(kubernetesVersion),
				},
				"global": map[string]string{
					"podCIDR":  network.Spec.PodCIDR,
					"nodeCIDR": string(nodeCIDR),
				},
				"vpa": map[string]interface{}{
					"enabled": true,
				},
				"config": map[string]interface{}{
					"backend": string(*networkConfigDeprecated.Backend),
					"ipam": map[string]interface{}{
						"type":   networkConfigDeprecated.IPAM.Type,
						"subnet": string(*networkConfigDeprecated.IPAM.CIDR),
					},
					"typha": map[string]interface{}{
						"enabled": trueVar,
					},
					"kubeControllers": map[string]interface{}{
						"enabled": trueVar,
					},
					"veth_mtu": defaultMtu,
					"monitoring": map[string]interface{}{
						"enabled":          true,
						"typhaMetricsPort": "9093",
						"felixMetricsPort": "9091",
					},
					"nonPrivileged": false,
					"felix": map[string]interface{}{
						"ipinip": map[string]interface{}{
							"enabled": true,
						},
						"bpf": map[string]interface{}{
							"enabled": false,
						},
						"bpfKubeProxyIPTablesCleanup": map[string]interface{}{
							"enabled": false,
						},
					},
					"ipv4": map[string]interface{}{
						"pool":                string(calicov1alpha1.PoolIPIP),
						"mode":                string(*networkConfigDeprecated.IPIP),
						"autoDetectionMethod": *networkConfigDeprecated.IPAutoDetectionMethod,
					},
				},
				"pspDisabled": false,
			}))
		})

		It("should correctly compute calico chart values when non-privileged mode is enabled", func() {
			values, err := charts.ComputeCalicoChartValues(network, networkConfigAll, kubernetesVersion, true, true, false, true, string(nodeCIDR))
			Expect(err).To(BeNil())

			actual, err := utils.GetFromValuesMap(values, "config", "nonPrivileged")
			Expect(err).To(BeNil())
			Expect(actual).To(BeTrue())
		})

		It("should correctly compute calico chart values when non-privileged mode and ebpf dataplane are enabled", func() {
			values, err := charts.ComputeCalicoChartValues(network, networkConfigAllEBPFDataplane, kubernetesVersion, true, true, false, true, string(nodeCIDR))
			Expect(err).To(BeNil())

			actual, err := utils.GetFromValuesMap(values, "config", "nonPrivileged")
			Expect(err).To(BeNil())
			Expect(actual).To(BeFalse())
		})

		It("should error on invalid config value", func() {
			_, err := charts.ComputeCalicoChartValues(network, networkConfigInvalid, kubernetesVersion, true, true, false, false, string(nodeCIDR))
			Expect(err).To(Equal(fmt.Errorf("error when generating calico config: unsupported value for backend: invalid")))
		})
	})

	Describe("#RenderCalicoChart", func() {
		var (
			ctrl                *gomock.Controller
			mockChartRenderer   *mockchartrenderer.MockInterface
			testManifestContent string
			mkManifest          func(name string) manifest.Manifest
		)
		BeforeEach(func() {
			ctrl = gomock.NewController(GinkgoT())
			mockChartRenderer = mockchartrenderer.NewMockInterface(ctrl)
			testManifestContent = "test-content"
			mkManifest = func(name string) manifest.Manifest {
				return manifest.Manifest{Name: fmt.Sprintf("test/templates/%s", name), Content: testManifestContent}
			}
		})
		It("Render Calico charts correctly", func() {
			mockChartRenderer.EXPECT().Render(calico.CalicoChartPath, calico.ReleaseName, metav1.NamespaceSystem, gomock.Any()).Return(&chartrenderer.RenderedChart{
				ChartName: "test",
				Manifests: []manifest.Manifest{
					mkManifest(charts.CalicoConfigKey),
				},
			}, nil)

			_, err := charts.RenderCalicoChart(mockChartRenderer, network, networkConfigNil, kubernetesVersion, false, true, false, false, string(nodeCIDR))
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
