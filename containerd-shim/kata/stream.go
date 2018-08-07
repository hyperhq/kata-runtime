// Copyright (c) 2018 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
package kata

import (
	"io"
	"sync"
	"syscall"

	"context"
	"github.com/containerd/fifo"
)

type TtyIO struct {
	Stdin  io.ReadCloser
	Stdout io.Writer
	Stderr io.Writer
}

func (tty *TtyIO) Close() {

	if tty.Stdin != nil {
		tty.Stdin.Close()
	}
	cf := func(w io.Writer) {
		if w == nil {
			return
		}
		if c, ok := w.(io.WriteCloser); ok {
			c.Close()
		}
	}
	cf(tty.Stdout)
	cf(tty.Stderr)
}

func newTtyIO(ctx context.Context, stdin, stdout, stderr string, console bool) (*TtyIO, error) {
	var in io.ReadCloser
	var outw io.Writer
	var errw io.Writer
	var err error

	if stdin != "" {
		in, err = fifo.OpenFifo(ctx, stdin, syscall.O_RDONLY, 0)
		if err != nil {
			return nil, err
		}
	}

	outw, err = fifo.OpenFifo(ctx, stdout, syscall.O_WRONLY, 0)
	if err != nil {
		return nil, err
	}

	if !console {
		errw, err = fifo.OpenFifo(ctx, stderr, syscall.O_WRONLY, 0)
		if err != nil {
			return nil, err
		}
	}

	ttyIO := &TtyIO{
		Stdin:  in,
		Stdout: outw,
		Stderr: errw,
	}

	return ttyIO, nil
}

func ioCopy(exitch chan struct{}, tty *TtyIO, stdinPipe io.WriteCloser, stdoutPipe, stderrPipe io.Reader) {
	var wg sync.WaitGroup
	var closeOnce sync.Once

	if tty.Stdin != nil {
		wg.Add(1)
		go func() {
			p := bufPool.Get().(*[]byte)
			defer bufPool.Put(p)
			io.CopyBuffer(stdinPipe, tty.Stdin, *p)
			wg.Done()
		}()
	}

	if tty.Stdout != nil {
		wg.Add(1)

		go func() {
			p := bufPool.Get().(*[]byte)
			defer bufPool.Put(p)
			io.CopyBuffer(tty.Stdout, stdoutPipe, *p)
			wg.Done()
			closeOnce.Do(tty.Close)
		}()
	}

	if tty.Stderr != nil && stderrPipe != nil {
		wg.Add(1)
		go func() {
			p := bufPool.Get().(*[]byte)
			defer bufPool.Put(p)
			_, _ = io.CopyBuffer(tty.Stderr, stderrPipe, *p)
			wg.Done()
		}()
	}

	wg.Wait()
	closeOnce.Do(tty.Close)
	close(exitch)
}
