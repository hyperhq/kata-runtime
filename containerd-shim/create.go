// Copyright (c) 2014,2015,2016 Docker, Inc.
// Copyright (c) 2017 Intel Corporation
// Copyright (c) 2018 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package containerd_shim

import (
	"context"
	"fmt"

	vc "github.com/kata-containers/runtime/virtcontainers"
	"github.com/kata-containers/runtime/virtcontainers/pkg/oci"

	taskAPI "github.com/containerd/containerd/runtime/v2/task"

	"github.com/kata-containers/runtime/pkg/katautils"
	"github.com/opencontainers/runtime-spec/specs-go"
)

func create(ctx context.Context, s *service, r *taskAPI.CreateTaskRequest, pid uint32, netns string,
	runtimeConfig *oci.RuntimeConfig) (*container, error) {

	detach := !r.Terminal

	// Checks the MUST and MUST NOT from OCI runtime specification
	bundlePath, err := validCreateParams(r.ID, r.Bundle)
	if err != nil {
		return nil, err
	}

	ociSpec, err := oci.ParseConfigJSON(bundlePath)
	if err != nil {
		return nil, err
	}

	containerType, err := ociSpec.ContainerType()
	if err != nil {
		return nil, err
	}

	//In the sandbox, the containers will only
	//use the mnt space to separate the rootfs,
	//and to share the other namesapces with host
	//in the sandbox, thus remove those namespaces
	//from ocispec except networkNamespace, since
	//it has been ignored by kata-agent in sandbox.

	for _, ns := range []specs.LinuxNamespaceType{
		specs.UserNamespace,
		specs.UTSNamespace,
		specs.IPCNamespace,
		specs.PIDNamespace,
		specs.CgroupNamespace,
	} {
		removeNameSpace(&ociSpec, ns)
	}

	//set the network namespace path
	//this set will be applied to sandbox's
	//network config and hasn't nothing to
	//do with containers in the sandbox since
	//networkNamesapce hasn't been ignored by
	//kata-agent in sandbox.
	for _, n := range ociSpec.Linux.Namespaces {
		if n.Type != specs.NetworkNamespace {
			continue
		}

		if n.Path == "" {
			n.Path = netns
		}
	}

	katautils.HandleFactory(ctx, vci, runtimeConfig)

	disableOutput := noNeedForOutput(detach, ociSpec.Process.Terminal)

	var c vc.VCContainer
	switch containerType {
	case vc.PodSandbox:
		if s.sandbox != nil {
			return nil, fmt.Errorf("cannot create another sandbox in sandbox: %s", s.sandbox.ID())
		}

		c, err = createSandbox(ctx, ociSpec, *runtimeConfig, r.ID, bundlePath, disableOutput)
		if err != nil {
			return nil, err
		}
		s.sandbox = c.Sandbox()

	case vc.PodContainer:
		if s.sandbox == nil {
			return nil, fmt.Errorf("BUG: Cannot start the container, since the sandbox hasn't been created")
		}

		err = createContainer(ctx, s.sandbox, ociSpec, r.ID, bundlePath, disableOutput)
		if err != nil {
			return nil, err
		}
	}

	container, err := newContainer(s, r, pid, containerType, &ociSpec)
	if err != nil {
		return nil, err
	}

	return container, nil
}

func createSandbox(ctx context.Context, ociSpec oci.CompatOCISpec, runtimeConfig oci.RuntimeConfig,
	containerID, bundlePath string, disableOutput bool) (vc.VCContainer, error) {

	err := katautils.SetKernelParams(containerID, &runtimeConfig)
	if err != nil {
		return nil, err
	}

	sandboxConfig, err := oci.SandboxConfig(ociSpec, runtimeConfig, bundlePath, containerID, "", disableOutput, false)
	if err != nil {
		return nil, err
	}

	sandboxConfig.Stateful = true

	//setup the networkNamespace if it hasn't been created, such as the using the CNM
	if err = setupNetworkNamespace(&sandboxConfig.NetworkConfig); err != nil {
		return nil, err
	}

	// Run pre-start OCI hooks.
	err = katautils.EnterNetNS(sandboxConfig.NetworkConfig.NetNSPath, func() error {
		return katautils.PreStartHooks(ctx, ociSpec, containerID, bundlePath)
	})
	if err != nil {
		return nil, err
	}

	sandbox, err := vci.CreateSandbox(ctx, sandboxConfig)
	if err != nil {
		return nil, err
	}

	containers := sandbox.GetAllContainers()
	if len(containers) != 1 {
		return nil, fmt.Errorf("BUG: Container list from sandbox is wrong, expecting only one container, found %d containers", len(containers))
	}

	return containers[0], nil
}

func createContainer(ctx context.Context, sandbox vc.VCSandbox, ociSpec oci.CompatOCISpec, containerID, bundlePath string,
	disableOutput bool) error {

	ociSpec = katautils.SetEphemeralStorageType(ociSpec)

	contConfig, err := oci.ContainerConfig(ociSpec, bundlePath, containerID, "", disableOutput)
	if err != nil {
		return err
	}

	// Run pre-start OCI hooks.
	err = katautils.EnterNetNS(sandbox.GetNetNs(), func() error {
		return katautils.PreStartHooks(ctx, ociSpec, containerID, bundlePath)
	})
	if err != nil {
		return err
	}

	_, err = sandbox.CreateContainer(contConfig)
	if err != nil {
		return err
	}

	return nil
}
