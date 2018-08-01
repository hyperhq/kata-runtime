// Copyright (c) 2018 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
package kata

import (
	"syscall"

	vc "github.com/kata-containers/runtime/virtcontainers"
)

type sandboxOp func(sb vc.VCSandbox) error

func pause(sandbox vc.VCSandbox, container vc.VCContainer, containerType vc.ContainerType) error {
	var err error
	
	switch containerType {
	case vc.PodSandbox:
		err := sandbox.Pause()
		if err != nil {
			return err
		}

	case vc.PodContainer:
		err = vc.PauseContainer(sandbox.ID(), container.ID())
		if err != nil {
			return err
		}
	}

	return err
}

func resume(sandbox vc.VCSandbox, container vc.VCContainer, containerType vc.ContainerType) error {
	var err error
	
	switch containerType {
	case vc.PodSandbox:
		err := sandbox.Resume()
		if err != nil {
			return err
		}

	case vc.PodContainer:
		err = vc.ResumeContainer(sandbox.ID(), container.ID())
		if err != nil {
			return err
		}
	}

	return err
}

func kill(sandbox vc.VCSandbox, container vc.VCContainer, execID string, signal uint32, all bool) error {
	err := sandbox.SignalProcess(container.ID(), execID, syscall.Signal(signal), all)
	if err != nil {
		return err
	}

	return err
}
