// Copyright (c) 2018 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package kata

import (
	"testing"
	"github.com/stretchr/testify/assert"
	taskAPI "github.com/containerd/containerd/runtime/v2/task"
)

func TestNewContainer(t *testing.T) {
	assert := assert.New(t)

	_, err := newContainer(nil, nil, TestPid)

	assert.Error(err)
}

func TestGetExec(t *testing.T) {
	assert := assert.New(t)

	r := &taskAPI.CreateTaskRequest{}

	c, err := newContainer(nil, r, TestPid)
	assert.NoError(err)

	_, err = c.getExec("")
	assert.Error(err)

	c.execs = make(map[string]*exec)
	_, err = c.getExec("")
	assert.Error(err)

	c.execs[TestID] = &exec{}
	_, err = c.getExec(TestID)
	assert.NoError(err)
}

