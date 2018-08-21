// Copyright (c) 2017 Intel Corporation
// Copyright (c) 2018 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package kata

import (
	"context"
	"os"
	"testing"

	"github.com/containerd/containerd/namespaces"
	taskAPI "github.com/containerd/containerd/runtime/v2/task"
	vc "github.com/kata-containers/runtime/virtcontainers"
	vcAnnotations "github.com/kata-containers/runtime/virtcontainers/pkg/annotations"
	"github.com/kata-containers/runtime/virtcontainers/pkg/vcmock"
	"github.com/stretchr/testify/assert"
)

func TestStartStartSandboxSuccess(t *testing.T) {
	assert := assert.New(t)

	sandbox := &vcmock.Sandbox{
		MockID: testSandboxID,
	}

	path, err := createTempContainerIDMapping(sandbox.ID(), sandbox.ID())
	assert.NoError(err)
	defer os.RemoveAll(path)

	testingImpl.StatusContainerFunc = func(sandboxID, containerID string) (vc.ContainerStatus, error) {
		return vc.ContainerStatus{
			ID: sandbox.ID(),
			Annotations: map[string]string{
				vcAnnotations.ContainerTypeKey: string(vc.PodSandbox),
			},
		}, nil
	}

	defer func() {
		testingImpl.StatusContainerFunc = nil
	}()

	s := &service{
		id:         testSandboxID,
		sandbox:    sandbox,
		containers: make(map[string]*container),
		processes:  make(map[uint32]string),
	}

	s.containers[testSandboxID] = &container{
		s:  s,
		id: testSandboxID,
	}

	req := &taskAPI.StartRequest{
		ID: testSandboxID,
	}

	testingImpl.StartSandboxFunc = func(sandboxID string) (vc.VCSandbox, error) {
		return sandbox, nil
	}

	defer func() {
		testingImpl.StartSandboxFunc = nil
	}()

	ctx := namespaces.WithNamespace(context.Background(), "UnitTest")
	_, err = s.Start(ctx, req)
	assert.NoError(err)
	os.RemoveAll(path)
}

func TestStartMissingAnnotation(t *testing.T) {
	assert := assert.New(t)

	sandbox := &vcmock.Sandbox{
		MockID: testSandboxID,
	}

	path, err := createTempContainerIDMapping(sandbox.ID(), sandbox.ID())
	assert.NoError(err)
	defer os.RemoveAll(path)

	testingImpl.StatusContainerFunc = func(sandboxID, containerID string) (vc.ContainerStatus, error) {
		return vc.ContainerStatus{
			ID:          sandbox.ID(),
			Annotations: map[string]string{},
		}, nil
	}

	defer func() {
		testingImpl.StatusContainerFunc = nil
	}()

	s := &service{
		id:         testSandboxID,
		sandbox:    sandbox,
		containers: make(map[string]*container),
		processes:  make(map[uint32]string),
	}

	s.containers[testSandboxID] = &container{
		s:  s,
		id: testSandboxID,
	}

	req := &taskAPI.StartRequest{
		ID: testSandboxID,
	}

	testingImpl.StartSandboxFunc = func(sandboxID string) (vc.VCSandbox, error) {
		return sandbox, nil
	}

	defer func() {
		testingImpl.StartSandboxFunc = nil
	}()

	ctx := namespaces.WithNamespace(context.Background(), "UnitTest")
	_, err = s.Start(ctx, req)
	assert.Error(err)
	os.RemoveAll(path)
	assert.False(vcmock.IsMockError(err))
}

func TestStartStartContainerSucess(t *testing.T) {
	assert := assert.New(t)

	sandbox := &vcmock.Sandbox{
		MockID: testSandboxID,
	}

	sandbox.MockContainers = []*vcmock.Container{
		{
			MockID:      testSandboxID,
			MockSandbox: sandbox,
		},
		{
			MockID:      testContainerID,
			MockSandbox: sandbox,
		},
	}

	path, err := createTempContainerIDMapping(testContainerID, sandbox.ID())
	assert.NoError(err)
	defer os.RemoveAll(path)

	testingImpl.StatusContainerFunc = func(sandboxID, containerID string) (vc.ContainerStatus, error) {
		return vc.ContainerStatus{
			ID: testContainerID,
			Annotations: map[string]string{
				vcAnnotations.ContainerTypeKey: string(vc.PodContainer),
			},
		}, nil
	}

	defer func() {
		testingImpl.StatusContainerFunc = nil
	}()

	testingImpl.StartContainerFunc = func(sandboxID, containerID string) (vc.VCContainer, error) {
		return sandbox.MockContainers[0], nil
	}

	defer func() {
		testingImpl.StartContainerFunc = nil
	}()

	s := &service{
		id:         testSandboxID,
		sandbox:    sandbox,
		containers: make(map[string]*container),
		processes:  make(map[uint32]string),
	}

	s.containers[testContainerID] = &container{
		s:  s,
		id: testContainerID,
	}

	req := &taskAPI.StartRequest{
		ID: testContainerID,
	}

	ctx := namespaces.WithNamespace(context.Background(), "UnitTest")
	_, err = s.Start(ctx, req)
	assert.NoError(err)
	os.RemoveAll(path)
}
