/*
Copyright © 2025 Dell Inc. or its subsidiaries. All Rights Reserved.

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

package nfs

import (
	"os/exec"
	"strings"

	log "github.com/sirupsen/logrus"
)

//go:generate mockgen -destination=mocks/executor.go -package=mocks . Executor
type Executor interface {
	ExecuteCommand(name string, args ...string) ([]byte, error)
}

type LocalExecutor struct{}

func (l *LocalExecutor) ExecuteCommand(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	return cmd.CombinedOutput()
}

// init_nfs_server uses systemctl to initialize an nfs server if necessary
func (cs *CsiNfsService) initializeNfsServer() error {
	log.Infof("checking status of nfs-server")
	// Check to see if nfs-server is active (exited ok)
	// Note: systemctl doesn't work when invoked from a container unless you ssh localhost before executing it.
	// You will get the message "Failed to connect to bus: No data available" because systemctl must think it's on the local host.
	restartNfsServer := false
	cmd := exec.Command("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-server")
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "Active: active") {
		log.Infof("nfs-server not active: %s", string(out))
		restartNfsServer = true
	}
	cmd = exec.Command("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-mountd")
	out, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "Active: active") {
		log.Infof("nfs-mountd not active: %s", string(out))
	}

	// Reinitialize the NFS server if necessary
	if !restartNfsServer {
		return nil
	}

	// First copy the nfs.conf file to /noderoot/etc/nfs.conf. This is a 2-step process.
	log.Infof("Configuring /etc/nfs.conf")
	cmd = exec.Command("cp", "/nfs.conf", "/noderoot/tmp/nfs.conf")
	out, err = cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Couldn't copy /nfs.conf to /noderoot/tmp/nfs.conf: %s", string(out))
		return err
	}
	cmd = exec.Command("chroot", "/noderoot", "cp", "/tmp/nfs.conf", "/etc/nfs.conf")
	out, err = cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Couldn't copy /tmp/nfs.conf to /etc/nfs.conf: %s", string(out))
		return err
	}

	// Now enable the nfs-server
	cmd = exec.Command("chroot", "/noderoot", "ssh", "localhost", "systemctl", "enable", "nfs-server")
	out, err = cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Couldn't enable nfs-server: %s", string(out))
		return err
	}

	// Now start the nfs-server
	cmd = exec.Command("chroot", "/noderoot", "ssh", "localhost", "systemctl", "start", "nfs-server")
	out, err = cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Couldn't start nfs-server: %s", string(out))
		return err
	}
	log.Infof("nfs-server start successful")

	// Recheck the status of nfs-server and nfs-mountd services"
	cmd = exec.Command("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-server")
	out, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "Active: active") {
		log.Infof("nfs-server not active: %s", string(out))
		return err
	}
	cmd = exec.Command("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-mountd")
	out, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "Active: active") {
		log.Infof("nfs-mountd not active: %s", string(out))
		return err
	}
	return nil
}
