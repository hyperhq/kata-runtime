// Copyright (c) 2018 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package kata

import (
	"github.com/containerd/containerd/api/types/task"
	taskAPI "github.com/containerd/containerd/runtime/v2/task"
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

func newContainer(s *service, r *taskAPI.CreateTaskRequest, pid uint32, container vc.VCContainer) *Container {
	c := &Container{
		s:        s,
		pid:      pid,
		id:       r.ID,
		bundle:   r.Bundle,
		stdin:    r.Stdin,
		stdout:   r.Stdout,
		stderr:   r.Stderr,
		terminal: r.Terminal,
		execs:    make(map[string]*Exec),
		status:   task.StatusCreated,
		exitIOch: make(chan struct{}),
		exitch:   make(chan uint32, 1),
		time:     time.Now(),
	}
	return c
}
