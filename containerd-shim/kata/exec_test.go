// Copyright (c) 2017 Intel Corporation
// Copyright (c) 2018 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package kata

import (
	"context"
	"testing"

	"github.com/containerd/containerd/namespaces"
	taskAPI "github.com/containerd/containerd/runtime/v2/task"
	"github.com/kata-containers/runtime/virtcontainers/pkg/vcmock"
	"github.com/stretchr/testify/assert"
)

func TestExecNoSpecFail(t *testing.T) {
	assert := assert.New(t)

	sandbox := &vcmock.Sandbox{
		MockID: testSandboxID,
	}

	s := &service{
		id:         testSandboxID,
		sandbox:    sandbox,
		containers: make(map[string]*container),
		processes:  make(map[uint32]string),
	}

	reqCreate := &taskAPI.CreateTaskRequest{
		ID: testContainerID,
	}

	var err error
	s.containers[testContainerID], err = newContainer(s, reqCreate, TestPid)
	assert.NoError(err)

	reqExec := &taskAPI.ExecProcessRequest{
		ID:     testContainerID,
		ExecID: testContainerID,
	}
	ctx := namespaces.WithNamespace(context.Background(), "UnitTest")

	_, err = s.Exec(ctx, reqExec)
	assert.Error(err)
}
