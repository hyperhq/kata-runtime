// Copyright (C) 2018 Red Hat, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package drivers

import (
	"encoding/hex"

	"github.com/kata-containers/runtime/virtcontainers/device/api"
	"github.com/kata-containers/runtime/virtcontainers/device/config"
	"github.com/kata-containers/runtime/virtcontainers/utils"
)

// VhostUserFSDevice is a virtio-fs vhost-user device
type VhostUserFSDevice struct {
	*GenericDevice
	config.VhostUserDeviceAttrs
}

// Device interface

func (device *VhostUserFSDevice) Attach(devReceiver api.DeviceReceiver) (err error) {
	skip, err := device.bumpAttachCount(true)
	if err != nil {
		return err
	}
	if skip {
		return nil
	}

	// generate a unique ID to be used for hypervisor commandline fields
	randBytes, err := utils.GenerateRandomBytes(8)
	if err != nil {
		return err
	}
	id := hex.EncodeToString(randBytes)

	device.DevID = id
	device.Type = device.DeviceType()

	defer func() {
		if err == nil {
			device.AttachCount = 1
		}
	}()
	return devReceiver.AppendDevice(device)
}

func (device *VhostUserFSDevice) Detach(devReceiver api.DeviceReceiver) error {
	skip, err := device.bumpAttachCount(true)
	if err != nil {
		return err
	}
	if skip {
		return nil
	}

	device.AttachCount = 0
	return nil
}

func (device *VhostUserFSDevice) DeviceType() config.DeviceType {
	return config.VhostUserFS
}

// GetDeviceInfo returns device information that the device is created based on
func (device *VhostUserFSDevice) GetDeviceInfo() interface{} {
       device.Type = device.DeviceType()
       return &device.VhostUserDeviceAttrs
}
