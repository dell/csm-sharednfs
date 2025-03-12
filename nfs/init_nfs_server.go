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
	"strings"

	log "github.com/sirupsen/logrus"
)

// init_nfs_server uses systemctl to initialize an nfs server if necessary
func (cs *CsiNfsService) initializeNfsServer() error {
	log.Infof("checking status of nfs-server")
	// Check to see if nfs-server is active (exited ok)
	// Note: systemctl doesn't work when invoked from a container unless you ssh localhost before executing it.
	// You will get the message "Failed to connect to bus: No data available" because systemctl must think it's on the local host.
	restartNfsServer := false
	out, err := cs.executor.ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-server")
	if err != nil || !strings.Contains(string(out), "Active: active") {
		log.Infof("nfs-server not active: %s", string(out))
		restartNfsServer = true
	}
	out, err = cs.executor.ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-mountd")
	if err != nil || !strings.Contains(string(out), "Active: active") {
		log.Infof("nfs-mountd not active: %s", string(out))
	}

	// Reinitialize the NFS server if necessary
	if !restartNfsServer {
		return nil
	}

	// First copy the nfs.conf file to /noderoot/etc/nfs.conf. This is a 2-step process.
	log.Infof("Configuring /etc/nfs.conf")
	out, err = cs.executor.ExecuteCommand("cp", "/nfs.conf", "/noderoot/tmp/nfs.conf")
	if err != nil {
		log.Errorf("Couldn't copy /nfs.conf to /noderoot/tmp/nfs.conf: %s", string(out))
		return err
	}
	out, err = cs.executor.ExecuteCommand("chroot", "/noderoot", "cp", "/tmp/nfs.conf", "/etc/nfs.conf")
	if err != nil {
		log.Errorf("Couldn't copy /tmp/nfs.conf to /etc/nfs.conf: %s", string(out))
		return err
	}

	// Now enable the nfs-server
	out, err = cs.executor.ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "enable", "nfs-server")
	if err != nil {
		log.Errorf("Couldn't enable nfs-server: %s", string(out))
		return err
	}

	// Now start the nfs-server
	out, err = cs.executor.ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "start", "nfs-server")
	if err != nil {
		log.Errorf("Couldn't start nfs-server: %s", string(out))
		return err
	}
	log.Infof("nfs-server start successful")

	// Recheck the status of nfs-server and nfs-mountd services"
	out, err = cs.executor.ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-server")
	if err != nil || !strings.Contains(string(out), "Active: active") {
		log.Infof("nfs-server not active: %s", string(out))
		return err
	}
	out, err = cs.executor.ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-mountd")
	if err != nil || !strings.Contains(string(out), "Active: active") {
		log.Infof("nfs-mountd not active: %s", string(out))
		return err
	}
	return nil
}
