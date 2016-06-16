package firewall

import (
	"errors"
	utilexec "github.com/romana/core/pkg/util/exec"
	"net"
)

type Firewall interface {
	// ProvisionEndpoint generates and applies rules for given endpoint.
	ProvisionEndpoint(netif FirewallEndpoint) error

	// EnsureRule checks if specified rule in desired state.
	EnsureRule(ruleSpec []string, op opIptablesAction) error

	// Cleanup deletes DB records and uninstall rules associated with given endpoint.
	Cleanup(netif FirewallEndpoint) error
}

// NetConfig is for agent.NetworkConfig.
type NetConfig interface {
	PNetCIDR() (cidr *net.IPNet, err error)
	TenantBits() uint
	SegmentBits() uint
	EndpointBits() uint
	EndpointNetmaskSize() uint
	RomanaGW() net.IP
}

// NewFirewall returns fully initialized firewall struct, with rules and chains
// configured for given endpoint.
func NewFirewall(executor utilexec.Executable, store interface{}, platform FirewallPlatform, nc NetConfig) (Firewall, error) {

	fwstore, ok := store.(firewallStore)
	if ok {
		return Iptables{}, errors.New("Failed to cast store to firewallStore type")
	}

	fw := new(Iptables)
	fw.Store = fwstore
	fw.os = executor
	fw.Platform = platform
	fw.networkConfig = nc

	return *fw, nil
}

type FirewallPlatform int

const (
	KubernetesPlatform FirewallPlatform = iota
	OpenStackPlatform
)

func (fp FirewallPlatform) String() string {
	var result string
	switch fp {
	case KubernetesPlatform:
		return "Kubernetes"
	case OpenStackPlatform:
		return "OpenStack"
	}

	return result
}

// opType for ensureIptablesRule.
// TODO rename in OpFirewallAction
type opIptablesAction int

const (
	ensureLast opIptablesAction = iota
	ensureFirst
	ensureAbsent
)

func (i opIptablesAction) String() string {
	var result string
	switch i {
	case ensureLast:
		result = "Ensuring rule at the bottom"
	case ensureFirst:
		result = "Ensuring rule at the top"
	case ensureAbsent:
		result = "Ensuring rule is absent"
	}

	return result
}

// Interface for agent.NetIf.
type FirewallEndpoint interface {
	GetMac() string
	GetIP() net.IP
	GetName() string
}
