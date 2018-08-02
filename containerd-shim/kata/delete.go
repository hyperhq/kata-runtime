// Copyright (c) 2018 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package kata

import (
	"github.com/containerd/containerd/mount"
	vc "github.com/kata-containers/runtime/virtcontainers"
	"github.com/sirupsen/logrus"
	"path"
)

func deleteContainer(s *service, c *Container) error {

	status, err := s.sandbox.StatusContainer(c.id)
	if err != nil {
		return err
	}
	if status.State.State == vc.StateRunning {
		_, err = vci.StopContainer(s.sandbox.ID(), c.id)
		if err != nil {
			return err
		}
	}

	_, err = s.sandbox.DeleteContainer(c.id)
	if err != nil {
		return err
	}

	rootfs := path.Join(c.bundle, "rootfs")
	if err := mount.UnmountAll(rootfs, 0); err != nil {
		logrus.WithError(err).Warn("failed to cleanup rootfs mount")
	}

	if err := delContainerIDMapping(c.id); err != nil {
		return err
	}

	delete(s.processes, c.pid)
	delete(s.containers, c.id)

	return nil
}
