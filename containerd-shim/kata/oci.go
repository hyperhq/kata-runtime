// Copyright (c) 2017 Intel Corporation
// Copyright (c) 2018 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package kata

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	vc "github.com/kata-containers/runtime/virtcontainers"
	"github.com/kata-containers/runtime/virtcontainers/pkg/oci"
	"github.com/opencontainers/runtime-spec/specs-go"
)

const ctrsMappingDirMode = os.FileMode(0750)

var ctrsMapTreePath = "/var/run/kata-containers/containers-mapping"

// getContainerInfo returns the container status and its sandbox ID.
func getContainerInfo(containerID string) (vc.ContainerStatus, string, error) {
	// container ID MUST be provided.
	if containerID == "" {
		return vc.ContainerStatus{}, "", fmt.Errorf("Missing container ID")
	}

	sandboxID, err := fetchContainerIDMapping(containerID)
	if err != nil {
		return vc.ContainerStatus{}, "", err
	}
	if sandboxID == "" {
		// Not finding a container should not trigger an error as
		// getContainerInfo is used for checking the existence and
		// the absence of a container ID.
		return vc.ContainerStatus{}, "", nil
	}

	ctrStatus, err := vci.StatusContainer(sandboxID, containerID)
	if err != nil {
		return vc.ContainerStatus{}, "", err
	}

	return ctrStatus, sandboxID, nil
}

func getExistingContainerInfo(containerID string) (vc.ContainerStatus, string, error) {
	cStatus, sandboxID, err := getContainerInfo(containerID)
	if err != nil {
		return vc.ContainerStatus{}, "", err
	}

	// container ID MUST exist.
	if cStatus.ID == "" {
		return vc.ContainerStatus{}, "", fmt.Errorf("Container ID (%v) does not exist", containerID)
	}

	return cStatus, sandboxID, nil
}

func validCreateParams(containerID, bundlePath string) (string, error) {
	// container ID MUST be provided.
	if containerID == "" {
		return "", fmt.Errorf("Missing container ID")
	}

	// container ID MUST be unique.
	cStatus, _, err := getContainerInfo(containerID)
	if err != nil {
		return "", err
	}

	if cStatus.ID != "" {
		return "", fmt.Errorf("ID already in use, unique ID should be provided")
	}

	// bundle path MUST be provided.
	if bundlePath == "" {
		return "", fmt.Errorf("Missing bundle path")
	}

	// bundle path MUST be valid.
	fileInfo, err := os.Stat(bundlePath)
	if err != nil {
		return "", fmt.Errorf("Invalid bundle path '%s': %s", bundlePath, err)
	}
	if fileInfo.IsDir() == false {
		return "", fmt.Errorf("Invalid bundle path '%s', it should be a directory", bundlePath)
	}

	resolved, err := resolvePath(bundlePath)
	if err != nil {
		return "", err
	}

	return resolved, nil
}

func noNeedForOutput(detach bool, tty bool) bool {
	if !detach {
		return false
	}

	if !tty {
		return false
	}

	return true
}

// This function assumes it should find only one file inside the container
// ID directory. If there are several files, we could not determine which
// file name corresponds to the sandbox ID associated, and this would throw
// an error.
func fetchContainerIDMapping(containerID string) (string, error) {
	if containerID == "" {
		return "", fmt.Errorf("Missing container ID")
	}

	dirPath := filepath.Join(ctrsMapTreePath, containerID)

	files, err := ioutil.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}

		return "", err
	}

	if len(files) != 1 {
		return "", fmt.Errorf("Too many files (%d) in %q", len(files), dirPath)
	}

	return files[0].Name(), nil
}

func addContainerIDMapping(containerID, sandboxID string) error {
	if containerID == "" {
		return fmt.Errorf("Missing container ID")
	}

	if sandboxID == "" {
		return fmt.Errorf("Missing sandbox ID")
	}

	parentPath := filepath.Join(ctrsMapTreePath, containerID)

	if err := os.RemoveAll(parentPath); err != nil {
		return err
	}

	path := filepath.Join(parentPath, sandboxID)

	if err := os.MkdirAll(path, ctrsMappingDirMode); err != nil {
		return err
	}

	return nil
}

func delContainerIDMapping(containerID string) error {
	if containerID == "" {
		return fmt.Errorf("Missing container ID")
	}

	path := filepath.Join(ctrsMapTreePath, containerID)

	return os.RemoveAll(path)
}

func removeNameSpace(s *oci.CompatOCISpec, nsType specs.LinuxNamespaceType){
	for i, n := range s.Linux.Namespaces {
		if n.Type == nsType {
			s.Linux.Namespaces = append(s.Linux.Namespaces[:i], s.Linux.Namespaces[i+1:]...)
			return
		}
	}
}