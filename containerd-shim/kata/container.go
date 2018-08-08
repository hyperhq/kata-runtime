// Copyright (c) 2018 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package kata

import (
	"github.com/containerd/containerd/api/types/task"
	vc "github.com/kata-containers/runtime/virtcontainers"
	"sync"
	"time"
)

type Container struct {
	s        *service
	pid      uint32
	id       string
	stdin    string
	stdout   string
	stderr   string
	ttyio    *TtyIO
	terminal bool

	exitIOch chan struct{}
	exitch   chan uint32

	bundle    string
	execs     map[string]*Exec
	container vc.VCContainer
	status    task.Status
	exit      uint32
	time      time.Time

	mu sync.Mutex
}
