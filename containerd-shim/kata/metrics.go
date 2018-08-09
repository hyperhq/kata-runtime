// Copyright (c) 2018 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
package kata

import (
	"github.com/containerd/cgroups"
	"github.com/containerd/typeurl"
	google_protobuf "github.com/gogo/protobuf/types"
)

func marshalMetrics(s *service, containerID string) (*google_protobuf.Any, error) {
	stats, err := s.sandbox.StatsContainer(containerID)
	if err != nil {
		return nil, err
	}

	cgStats := stats.CgroupStats

	var hugetlb []*cgroups.HugetlbStat
	for _, v := range cgStats.HugetlbStats {
		hugetlb = append(
			hugetlb,
			&cgroups.HugetlbStat{
				Usage:   v.Usage,
				Max:     v.MaxUsage,
				Failcnt: v.Failcnt,
			})
	}

	var perCPU []uint64
	for _, v := range cgStats.CPUStats.CPUUsage.PercpuUsage {
		perCPU = append(perCPU, v)
	}

	metrics := &cgroups.Metrics{
		Hugetlb: hugetlb,
		Pids: &cgroups.PidsStat{
			Current: cgStats.PidsStats.Current,
			Limit:   cgStats.PidsStats.Limit,
		},
		CPU: &cgroups.CPUStat{
			Usage: &cgroups.CPUUsage{
				Total:  cgStats.CPUStats.CPUUsage.TotalUsage,
				PerCPU: perCPU,
			},
		},
		Memory: &cgroups.MemoryStat{
			Cache: cgStats.MemoryStats.Cache,
			Usage: &cgroups.MemoryEntry{
				Limit: cgStats.MemoryStats.Usage.Limit,
				Usage: cgStats.MemoryStats.Usage.Usage,
			},
		},
	}

	data, err := typeurl.MarshalAny(metrics)
	if err != nil {
		return nil, err
	}

	return data, nil
}
