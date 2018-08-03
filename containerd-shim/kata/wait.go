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
	var execs *Exec
	var err error

	processID := c.id
	pid := c.pid
	
	if execID == "" {
		//wait until the io closed, then wait the container
		<-c.exitch
	} else {
		execs, err = c.getExec(execID)
		if err != nil {
			return int32(255), err
		}
		<-execs.exitch
		//This wait could be triggered before exec start which
		//will get the exec's id, thus this assignment must after
		//the exec exit, to make sure it get the exec's id.
		processID = execs.id
		pid = execs.pid
	}

	ret, err := s.sandbox.WaitProcess(c.id, processID)
	if err != nil {
		return ret, err
	}

	timeStamp := time.Now()
	c.mu.Lock()
	if execID == "" {
		c.status = task.StatusStopped
		c.exit = uint32(ret)
		c.time = timeStamp
	} else {
		execs.status = task.StatusStopped
		execs.exitCode = ret
		execs.exitTime = timeStamp
	}
	c.mu.Unlock()

	go cReap(s, int(pid), int(ret), c.id, execID, timeStamp)

	return ret, nil
}
