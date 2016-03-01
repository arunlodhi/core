// Copyright (c) 2016 Pani Networks
// All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package ipam

import (
	//	"database/sql"
	"github.com/romana/core/common"
	"log"
)

type Vm struct {
	//	Id        uint64 `json:"id"`
	Ip           string `json:"ip"`
	TenantId     string `json:"tenant_id"`
	SegmentId    string `json:"segment_id"`
	HostId       string `json:"host_id"`
	Name         string `json:"instance"`
	RequestToken string `json:"request_token" sql:"unique"`
	// Ordinal number of this VM in the host/tenant combination
	Seq uint64 `json:"sequence"`
	// Calculated effective sequence number of this VM --
	// taking into account stride (endpoint space bits)
	// and alignment thereof. This is used in IP calculation.
	EffectiveSeq uint64 `json:"effective_sequence"`
}

type IpamHost struct {
	Vms []IpamVm
	Id  string `sql:"unique_index"`
}

type IpamSegment struct {
	Vms []IpamVm
	Id  string `sql:"unique_index"`
}

// TODO change this to Endpoint
type IpamVm struct {
	Vm

	Id uint64 `sql:"AUTO_INCREMENT"`
	//	IpamHostId sql.NullString
}

// ipamStore is a backing store for IPAM service.
type ipamStore struct {
	common.DbStore
}

func (ipamStore *ipamStore) addVm(stride uint, vm *Vm) error {
	tx := ipamStore.DbStore.Db.Begin()
	// TODO
	// JB: Isn't "Db" and therefore "Db.Begin()" really a lower-level 
	// implementation detail? Should users of DbStore need to know this? 
	// What if at some point we replace this with a NoSQL store, where we won't have gorm?
	// Maybe DbStore itself should offer 'begin', 'end', 'commit', whatever, so that it can 
	// effectively hide those lower level implementation choices?
	// GG: I agree but the functionality does not exactly map well to RDBMS backing store 
	// and hiding it neatly doesn't work - yet.
	row := tx.Model(IpamVm{}).Where("host_id = ? AND segment_id = ?", vm.HostId, vm.SegmentId).Select("IFNULL(MAX(seq),-1)+1").Row()
	row.Scan(&vm.Seq)
	log.Printf("New sequence is %d\n", vm.Seq)

	// vmSeq is the sequence number of VM in a given host
	effectiveVmSeq := getEffectiveSeq(vm.Seq, stride)
	log.Printf("Effective sequence for seq %d (stride %d): %d\n", vm.Seq, stride, effectiveVmSeq)
	vm.EffectiveSeq = effectiveVmSeq
	ipamVm := IpamVm{Vm: *vm}
	tx.NewRecord(ipamVm)
	tx.Create(&ipamVm)
	err := common.MakeMultiError(ipamStore.DbStore.Db.GetErrors())
	if err != nil {
		tx.Rollback()
		return err
	}
	tx.Commit()
	return nil
}

// getEffectiveSeq gets effective sequence number of a VM
// on a given host.
func getEffectiveSeq(vmSeq uint64, stride uint) uint64 {
	var effectiveVmSeq uint64
	// We start with 3 because we reserve 1 for gateway
	// and 2 for DHCP.
	effectiveVmSeq = 3 + (1<<stride)*vmSeq
	return effectiveVmSeq
}

// Entities implements Entities method of Service interface.
func (ipamStore ipamStore) Entities() []interface{} {
	retval := make([]interface{}, 1)
	retval[0] = IpamVm{}
	return retval
}

// CreateSchemaPostProcess implements CreateSchemaPostProcess method of
// Service interface.
func (ipamStore ipamStore) CreateSchemaPostProcess() error {
	ipamStore.Db.Model(&IpamVm{}).AddUniqueIndex("idx_segment_host_seq", "segment_id", "host_id", "seq")
	return nil
}
