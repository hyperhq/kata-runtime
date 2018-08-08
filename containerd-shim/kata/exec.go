// Copyright (c) 2018 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package kata

import (
	"time"
	"github.com/containerd/containerd/api/types/task"
	vc "github.com/kata-containers/runtime/virtcontainers"
)

type Exec struct {
	id        string
	pid       uint32
	container *Container
	cmds      *vc.Cmd
	exitCode  int32
	tty       *Tty
	ttyio     *TtyIO
	status    task.Status

	exitIOch chan struct{}
	exitch   chan uint32

	exitTime time.Time
}

type Tty struct {
	stdin    string
	stdout   string
	stderr   string
	height   uint32
	width    uint32
	terminal bool
}