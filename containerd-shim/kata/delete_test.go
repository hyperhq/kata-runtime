// Copyright (c) 2017 Intel Corporation
// Copyright (c) 2018 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package kata

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/containerd/containerd/namespaces"
	taskAPI "github.com/containerd/containerd/runtime/v2/task"
	vc "github.com/kata-containers/runtime/virtcontainers"
	vcAnnotations "github.com/kata-containers/runtime/virtcontainers/pkg/annotations"
	"github.com/kata-containers/runtime/virtcontainers/pkg/vcmock"
	"github.com/stretchr/testify/assert"
)

func TestDeleteContainerSuccessAndFail(t *testing.T) {
	assert := assert.New(t)

	sandbox := &vcmock.Sandbox{
		MockID: testSandboxID,
	}

	rootPath, configPath := testConfigSetup(t)
	defer os.RemoveAll(rootPath)
	configJSON, err := readOCIConfigJSON(configPath)
	assert.NoError(err)

	path, err := createTempContainerIDMapping(testContainerID, sandbox.ID())
	assert.NoError(err)
	defer os.RemoveAll(path)

	s := &service{
		id:         testSandboxID,
		sandbox:    sandbox,
		containers: make(map[string]*container),
		processes:  make(map[uint32]string),
	}

	reqCreate := &taskAPI.CreateTaskRequest{
		ID: testContainerID,
	}
	s.containers[testContainerID], err = newContainer(s, reqCreate, TestPid)
	assert.NoError(err)

	reqDelete := &taskAPI.DeleteRequest{
		ID: testContainerID,
	}
	ctx := namespaces.WithNamespace(context.Background(), "UnitTest")

	testingImpl.StatusContainerFunc = func(sandboxID, containerID string) (vc.ContainerStatus, error) {
		return vc.ContainerStatus{
			ID: testContainerID,
			Annotations: map[string]string{
				vcAnnotations.ContainerTypeKey: string(vc.PodContainer),
				vcAnnotations.ConfigJSONKey:    configJSON,
			},
			State: vc.State{
				State: vc.StateReady,
			},
		}, nil
	}

	defer func() {
		testingImpl.StatusContainerFunc = nil
	}()

	_, err = s.Delete(ctx, reqDelete)
	assert.Error(err)
	assert.True(vcmock.IsMockError(err))

	testingImpl.StopContainerFunc = func(sandboxID, containerID string) (vc.VCContainer, error) {
		return &vcmock.Container{}, nil
	}
	defer func() {
		testingImpl.StopContainerFunc = nil
	}()

	_, err = s.Delete(ctx, reqDelete)
	assert.Error(err)
	assert.True(vcmock.IsMockError(err))

	testingImpl.DeleteContainerFunc = func(sandboxID, containerID string) (vc.VCContainer, error) {
		return &vcmock.Container{}, nil
	}
	defer func() {
		testingImpl.DeleteContainerFunc = nil
	}()

	// Before deleting container, we should checkout the status of container, and stop it.
	_, err = s.Delete(ctx, reqDelete)
	assert.NoError(err)
}

func testConfigSetup(t *testing.T) (rootPath string, configPath string) {
	assert := assert.New(t)

	tmpdir, err := ioutil.TempDir("", "")
	assert.NoError(err)

	bundlePath := filepath.Join(tmpdir, "bundle")
	err = os.MkdirAll(bundlePath, testDirMode)
	assert.NoError(err)

	err = createOCIConfig(bundlePath)
	assert.NoError(err)

	// config json path
	configPath = filepath.Join(bundlePath, "config.json")
	return tmpdir, configPath
}
