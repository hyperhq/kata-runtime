// Copyright (c) 2018 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package kata

import (
	"github.com/containerd/containerd/api/types/task"
	"time"
)

func wait(s *service, c *Container, execID string) (int32, error) {
	var err error

	processID := c.id
	pid := c.pid

	if execID == "" {
		//wait until the io closed, then wait the container
		<-c.exitIOch
	}

	ret, err := s.sandbox.WaitProcess(c.id, processID)
	if err != nil {
		return ret, err
	}

	if execID == "" {
		c.exitch <- uint32(ret)
	}

	timeStamp := time.Now()
	c.mu.Lock()
	if execID == "" {
		c.status = task.StatusStopped
		c.exit = uint32(ret)
		c.time = timeStamp
	}
	c.mu.Unlock()

	go cReap(s, int(pid), int(ret), c.id, execID, timeStamp)

	return ret, nil
}
