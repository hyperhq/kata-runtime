// Copyright (c) 2018 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package containerdshim

import (
	"github.com/containernetworking/plugins/pkg/ns"
	vc "github.com/kata-containers/runtime/virtcontainers"
)

func setupNetworkNamespace(config *vc.NetworkConfig) error {
	if config.NetNSPath != "" {
		return nil
	}

	n, err := ns.NewNS()
	if err != nil {
		return err
	}

	config.NetNSPath = n.Path()
	config.NetNsCreated = true

	return nil
}
