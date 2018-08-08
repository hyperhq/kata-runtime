// Copyright (c) 2018 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package kata

import (
	"context"
	"github.com/containerd/containerd/api/types/task"
	vc "github.com/kata-containers/runtime/virtcontainers"
	"github.com/kata-containers/runtime/virtcontainers/pkg/oci"
)

func startContainer(ctx context.Context, s *service, c *Container) error {
	//start a container
	// Checks the MUST and MUST NOT from OCI runtime specification
	status, sandboxID, err := getExistingContainerInfo(c.id)
	if err != nil {
		return err
	}

	containerType, err := oci.GetContainerType(status.Annotations)
	if err != nil {
		return err
	}
	if containerType.IsSandbox() {
		_, err := vc.StartSandbox(s.sandbox.ID())
		if err != nil {
			return err
		}
	} else {
		_, err = vci.StartContainer(sandboxID, c.id)
		if err != nil {
			return err
		}
	}

	c.status = task.StatusRunning

	stdin, stdout, stderr, err := s.sandbox.IOStream(c.id, c.id)
	if err != nil {
		return err
	}
	tty, err := newTtyIO(ctx, c.stdin, c.stdout, c.stderr, c.terminal)
	c.ttyio = tty

	go ioCopy(c.exitIOch, tty, stdin, stdout, stderr)

	go wait(s, c, "")

	return nil
}
