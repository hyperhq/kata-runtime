// Copyright (c) 2018 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
"fmt"
"os"

"github.com/kata-containers/runtime/containerd-shim/kata"
"github.com/containerd/containerd/runtime/v2/shim"
)

func main() {
	if err := shim.Run(kata.New); err != nil {
		fmt.Fprintf(os.Stderr, "containerd-shim-kata-v2: %s\n", err)
		os.Exit(1)
	}
}

