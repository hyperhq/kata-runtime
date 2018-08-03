// Copyright (c) 2018 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package kata

import (
	"github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/errdefs"
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
	terminal bool
	exitch   chan struct{}

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
		exitch:   make(chan struct{}),
		time:     time.Now(),
	}
	return c
}

func (c *Container) getExec(id string) (*Exec, error) {
	exec := c.execs[id]

	if exec == nil {
		return nil, errdefs.ToGRPCf(errdefs.ErrNotFound, "exec does not exist %s", id)
	}

	return exec, nil
}
