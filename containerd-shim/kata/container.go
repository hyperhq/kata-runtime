// Copyright (c) 2018 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package kata

import (
	"github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/errdefs"
	taskAPI "github.com/containerd/containerd/runtime/v2/task"
	"sync"
	"time"
)

type container struct {
	s        *service
	ttyio    *ttyIO
	time     time.Time
	execs    map[string]*exec
	exitIOch chan struct{}
	exitch   chan uint32
	id       string
	stdin    string
	stdout   string
	stderr   string
	bundle   string
	mu       sync.Mutex
	pid      uint32
	exit     uint32
	status   task.Status
	terminal bool
}

func newContainer(s *service, r *taskAPI.CreateTaskRequest, pid uint32) *container {
	c := &container{
		s:        s,
		pid:      pid,
		id:       r.ID,
		bundle:   r.Bundle,
		stdin:    r.Stdin,
		stdout:   r.Stdout,
		stderr:   r.Stderr,
		terminal: r.Terminal,
		execs:    make(map[string]*exec),
		status:   task.StatusCreated,
		exitIOch: make(chan struct{}),
		exitch:   make(chan uint32, 1),
		time:     time.Now(),
	}
	return c
}

func (c *container) getExec(id string) (*exec, error) {
	exec := c.execs[id]

	if exec == nil {
		return nil, errdefs.ToGRPCf(errdefs.ErrNotFound, "exec does not exist %s", id)
	}

	return exec, nil
}
