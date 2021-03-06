// Copyright (c) 2016 Pani Networks
// All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//  http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package agent

import (
	"fmt"
	"github.com/golang/glog"
	"github.com/romana/core/common"
	"github.com/romana/core/pkg/util/firewall"
)

// addPolicy is a placeholder. TODO
func (a *Agent) addPolicy(input interface{}, ctx common.RestContext) (interface{}, error) {
	//	policy := input.(*common.Policy)
	return nil, nil
}

// deletePolicy is a placeholder. TODO
func (a *Agent) deletePolicy(input interface{}, ctx common.RestContext) (interface{}, error) {
	//	policyId := ctx.PathVariables["policyID"]
	return nil, nil
}

// listPolicies is a placeholder. TODO.
func (a *Agent) listPolicies(input interface{}, ctx common.RestContext) (interface{}, error) {
	return nil, nil
}

// Status is a structure containing statistics returned by statusHandler
type Status struct {
	Rules      []firewall.IPtablesRule `json:"rules"`
	Interfaces []NetIf                 `json:"interfaces"`
}

// statusHandler reports operational statistics.
func (a *Agent) statusHandler(input interface{}, ctx common.RestContext) (interface{}, error) {
	glog.V(1).Infoln("Agent: Entering statusHandler()")
	fw, err := firewall.NewFirewall(a.Helper.Executor, a.store, a.networkConfig)
	if err != nil {
		return nil, err
	}

	rules, err := fw.ListRules()
	if err != nil {
		return nil, err
	}
	ifaces, err := a.store.listNetIfs()
	if err != nil {
		return nil, err
	}
	status := Status{Rules: rules, Interfaces: ifaces}
	return status, nil
}

// podDownHandler cleans up after pod deleted.
func (a *Agent) podDownHandler(input interface{}, ctx common.RestContext) (interface{}, error) {
	glog.V(1).Infoln("Agent: Entering podDownHandler()")
	netReq := input.(*NetworkRequest)
	netif := netReq.NetIf

	// We need new firewall instance here to use it's Cleanup()
	// to uninstall firewall rules related to the endpoint.
	fw, err := firewall.NewFirewall(a.Helper.Executor, a.store, a.networkConfig)
	if err != nil {
		return nil, err
	}

	err = fw.Cleanup(netif)
	if err != nil {
		return nil, err
	}

	// Spawn new thread to process the request
	glog.Infof("Agent: Got request for pod teardown %v\n", netReq)

	return "OK", nil
}

// podUpHandler handles HTTP requests for endpoints provisioning.
func (a *Agent) podUpHandler(input interface{}, ctx common.RestContext) (interface{}, error) {
	glog.Infof("Agent: Entering podUpHandler()")
	netReq := input.(*NetworkRequest)

	glog.Infof("Agent: Got request for network configuration: %v\n", netReq)
	// Spawn new thread to process the request

	// TODO don't know if fork-bombs are possible in go but if they are this
	// need to be refactored as buffered channel with fixed pool of workers
	go a.podUpHandlerAsync(*netReq)

	// TODO I wonder if this should actually return something like a
	// link to a status of this request which will later get updated
	// with success or failure -- Greg.
	return "OK", nil
}

// vmDownHandler handles HTTP requests for endpoints teardown.
func (a *Agent) vmDownHandler(input interface{}, ctx common.RestContext) (interface{}, error) {
	glog.Infof("In vmDownHandler() with %T %v", input, input)
	netif := input.(*NetIf)
	if netif.Name == "" {
		// This is a request from OpenStack Mech driver who does not have a name,
		// let's find it by mac.
		err := a.store.findNetIf(netif)
		if err != nil {
			return nil, err
		}
	}
	glog.Infof("In vmDownHandler() with Name %s, IP %s Mac %s\n", netif.Name, netif.IP, netif.Mac)

	glog.Info("Agent: provisioning DHCP")
	if err := a.leaseFile.provisionLease(netif, leaseRemove); err != nil {
		glog.Error(agentError(err))
		return "Error removing DHCP lease", agentError(err)
	}

	// We need new firewall instance here to use it's Cleanup()
	// to uninstall firewall rules related to the endpoint.
	fw, err := firewall.NewFirewall(a.Helper.Executor, a.store, a.networkConfig)
	if err != nil {
		return nil, err
	}

	err = fw.Cleanup(netif)
	if err != nil {
		return nil, err
	}
	err = a.store.deleteNetIf(netif)
	if err != nil {
		return nil, err
	}
	return "OK", nil
}

// vmUpHandler handles HTTP requests for endpoints provisioning.
// Currently tested with Romana ML2 driver.
func (a *Agent) vmUpHandler(input interface{}, ctx common.RestContext) (interface{}, error) {
	// Parse out NetIf form the request
	netif := input.(*NetIf)

	glog.Infof("Got interface: Name %s, IP %s Mac %s\n", netif.Name, netif.IP, netif.Mac)

	// Spawn new thread to process the request

	// TODO don't know if fork-bombs are possible in go but if they are this
	// need to be refactored as buffered channel with fixed pool of workers
	go a.vmUpHandlerAsync(*netif)

	// TODO I wonder if this should actually return something like a
	// link to a status of this request which will later get updated
	// with success or failure -- Greg.
	return "OK", nil
}

// podUpHandlerAsync does a number of operations on given endpoint to ensure
// it's connected:
// 1. Ensures interface is ready
// 2. Creates ip route pointing new interface
// 3. Provisions firewall rules
func (a *Agent) podUpHandlerAsync(netReq NetworkRequest) error {
	glog.V(1).Info("Agent: Entering podUpHandlerAsync()")

	netif := netReq.NetIf
	if netif.Name == "" {
		return agentErrorString("Agent: Interface name required")
	}
	if !a.Helper.waitForIface(netif.Name) {
		// TODO should we resubmit failed interface in queue for later
		// retry ? ... considering openstack will give up as well after
		// timeout
		msg := fmt.Sprintf("Requested interface not available in time - %s", netif.Name)
		glog.Infoln("Agent: ", msg)
		return agentErrorString(msg)
	}
	glog.Info("Agent: creating endpoint routes")
	if err := a.Helper.ensureRouteToEndpoint(&netif); err != nil {
		glog.Error(agentError(err))
		return agentError(err)
	}

	glog.Info("Agent: provisioning firewall")
	fw, err := firewall.NewFirewall(a.Helper.Executor, a.store, a.networkConfig)
	if err != nil {
		glog.Error(agentError(err))
		return agentError(err)
	}

	if err1 := fw.Init(netif); err1 != nil {
		glog.Error(agentError(err))
		return agentError(err)
	}

	metadata := fw.Metadata()
	chainNames := metadata["chains"].([]string)

	// Default firewall rules for Kubernetes
	var defaultRules []firewall.FirewallRule

	// ProvisionEndpoint applies default rules in reverse order
	// so DROP goes first
	inboundChain := chainNames[firewall.InputChainIndex]

	inboundRule := firewall.NewFirewallRule()
	inboundRule.SetBody(fmt.Sprintf("%s %s", inboundChain, "-m comment --comment DefaultDrop -j DROP"))
	defaultRules = append(defaultRules, inboundRule)

	inboundRule = firewall.NewFirewallRule()
	inboundRule.SetBody(fmt.Sprintf("%s %s", inboundChain, "-m state --state ESTABLISHED -j ACCEPT"))
	defaultRules = append(defaultRules, inboundRule)

	forwardOutChain := chainNames[firewall.ForwardOutChainIndex]
	forwardOutRule := firewall.NewFirewallRule()
	forwardOutRule.SetBody(fmt.Sprintf("%s %s", forwardOutChain, "-m comment --comment Outgoing -j RETURN"))
	defaultRules = append(defaultRules, forwardOutRule)

	forwardInChain := chainNames[firewall.ForwardInChainIndex]
	forwardInRule := firewall.NewFirewallRule()
	forwardInRule.SetBody(fmt.Sprintf("%s %s", forwardInChain, "-m state --state RELATED,ESTABLISHED -j ACCEPT"))
	defaultRules = append(defaultRules, forwardInRule)

	fw.SetDefaultRules(defaultRules)

	if err := fw.ProvisionEndpoint(); err != nil {
		glog.Error(agentError(err))
		return agentError(err)
	}

	glog.Info("Agent: All good", netif)
	return nil
}

// vmUpHandlerAsync does a number of operations on given endpoint to ensure
// it's connected:
// 1. Ensures interface is ready
// 2. Checks if DHCP is running
// 3. Creates ip route pointing new interface
// 4. Provisions static DHCP lease for new interface
// 5. Provisions firewall rules
func (a *Agent) vmUpHandlerAsync(netif NetIf) error {
	glog.V(1).Info("Agent: Entering interfaceHandle()")
	if !a.Helper.waitForIface(netif.Name) {
		// TODO should we resubmit failed interface in queue for later
		// retry ? ... considering oenstack will give up as well after
		// timeout
		return agentErrorString(fmt.Sprintf("Requested interface not available in time - %s", netif.Name))
	}

	// dhcpPid is only needed here for fail fast check
	// will try to poll the pid again in provisionLease
	glog.Info("Agent: checking if DHCP is running")
	_, err := a.Helper.DhcpPid()
	if err != nil {
		glog.Error(agentError(err))
		return agentError(err)
	}
	err = a.store.addNetIf(&netif)
	if err != nil {
		return agentError(err)
	}
	glog.Info("Agent: creating endpoint routes")
	if err := a.Helper.ensureRouteToEndpoint(&netif); err != nil {
		glog.Error(agentError(err))
		return agentError(err)
	}
	glog.Info("Agent: provisioning DHCP")
	if err := a.leaseFile.provisionLease(&netif, leaseAdd); err != nil {
		glog.Error(agentError(err))
		return agentError(err)
	}

	glog.Info("Agent: provisioning firewall")
	fw, err := firewall.NewFirewall(a.Helper.Executor, a.store, a.networkConfig)
	if err != nil {
		glog.Error(agentError(err))
		return agentError(err)
	}

	if err1 := fw.Init(netif); err1 != nil {
		glog.Error(agentError(err))
		return agentError(err)
	}

	metadata := fw.Metadata()
	chainNames := metadata["chains"].([]string)
	u32filter := metadata["u32filter"]
	hostAddr := a.networkConfig.RomanaGW()

	// Default firewall rules for OpenStack
	inboundChain := chainNames[firewall.InputChainIndex]
	var defaultRules []firewall.FirewallRule

	inboundRule := firewall.NewFirewallRule()
	inboundRule.SetBody(fmt.Sprintf("%s %s", inboundChain, "-m comment --comment DefaultDrop -j DROP"))
	defaultRules = append(defaultRules, inboundRule)

	inboundRule = firewall.NewFirewallRule()
	inboundRule.SetBody(fmt.Sprintf("%s %s", inboundChain, "-m state --state ESTABLISHED -j ACCEPT"))
	defaultRules = append(defaultRules, inboundRule)

	forwardOutChain := chainNames[firewall.ForwardOutChainIndex]
	forwardOutRule := firewall.NewFirewallRule()
	forwardOutRule.SetBody(fmt.Sprintf("%s %s", forwardOutChain, "-m comment --comment Outgoing -j RETURN"))
	defaultRules = append(defaultRules, forwardOutRule)

	forwardInChain := chainNames[firewall.ForwardInChainIndex]
	forwardInRule := firewall.NewFirewallRule()
	forwardInRule.SetBody(fmt.Sprintf("%s %s", forwardInChain, "-m state --state ESTABLISHED -j ACCEPT"))
	defaultRules = append(defaultRules, forwardInRule)

	forwardInRule = firewall.NewFirewallRule()
	forwardInRule.SetBody(fmt.Sprintf("%s ! -s %s -m u32 --u32 %s %s", forwardInChain, hostAddr, u32filter, "-j ACCEPT"))
	defaultRules = append(defaultRules, forwardInRule)

	fw.SetDefaultRules(defaultRules)

	if err := fw.ProvisionEndpoint(); err != nil {
		glog.Error(agentError(err))
		return agentError(err)
	}

	glog.Info("All good", netif)
	return nil
}
