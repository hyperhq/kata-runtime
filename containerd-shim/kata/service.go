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

	"github.com/containerd/cgroups"
	eventstypes "github.com/containerd/containerd/api/events"
	"github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/events"
	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/namespaces"
	cdruntime "github.com/containerd/containerd/runtime"
	cdshim "github.com/containerd/containerd/runtime/v2/shim"
	taskAPI "github.com/containerd/containerd/runtime/v2/task"
	"github.com/containerd/typeurl"
	vc "github.com/kata-containers/runtime/virtcontainers"
	"github.com/kata-containers/runtime/virtcontainers/pkg/oci"

	ptypes "github.com/gogo/protobuf/types"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"path/filepath"
)

const bufferSize = 32

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
		ec:         make(chan Exit, bufferSize),
		ep:         ep,
	}

	go s.processExits()

	go s.forward(publisher)

	vci.SetLogger(logrus.WithField("ID", id))

	return s, nil
}

type Exit struct {
	id        string
	execid    string
	pid       int
	status    int
	timestamp time.Time
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

	//TODO: replace runcC.Exit with a general Exit in shim module
	ec chan Exit

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
	container := newContainer(s, r, pid, c)
	container.status = task.StatusCreated

	s.containers[r.ID] = container
	s.processes[pid] = c.Process()

	return &taskAPI.CreateTaskResponse{
		Pid: pid,
	}, nil
}

// Start a process
func (s *service) Start(ctx context.Context, r *taskAPI.StartRequest) (*taskAPI.StartResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, err := s.getContainer(r.ID)
	if err != nil {
		return nil, err
	}

	//start a container
	if r.ExecID == "" {
		err = startContainer(ctx, s, c)
		if err != nil {
			return nil, errdefs.ToGRPC(err)
		}

		return &taskAPI.StartResponse{
			Pid: c.pid,
		}, nil
	} else { //start an exec
		execs, err := startExec(ctx, s, r.ID, r.ExecID)
		if err != nil {
			return nil, errdefs.ToGRPC(err)
		}

		return &taskAPI.StartResponse{
			Pid: execs.pid,
		}, nil
	}
}

// Delete the initial process and container
func (s *service) Delete(ctx context.Context, r *taskAPI.DeleteRequest) (*taskAPI.DeleteResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, err := s.getContainer(r.ID)
	if err != nil {
		return nil, err
	}

	if r.ExecID == "" {
		err = deleteContainer(s, c)
		if err != nil {
			return nil, err
		}

		return &taskAPI.DeleteResponse{
			ExitStatus: c.exit,
			ExitedAt:   c.time,
			Pid:        c.pid,
		}, nil
	} else {
		execs, err := c.getExec(r.ExecID)
		if err != nil {
			return nil, err
		}

		delete(s.processes, execs.pid)
		delete(c.execs, r.ExecID)

		return &taskAPI.DeleteResponse{
			ExitStatus: uint32(execs.exitCode),
			ExitedAt:   execs.exitTime,
			Pid:        execs.pid,
		}, nil
	}
}

// Exec an additional process inside the container
func (s *service) Exec(ctx context.Context, r *taskAPI.ExecProcessRequest) (*ptypes.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, err := s.getContainer(r.ID)
	if err != nil {
		return nil, err
	}

	if execs := c.execs[r.ExecID]; execs != nil {
		return nil, errdefs.ToGRPCf(errdefs.ErrAlreadyExists, "id %s", r.ExecID)
	}

	execs, err := newExec(c, r.Stdin, r.Stdout, r.Stderr, r.Terminal, r.Spec)
	if err != nil {
		return nil, errdefs.ToGRPC(err)
	}

	c.execs[r.ExecID] = execs

	return empty, nil
}

// ResizePty of a process
func (s *service) ResizePty(ctx context.Context, r *taskAPI.ResizePtyRequest) (*ptypes.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, err := s.getContainer(r.ID)
	if err != nil {
		return nil, err
	}

	processID := c.id
	if r.ExecID != "" {
		execs, err := c.getExec(r.ExecID)
		if err != nil {
			return nil, err
		}
		execs.tty.height = r.Height
		execs.tty.width = r.Width

		return empty, nil

	}
	err = s.sandbox.WinsizeProcess(c.id, processID, r.Height, r.Width)
	if err != nil {
		return nil, err
	}

	return empty, err
}

// State returns runtime state information for a process
func (s *service) State(ctx context.Context, r *taskAPI.StateRequest) (*taskAPI.StateResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, err := s.getContainer(r.ID)
	if err != nil {
		return nil, err
	}

	if r.ExecID == "" {
		return &taskAPI.StateResponse{
			ID:         c.id,
			Bundle:     c.bundle,
			Pid:        c.pid,
			Status:     c.status,
			Stdin:      c.stdin,
			Stdout:     c.stdout,
			Stderr:     c.stderr,
			Terminal:   c.terminal,
			ExitStatus: c.exit,
		}, nil
	} else {
		execs, err := c.getExec(r.ExecID)
		if err != nil {
			return nil, err
		}
		return &taskAPI.StateResponse{
			ID:         execs.id,
			Bundle:     c.bundle,
			Pid:        execs.pid,
			Status:     execs.status,
			Stdin:      execs.tty.stdin,
			Stdout:     execs.tty.stdout,
			Stderr:     execs.tty.stderr,
			Terminal:   execs.tty.terminal,
			ExitStatus: uint32(execs.exitCode),
		}, nil
	}
}

// Pause the container
func (s *service) Pause(ctx context.Context, r *taskAPI.PauseRequest) (*ptypes.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, err := s.getContainer(r.ID)
	if err != nil {
		return nil, err
	}

	c.status = task.StatusPausing

	err = vc.PauseContainer(r.ID, c.id)
	if err == nil {
		c.status = task.StatusPaused
	} else {
		c.status = task.StatusUnknown
	}

	return nil, err
}

// Resume the container
func (s *service) Resume(ctx context.Context, r *taskAPI.ResumeRequest) (*ptypes.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, err := s.getContainer(r.ID)
	if err != nil {
		return nil, err
	}

	err = vc.ResumeContainer(r.ID, c.id)
	if err == nil {
		c.status = task.StatusRunning
	} else {
		c.status = task.StatusUnknown
	}

	return nil, err
}

// Kill a process with the provided signal
func (s *service) Kill(ctx context.Context, r *taskAPI.KillRequest) (*ptypes.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, err := s.getContainer(r.ID)
	if err != nil {
		return nil, err
	}

	processID := c.id
	if r.ExecID != "" {
		execs, err := c.getExec(r.ExecID)
		if err != nil {
			return nil, err
		}
		processID = execs.id
	}

	err = s.sandbox.SignalProcess(c.id, processID, syscall.Signal(r.Signal), r.All)
	if err == nil {
		c.status = task.StatusStopped
	} else {
		c.status = task.StatusUnknown
	}

	return nil, err
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
	s.mu.Lock()
	if len(s.containers) == 0 {
		defer os.Exit(0)

		_, err := vci.StopSandbox(s.sandbox.ID())
		if err != nil {
			s.mu.Unlock()
			return empty, err
		}

		_, err = vci.DeleteSandbox(s.sandbox.ID())
		if err != nil {
			s.mu.Unlock()
			return empty, err
		}
	}
	defer s.mu.Unlock()

	return empty, nil
}

func (s *service) Stats(ctx context.Context, r *taskAPI.StatsRequest) (*taskAPI.StatsResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c, err := s.getContainer(r.ID)
	if err != nil {
		return nil, err
	}

	stats, err := s.sandbox.StatsContainer(c.id)
	if err != nil {
		return nil, err
	}

	cgStats := stats.CgroupStats
	metrics := &cgroups.Metrics{
		Memory: &cgroups.MemoryStat{
			Usage: &cgroups.MemoryEntry{
				Limit: cgStats.MemoryStats.Usage.Limit,
				Usage: cgStats.MemoryStats.Usage.Usage,
			},
		},
	}

	data, err := typeurl.MarshalAny(metrics)
	if err != nil {
		return nil, err
	}

	return &taskAPI.StatsResponse{
		Stats: data,
	}, nil
}

// Update a running container
func (s *service) Update(ctx context.Context, r *taskAPI.UpdateTaskRequest) (*ptypes.Empty, error) {
	return nil, errdefs.ToGRPCf(errdefs.ErrNotImplemented, "service Update")
}

// Wait for a process to exit
func (s *service) Wait(ctx context.Context, r *taskAPI.WaitRequest) (*taskAPI.WaitResponse, error) {
	s.mu.Lock()
	c, err := s.getContainer(r.ID)
	s.mu.Unlock()

	if err != nil {
		return &taskAPI.WaitResponse{
			ExitStatus: uint32(0),
		}, err
	}

	ret, err := wait(s, c, r.ExecID)

	return &taskAPI.WaitResponse{
		ExitStatus: uint32(ret),
	}, err
}

func (s *service) processExits() {
	for e := range s.ec {
		s.checkProcesses(e)
	}
}

func (s *service) checkProcesses(e Exit) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := e.execid
	if id == "" {
		id = e.id
	}
	s.events <- &eventstypes.TaskExit{
		ContainerID: e.id,
		ID:          id,
		Pid:         uint32(e.pid),
		ExitStatus:  uint32(e.status),
		ExitedAt:    e.timestamp,
	}
	return
}

func (s *service) getContainer(id string) (*Container, error) {
	c := s.containers[id]

	if c == nil {
		return nil, errdefs.ToGRPCf(errdefs.ErrNotFound, "container does not exist %s", id)
	}

	return c, nil
}
