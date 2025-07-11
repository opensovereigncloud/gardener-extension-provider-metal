// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ControlPlaneConfig contains configuration settings for the control plane.
type ControlPlaneConfig struct {
	metav1.TypeMeta `json:",inline"`

	// CloudControllerManager contains configuration settings for the cloud-controller-manager.
	// +optional
	CloudControllerManager *CloudControllerManagerConfig `json:"cloudControllerManager,omitempty"`

	// LoadBalancerConfig contains configuration settings for the shoot loadbalancing.
	// +optional
	LoadBalancerConfig *LoadBalancerConfig `json:"loadBalancerConfig,omitempty"`

	// NodeNamePolicy is a policy for generating hostnames for the worker nodes.
	NodeNamePolicy NodeNamePolicy `json:"nodeNamePolicy,omitempty"`
}

// NodeNamePolicy is a policy for generating hostnames for the worker nodes.
type NodeNamePolicy string

const (
	// NodeNamePolicyBMCName is a policy for generating hostnames based on the BMC.
	NodeNamePolicyBMCName NodeNamePolicy = "BMCName"
	// NodeNamePolicyServerName is a policy for generating hostnames based on the Server.
	NodeNamePolicyServerName NodeNamePolicy = "ServerName"
	// NodeNamePolicyServerClaimName is a policy for generating hostnames based on the ServerClaim.
	NodeNamePolicyServerClaimName NodeNamePolicy = "ServerClaimName"
)

// CloudControllerNetworking contains configuration settings for CCM networking.
type CloudControllerNetworking struct {
	// ConfigureNodeAddresses enables the configuration of node addresses.
	// +optional
	ConfigureNodeAddresses bool `json:"configureNodeAddresses,omitempty"`
	// IPAMKind enables the IPAM integration.
	// +optional
	IPAMKind *IPAMKind `json:"ipamKind,omitempty"`
}

// IPAMKind specifiers the IPAM objects in-use.
type IPAMKind struct {
	// APIGroup is the resource group.
	APIGroup string `json:"apiGroup"`
	// Kind is the resource type.
	Kind string `json:"kind"`
}

// CloudControllerManagerConfig contains configuration settings for the cloud-controller-manager.
type CloudControllerManagerConfig struct {
	// FeatureGates contains information about enabled feature gates.
	// +optional
	FeatureGates map[string]bool `json:"featureGates,omitempty"`

	// Networking contains configuration settings for CCM networking.
	// +optional
	Networking *CloudControllerNetworking `json:"networking,omitempty"`
}

// LoadBalancerConfig contains configuration settings for the shoot loadbalancing.
type LoadBalancerConfig struct {
	// MetallbConfig contains configuration settings for metallb.
	// +optional
	MetallbConfig *MetallbConfig `json:"metallbConfig,omitempty"`

	// CalicoBgpConfig contains configuration settings for calico.
	// +optional
	CalicoBgpConfig *CalicoBgpConfig `json:"calicoBgpConfig,omitempty"`

	// MetalLoadBalancerConfig contains configuration settings for the metal load balancer.
	MetalLoadBalancerConfig *MetalLoadBalancerConfig `json:"metalLoadBalancerConfig,omitempty"`
}

// MetalLoadBalancerConfig contains configuration settings for the metal load balancer.
type MetalLoadBalancerConfig struct {
	// NodeCIDRMask is the mask for the node CIDR.
	NodeCIDRMask int32 `json:"nodeCIDRMask"`
	// AllocateNodeCIDRs enables the allocation of node CIDRs.
	AllocateNodeCIDRs bool `json:"allocateNodeCIDRs"`
	// VNI is the VNI used for IP announcements.
	VNI int32 `json:"vni"`
	// MetalBondServer is the URL of the metal bond server.
	MetalBondServer string `json:"metalBondServer,omitempty"`
}

// MetallbConfig contains configuration settings for metallb.
type MetallbConfig struct {
	// IPAddressPool contains IP address pools for metallb.
	// +optional
	IPAddressPool []string `json:"ipAddressPool,omitempty"`

	// EnableSpeaker enables the metallb speaker.
	// +optional
	EnableSpeaker bool `json:"enableSpeaker,omitempty"`

	// EnableL2Advertisement enables L2 advertisement.
	// +optional
	EnableL2Advertisement bool `json:"enableL2Advertisement,omitempty"`
}

// CalicoBgpConfig contains BGP configuration settings for calico.
type CalicoBgpConfig struct {
	// ASNumber is the default AS number used by a node.
	// +required
	ASNumber int `json:"asNumber"`

	// nodeToNodeMeshEnabled enables the node-to-node mesh.
	// +optional
	NodeToNodeMeshEnabled bool `json:"nodeToNodeMeshEnabled,omitempty"`

	// ServiceLoadBalancerIPs are the CIDR blocks for Kubernetes Service LoadBalancer IPs.
	// +optional
	ServiceLoadBalancerIPs []string `json:"serviceLoadBalancerIPs,omitempty"`

	// ServiceExternalIPs are the CIDR blocks for Kubernetes Service External IPs.
	// +optional
	ServiceExternalIPs []string `json:"serviceExternalIPs,omitempty"`

	// ServiceClusterIPs are the CIDR blocks from which service cluster IPs are allocated.
	// +optional
	ServiceClusterIPs []string `json:"serviceClusterIPs,omitempty"`

	// BGPPeer contains configuration for BGPPeer resource.
	// +optional
	BgpPeer []BgpPeer `json:"bgpPeer,omitempty"`

	// BGPFilter contains configuration for BGPFilter resource.
	// +optional
	BGPFilter []BGPFilter `json:"bgpFilter,omitempty"`
}

// BgpPeer contains configuration for BGPPeer resource.
type BgpPeer struct {
	// PeerIP contains IP address of BGP peer followed by an optional port number to peer with.
	// +required
	PeerIP string `json:"peerIP"`

	// ASNumber contains the AS number of the BGP peer.
	// +required
	ASNumber int `json:"asNumber"`

	// NodeSelector is a key-value pair to select nodes that should have this peering.
	// +optional
	NodeSelector string `json:"nodeSelector,omitempty"`

	// Filters contains the filters for the BGP peer.
	// +optional
	Filters []string `json:"filters,omitempty"`
}

// BGPFilter contains configuration for BGPFilter resource.
type BGPFilter struct {
	// Name is the name of the BGPFilter resource.
	Name string `json:"name"`

	// The ordered set of IPv4 BGPFilter rules acting on exporting routes to a peer.
	// +optional
	ExportV4 []BGPFilterRule `json:"exportV4,omitempty"`

	// The ordered set of IPv4 BGPFilter rules acting on importing routes from a peer.
	// +optional
	ImportV4 []BGPFilterRule `json:"importV4,omitempty"`

	// The ordered set of IPv6 BGPFilter rules acting on exporting routes to a peer.
	// +optional
	ExportV6 []BGPFilterRule `json:"exportV6,omitempty"`

	// The ordered set of IPv6 BGPFilter rules acting on importing routes from a peer.
	// +optional
	ImportV6 []BGPFilterRule `json:"importV6,omitempty"`
}

// BGPFilterRule defines a BGP filter rule consisting a single CIDR block and a filter action for this CIDR.
type BGPFilterRule struct {
	CIDR string `json:"cidr"`

	// +kubebuilder:validation:Enum=Equal;NotEqual;In;NotIn
	MatchOperator string `json:"matchOperator"`

	// +kubebuilder:validation:Enum=Accept;Reject
	Action string `json:"action"`
}
