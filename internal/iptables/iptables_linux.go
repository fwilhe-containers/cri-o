/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Vendored from https://git.k8s.io/kubernetes/pkg/util/iptables/iptables_linux.go

package iptables

import (
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/sys/unix"
	"k8s.io/apimachinery/pkg/util/wait"
)

type locker struct {
	lock16 *os.File
	lock14 *net.UnixListener
}

func (l *locker) Close() (err error) {
	if l.lock16 != nil {
		err = errors.Join(l.lock16.Close())
	}

	if l.lock14 != nil {
		err = errors.Join(err, l.lock14.Close())
	}

	return err
}

func grabIptablesLocks(lockfilePath14x, lockfilePath16x string) (iptablesLocker, error) {
	var err error

	var success bool

	l := &locker{}
	defer func(l *locker) {
		// Clean up immediately on failure
		if !success {
			l.Close()
		}
	}(l)

	// Grab both 1.6.x and 1.4.x-style locks; we don't know what the
	// iptables-restore version is if it doesn't support --wait, so we
	// can't assume which lock method it'll use.

	// Roughly duplicate iptables 1.6.x xtables_lock() function.
	l.lock16, err = os.OpenFile(lockfilePath16x, os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("failed to open iptables lock %s: %w", lockfilePath16x, err)
	}

	if err := wait.PollImmediate(200*time.Millisecond, 2*time.Second, func() (bool, error) { //nolint:staticcheck
		if err := grabIptablesFileLock(l.lock16); err != nil {
			return false, nil
		}

		return true, nil
	}); err != nil {
		return nil, fmt.Errorf("failed to acquire new iptables lock: %w", err)
	}

	// Roughly duplicate iptables 1.4.x xtables_lock() function.
	if err := wait.PollImmediate(200*time.Millisecond, 2*time.Second, func() (bool, error) { //nolint:staticcheck
		l.lock14, err = net.ListenUnix("unix", &net.UnixAddr{Name: lockfilePath14x, Net: "unix"})
		if err != nil {
			return false, nil
		}

		return true, nil
	}); err != nil {
		return nil, fmt.Errorf("failed to acquire old iptables lock: %w", err)
	}

	success = true

	return l, nil
}

func grabIptablesFileLock(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
}
