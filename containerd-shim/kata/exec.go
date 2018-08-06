// Copyright (c) 2018 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package kata

import (
	"encoding/json"
	"fmt"
	"github.com/containerd/containerd/api/types/task"
	googleProtobuf "github.com/gogo/protobuf/types"
	vc "github.com/kata-containers/runtime/virtcontainers"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"strings"
	"time"
)

type Exec struct {
	id        string
	pid       uint32
	container *Container
	cmds      *vc.Cmd
	exitCode  int32
	tty       *Tty
	ttyio     *TtyIO
	status    task.Status

	exitIOch chan struct{}
	exitch   chan uint32

	exitTime time.Time
}

type Tty struct {
	stdin    string
	stdout   string
	stderr   string
	height   uint32
	width    uint32
	terminal bool
}

func getEnvs(envs []string) []vc.EnvVar {
	var vcEnvs = []vc.EnvVar{}
	var env vc.EnvVar

	for _, v := range envs {
		pair := strings.SplitN(v, "=", 2)

		if len(pair) == 2 {
			env = vc.EnvVar{Var: pair[0], Value: pair[1]}
		} else if len(pair) == 1 {
			env = vc.EnvVar{Var: pair[0], Value: ""}
		}

		vcEnvs = append(vcEnvs, env)
	}

	return vcEnvs
}

func newExec(c *Container, stdin, stdout, stderr string, terminal bool, jspec *googleProtobuf.Any) (*Exec, error) {
	var height uint32
	var width uint32

	// process exec request
	var spec specs.Process
	if err := json.Unmarshal(jspec.Value, &spec); err != nil {
		return nil, err
	}

	if spec.ConsoleSize != nil {
		height = uint32(spec.ConsoleSize.Height)
		width = uint32(spec.ConsoleSize.Width)
	}

	tty := &Tty{
		stdin:    stdin,
		stdout:   stdout,
		stderr:   stderr,
		height:   height,
		width:    width,
		terminal: terminal,
	}

	cmds := &vc.Cmd{
		Args:            spec.Args,
		Envs:            getEnvs(spec.Env),
		User:            fmt.Sprintf("%d", spec.User.UID),
		PrimaryGroup:    fmt.Sprintf("%d", spec.User.GID),
		WorkDir:         spec.Cwd,
		Interactive:     terminal,
		Detach:          !terminal,
		NoNewPrivileges: spec.NoNewPrivileges,
	}

	exec := &Exec{
		container: c,
		cmds:      cmds,
		tty:       tty,
		exitCode:  int32(255),
		exitIOch:  make(chan struct{}),
		exitch:    make(chan uint32, 1),
		status:    task.StatusCreated,
	}

	return exec, nil
}
