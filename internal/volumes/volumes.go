// SPDX-License-Identifier: Apache-2.0
// Package volumes enumerates only local storage volumes.
package volumes

import (
	"context"
	"encoding/json"
	"strings"
)

type Volume struct {
	ID, Label, Type, Filesystem string
	Total, Free                 uint64
	Recycle                     bool
}

func (v Volume) Used() uint64 { return v.Total - v.Free }
func (v Volume) UsedPercent() int {
	if v.Total == 0 {
		return 0
	}
	return int(v.Used() * 100 / v.Total)
}

func (v Volume) MarshalJSON() ([]byte, error) {
	type wire struct {
		ID          string `json:"id"`
		Label       string `json:"label"`
		Type        string `json:"type"`
		Filesystem  string `json:"filesystem"`
		Total       uint64 `json:"total"`
		Used        uint64 `json:"used"`
		Free        uint64 `json:"free"`
		UsedPercent int    `json:"used_percent"`
		Recycle     bool   `json:"recycle"`
	}
	return json.Marshal(wire{v.ID, v.Label, v.Type, v.Filesystem, v.Total, v.Used(), v.Free, v.UsedPercent(), v.Recycle})
}

type Provider interface {
	List(context.Context) ([]Volume, error)
}

// acceptedTarget implements ADR-006's fail-closed DOS-device allow list.
func acceptedTarget(targets []string) bool {
	return len(targets) == 1 && strings.HasPrefix(strings.ToLower(targets[0]), `\device\harddiskvolume`)
}
