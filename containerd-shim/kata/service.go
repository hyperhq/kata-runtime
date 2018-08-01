// Copyright (c) 2018 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
package kata

import (
	"context"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	eventstypes "github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/events"
	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/namespaces"
	cdruntime "github.com/containerd/containerd/runtime"
	cdshim "github.com/containerd/containerd/runtime/v2/shim"
	taskAPI "github.com/containerd/containerd/runtime/v2/task"
	runcC "github.com/containerd/go-runc"
	vc "github.com/kata-containers/runtime/virtcontainers"
	"github.com/kata-containers/runtime/virtcontainers/pkg/oci"

	ptypes "github.com/gogo/protobuf/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"path/filepath"
)

var (
	empty   = &ptypes.Empty{}
	bufPool = sync.Pool{
		New: func() interface{} {
			buffer := make([]byte, 32<<10)
			return &buffer
		},
	}
)

var _ taskAPI.TaskService = (taskAPI.TaskService)(&service{})

//The init pid that passed to containerd. This pid is just used to
//map the unique process in sandbox.
var pidCount uint32 = 5

// concrete virtcontainer implementation
var vci vc.VC = &vc.VCImpl{}

// New returns a new shim service that can be used via GRPC
func New(ctx context.Context, id string, publisher events.Publisher) (cdshim.Shim, error) {
	ep, err := newOOMEpoller(publisher)
	if err != nil {
		return nil, err
	}
	go ep.run(ctx)

	runtimeConfig, err := loadConfiguration()

	if err != nil {
		return nil, err
	}

	s := &service{
		id:         id,
		context:    ctx,
		config:     runtimeConfig,
		containers: make(map[string]*Container),
		processes:  make(map[uint32]vc.Process),
		events:     make(chan interface{}, 128),
		ec:         cdshim.Default.Subscribe(),
		ep:         ep,
	}

	go s.forward(publisher)

	vci.SetLogger(logrus.WithField("ID", id))

	return s, nil
}

// service is the shim implementation of a remote shim over GRPC
type service struct {
	mu sync.Mutex

	context    context.Context
	sandbox    vc.VCSandbox
	containers map[string]*Container
	processes  map[uint32]vc.Process
	config     *oci.RuntimeConfig
	events     chan interface{}

	//When the sandbox was created, it will be closed
	//to notify other goroutines
	completed chan struct{}

	//TODO: replace runcC.Exit with a general Exit in shim module
	ec chan runcC.Exit

	ep *epoller

	id string
}

//get a unique pid in this sandbox
func (s *service) pid() uint32 {
	for true {
		_, ok := s.processes[pidCount]
		if !ok {
			break
		} else {
			pidCount += 1
			//if it overflows, recount from 5
			if pidCount < 5 {
				pidCount = 5
			}
		}
	}
	return pidCount
}

func newCommand(ctx context.Context, containerdBinary, containerdAddress string) (*exec.Cmd, error) {
	ns, err := namespaces.NamespaceRequired(ctx)
	if err != nil {
		return nil, err
	}
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	args := []string{
		"-namespace", ns,
		"-address", containerdAddress,
		"-publish-binary", containerdBinary,
	}
	cmd := exec.Command(self, args...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "GOMAXPROCS=2")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	return cmd, nil
}

func (s *service) StartShim(ctx context.Context, id, containerdBinary, containerdAddress string) (string, error) {
	cmd, err := newCommand(ctx, containerdBinary, containerdAddress)
	if err != nil {
		return "", err
	}
	address, err := cdshim.SocketAddress(ctx, id)
	if err != nil {
		return "", err
	}
	socket, err := cdshim.NewSocket(address)
	if err != nil {
		return "", err
	}
	defer socket.Close()
	f, err := socket.File()
	if err != nil {
		return "", err
	}
	defer f.Close()

	cmd.ExtraFiles = append(cmd.ExtraFiles, f)

	if err := cmd.Start(); err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			cmd.Process.Kill()
		}
	}()

	// make sure to wait after start
	go cmd.Wait()
	if err := cdshim.WritePidFile("shim.pid", cmd.Process.Pid); err != nil {
		return "", err
	}
	if err := cdshim.WriteAddress("address", address); err != nil {
		return "", err
	}
	if err := cdshim.SetScore(cmd.Process.Pid); err != nil {
		return "", errors.Wrap(err, "failed to set OOM Score on shim")
	}
	return address, nil
}

func (s *service) forward(publisher events.Publisher) {
	for e := range s.events {
		if err := publisher.Publish(s.context, getTopic(s.context, e), e); err != nil {
			logrus.WithError(err).Error("post event")
		}
	}
}

func getTopic(ctx context.Context, e interface{}) string {
	switch e.(type) {
	case *eventstypes.TaskCreate:
		return cdruntime.TaskCreateEventTopic
	case *eventstypes.TaskStart:
		return cdruntime.TaskStartEventTopic
	case *eventstypes.TaskOOM:
		return cdruntime.TaskOOMEventTopic
	case *eventstypes.TaskExit:
		return cdruntime.TaskExitEventTopic
	case *eventstypes.TaskDelete:
		return cdruntime.TaskDeleteEventTopic
	case *eventstypes.TaskExecAdded:
		return cdruntime.TaskExecAddedEventTopic
	case *eventstypes.TaskExecStarted:
		return cdruntime.TaskExecStartedEventTopic
	case *eventstypes.TaskPaused:
		return cdruntime.TaskPausedEventTopic
	case *eventstypes.TaskResumed:
		return cdruntime.TaskResumedEventTopic
	case *eventstypes.TaskCheckpointed:
		return cdruntime.TaskCheckpointedEventTopic
	default:
		logrus.Warnf("no topic for type %#v", e)
	}
	return cdruntime.TaskUnknownTopic
}

func (s *service) Cleanup(ctx context.Context) (*taskAPI.DeleteResponse, error) {
	return &taskAPI.DeleteResponse{
		ExitedAt:   time.Now(),
		ExitStatus: 128 + uint32(unix.SIGKILL),
	}, nil
}

// Create a new sandbox or container with the underlying OCI runtime
func (s *service) Create(ctx context.Context, r *taskAPI.CreateTaskRequest) (_ *taskAPI.CreateTaskResponse, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rootfs := filepath.Join(r.Bundle, "rootfs")
	defer func() {
		if err != nil {
			if err2 := mount.UnmountAll(rootfs, 0); err2 != nil {
				logrus.WithError(err2).Warn("failed to cleanup rootfs mount")
			}
		}
	}()
	for _, rm := range r.Rootfs {
		m := &mount.Mount{
			Type:    rm.Type,
			Source:  rm.Source,
			Options: rm.Options,
		}
		if err := m.Mount(rootfs); err != nil {
			return nil, errors.Wrapf(err, "failed to mount rootfs component %v", m)
		}
	}

	c, err := create(s, r.ID, r.Bundle, !r.Terminal, s.config)
	if err != nil {
		return nil, err
	}

	pid := s.pid()
	s.containers[r.ID] = newContainer(s, r, pid, c)
	s.processes[pid] = c.Process()

	return &taskAPI.CreateTaskResponse{
		Pid: pid,
	}, nil
}

// Start a process
func (s *service) Start(ctx context.Context, r *taskAPI.StartRequest) (*taskAPI.StartResponse, error) {

	c, ok := s.containers[r.ID]
	if !ok {
		return nil, errdefs.ToGRPCf(errdefs.ErrNotFound, "process does not exist %s", r.ID)
	}

	//start a sandbox or container, instead of an exec
	if r.ExecID == "" {

		_, err := start(s, r.ID, r.ExecID)
		if err != nil {
			return nil, errdefs.ToGRPC(err)
		}

		stdin, stdout, stderr, err := s.sandbox.IOStream(s.sandbox.ID(), c.id)
		if err != nil {
			return nil, err
		}
		tty, err := newTtyIO(ctx, c.stdin, c.stdout, c.stderr, c.terminal)

		go ioCopy(tty, stdin, stdout, stderr)

		return &taskAPI.StartResponse{
			Pid: c.pid,
		}, nil

	}
	return nil, errdefs.ErrNotImplemented
}

// Delete the initial process and container
func (s *service) Delete(ctx context.Context, r *taskAPI.DeleteRequest) (*taskAPI.DeleteResponse, error) {
	return nil, errdefs.ToGRPCf(errdefs.ErrNotImplemented, "service Delete id=%s", r.ID)
}

// Exec an additional process inside the container
func (s *service) Exec(ctx context.Context, r *taskAPI.ExecProcessRequest) (*ptypes.Empty, error) {
	return nil, errdefs.ToGRPCf(errdefs.ErrNotImplemented, "service Exec")
}

// ResizePty of a process
func (s *service) ResizePty(ctx context.Context, r *taskAPI.ResizePtyRequest) (*ptypes.Empty, error) {
//	return nil, errdefs.ToGRPCf(errdefs.ErrNotImplemented, "service ResizePty")
	return  empty, nil
}

// State returns runtime state information for a process
func (s *service) State(ctx context.Context, r *taskAPI.StateRequest) (*taskAPI.StateResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	c, _ := s.containers[r.ID]

	vcstatus, _ := s.sandbox.StatusContainer(r.ID)
	
	status := task.StatusUnknown
	switch vcstatus.State.State {
	case "ready":
		status = task.StatusCreated
	case "running":
		status = task.StatusRunning
	case "stopped":
		status = task.StatusStopped
	case "paused":
		status = task.StatusPaused
	}
	return &taskAPI.StateResponse{
		ID:         c.id,
		Bundle:     c.bundle,
		Pid:        c.pid,
		Status:     status,
		Stdin:      c.stdin,
		Stdout:     c.stdout,
		Stderr:     c.stderr,
		Terminal:   c.terminal,
		ExitStatus: uint32(0),
	}, nil
}

// Pause the container
func (s *service) Pause(ctx context.Context, r *taskAPI.PauseRequest) (*ptypes.Empty, error) {
	c, ok := s.containers[r.ID]
	if !ok {
		return nil, errdefs.ToGRPCf(errdefs.ErrNotFound, "container does not exist %s", r.ID)
	}

	containerType, err := oci.GetContainerType(s.containers[r.ID].container.GetAnnotations())
	if err != nil {
		return nil, err
	}

	return nil, pause(s.sandbox, c.container, containerType)
}

// Resume the container
func (s *service) Resume(ctx context.Context, r *taskAPI.ResumeRequest) (*ptypes.Empty, error) {

	c, ok := s.containers[r.ID]
	if !ok {
		return nil, errdefs.ToGRPCf(errdefs.ErrNotFound, "container does not exist %s", r.ID)
	}

	containerType, err := oci.GetContainerType(s.containers[r.ID].container.GetAnnotations())
	if err != nil {
		return nil, err
	}

	return nil, resume(s.sandbox, c.container, containerType)
}

// Kill a process with the provided signal
func (s *service) Kill(ctx context.Context, r *taskAPI.KillRequest) (*ptypes.Empty, error) {
	c, ok := s.containers[r.ID]
	if !ok {
		return nil, errdefs.ToGRPCf(errdefs.ErrNotFound, "container does not exist %s", r.ID)
	}
	
	return nil, kill(s.sandbox, c.container, r.ExecID, r.Signal, r.All)
}

// Pids returns all pids inside the container
func (s *service) Pids(ctx context.Context, r *taskAPI.PidsRequest) (*taskAPI.PidsResponse, error) {
	return nil, errdefs.ToGRPCf(errdefs.ErrNotImplemented, "service Pids")
}

// CloseIO of a process
func (s *service) CloseIO(ctx context.Context, r *taskAPI.CloseIORequest) (*ptypes.Empty, error) {
	return nil, errdefs.ToGRPCf(errdefs.ErrNotImplemented, "service CloseIO")
}

// Checkpoint the container
func (s *service) Checkpoint(ctx context.Context, r *taskAPI.CheckpointTaskRequest) (*ptypes.Empty, error) {
	return nil, errdefs.ToGRPCf(errdefs.ErrNotImplemented, "service Checkpoint")
}

// Connect returns shim information such as the shim's pid
func (s *service) Connect(ctx context.Context, r *taskAPI.ConnectRequest) (*taskAPI.ConnectResponse, error) {
	return nil, errdefs.ToGRPCf(errdefs.ErrNotImplemented, "service Connect")
}

func (s *service) Shutdown(ctx context.Context, r *taskAPI.ShutdownRequest) (*ptypes.Empty, error) {
	return nil, errdefs.ToGRPCf(errdefs.ErrNotImplemented, "service Shutdown")
}

func (s *service) Stats(ctx context.Context, r *taskAPI.StatsRequest) (*taskAPI.StatsResponse, error) {
	return nil, errdefs.ToGRPCf(errdefs.ErrNotImplemented, "service Stats")
}

// Update a running container
func (s *service) Update(ctx context.Context, r *taskAPI.UpdateTaskRequest) (*ptypes.Empty, error) {
	return nil, errdefs.ToGRPCf(errdefs.ErrNotImplemented, "service Update")
}

// Wait for a process to exit
func (s *service) Wait(ctx context.Context, r *taskAPI.WaitRequest) (*taskAPI.WaitResponse, error) {
	ret, err := s.sandbox.WaitProcess(r.ID, r.ID)
	return &taskAPI.WaitResponse{
		ExitStatus: uint32(ret),
	}, err
}
