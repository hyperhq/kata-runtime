// Copyright (c) 2017 Intel Corporation
// Copyright (c) 2018 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package kata

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	k8sEmptyDir = "kubernetes.io~empty-dir"
)

// IsEphemeralStorage returns true if the given path
// to the storage belongs to kubernetes ephemeral storage
//
// This method depends on a specific path used by k8s
// to detect if it's of type ephemeral. As of now,
// this is a very k8s specific solution that works
// but in future there should be a better way for this
// method to determine if the path is for ephemeral
// volume type
func IsEphemeralStorage(path string) bool {
	splitSourceSlice := strings.Split(path, "/")
	if len(splitSourceSlice) > 1 {
		storageType := splitSourceSlice[len(splitSourceSlice)-2]
		if storageType == k8sEmptyDir {
			return true
		}
	}
	return false
}

// resolvePath returns the fully resolved and expanded value of the
// specified path.
func resolvePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path must be specified")
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		if os.IsNotExist(err) {
			// Make the error clearer than the default
			return "", fmt.Errorf("file %v does not exist", absolute)
		}

		return "", err
	}

	return resolved, nil
}

func cReap(s *service, pid, status int, id, execid string, exitat time.Time) {
	s.ec <- Exit{
		timestamp: exitat,
		pid:       pid,
		status:    status,
		id:        id,
		execid:    execid,
	}
}
