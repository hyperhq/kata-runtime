// Copyright (c) 2018 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package containerd_shim

import (
	"context"
	"path"

	"github.com/containerd/containerd/mount"
	"github.com/kata-containers/runtime/pkg/katautils"
	vc "github.com/kata-containers/runtime/virtcontainers"
	"github.com/sirupsen/logrus"
)

func deleteContainer(ctx context.Context, s *service, c *container) error {

	status, err := vci.StatusContainer(ctx, s.sandbox.ID(), c.id)
	if err != nil {
		return err
	}
	if status.State.State != vc.StateStopped {
		_, err = vci.StopContainer(ctx, s.sandbox.ID(), c.id)
		if err != nil {
			return err
		}
	}

	_, err = vci.DeleteContainer(ctx, s.sandbox.ID(), c.id)
	if err != nil {
		return err
	}

	// Run post-stop OCI hooks.
	if err := katautils.PostStopHooks(ctx, *c.spec, s.sandbox.ID(), c.bundle); err != nil {
		return err
	}

	rootfs := path.Join(c.bundle, "rootfs")
	if err := mount.UnmountAll(rootfs, 0); err != nil {
		logrus.WithError(err).Warn("failed to cleanup rootfs mount")
	}

	delete(s.processes, c.pid)
	delete(s.containers, c.id)

	return nil
}
