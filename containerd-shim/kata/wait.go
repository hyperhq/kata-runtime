// Copyright (c) 2018 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package kata

import (
	"github.com/containerd/containerd/api/types/task"
	"github.com/sirupsen/logrus"
	"time"
)

func wait(s *service, c *container, execID string) (int32, error) {
	var execs *exec
	var err error

	processID := c.id
	pid := c.pid

	if execID == "" {
		//wait until the io closed, then wait the container
		<-c.exitIOch
	} else {
		execs, err = c.getExec(execID)
		if err != nil {
			return int32(255), err
		}
		<-execs.exitIOch
		//This wait could be triggered before exec start which
		//will get the exec's id, thus this assignment must after
		//the exec exit, to make sure it get the exec's id.
		processID = execs.id
		pid = execs.pid
	}

	ret, err := s.sandbox.WaitProcess(c.id, processID)
	if err != nil {
		logrus.Errorf("Wait for process cid=%s,processID=%s  failed with err: %v", c.id, processID, err)
	}

	if execID == "" {
		c.exitch <- uint32(ret)
	} else {
		execs.exitch <- uint32(ret)
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
