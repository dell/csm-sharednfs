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
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
)

func (cs *CsiNfsService) updateKnownHosts() error {
	// Run ssh-keyscan
	cmd, err := cs.executor.ExecuteCommand("chroot", "/noderoot", "ssh-keyscan", "-t", "rsa,ecdsa,ed25519", "localhost")
	// Run ssh-keyscan for multiple key types
	if err != nil {
		return fmt.Errorf("failed to run ssh-keyscan: %v", err)
	}
	// Extract the keys from the ssh-keyscan output
	output := strings.TrimSpace(string(cmd))
	lines := strings.Split(output, "\n")
	keys := make(map[string]string)
	for _, line := range lines {
		if strings.Contains(line, "localhost") && !strings.HasPrefix(line, "#") {
			parts := strings.SplitN(line, " ", 3)
			if len(parts) == 3 {
				keys[parts[1]] = parts[2] // Store key type and key
			}
		}
	}

	// Read the known_hosts file
	knownHostsPath := "/noderoot/root/.ssh/known_hosts"
	knownHosts, err := os.ReadFile(knownHostsPath)
	if err != nil {
		return fmt.Errorf("failed to read known_hosts: %v", err)
	}

	// Check if the keys already exist and update them if necessary
	scanner := bufio.NewScanner(bytes.NewReader(knownHosts))
	var updatedHosts bytes.Buffer
	existingKeys := make(map[string]bool)
	for scanner.Scan() {
		line := scanner.Text()
		// Ignore lines that are comments and end with \n
		if strings.HasPrefix(line, "#") && strings.HasSuffix(line, "\n") {
			continue
		}
		updated := false
		for keyType, key := range keys {
			if strings.Contains(line, "localhost") && strings.Contains(line, keyType) {
				// Replace the existing localhost entry with the new key
				updatedHosts.WriteString("localhost " + keyType + " " + key + "\n")
				updated = true
				existingKeys[keyType] = true // Mark this key type as updated
				break
			}
		}
		if !updated {
			updatedHosts.WriteString(line + "\n")
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan known_hosts: %v", err)
	}

	// Append any new keys that were not found in the existing known_hosts file
	for keyType, key := range keys {
		if !existingKeys[keyType] {
			updatedHosts.WriteString("localhost " + keyType + " " + key + "\n")
		}
	}

	// Write the updated known_hosts file
	if err := os.WriteFile(knownHostsPath, updatedHosts.Bytes(), 0o644); err != nil {
		return fmt.Errorf("failed to write known_hosts: %v", err)
	}

	return nil
}

// init_nfs_server uses systemctl to initialize an nfs server if necessary
func (cs *CsiNfsService) initializeNfsServer() error {
	log.Infof("checking status of nfs-server")
	err := cs.updateKnownHosts()
	if err != nil {
		log.Warnf("Could not update known hosts : %v. proceeding anyway", err)
	}
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
		log.Infof("nfs-server and nfs-mountd are active")
		return nil
	}

	log.Infof("nfs-server and nfs-mountd are not active, attempting to configure them")

	// First copy the nfs.conf file to /noderoot/etc/nfs.conf. This is a 2-step process.
	// log.Infof("Configuring /etc/nfs.conf")
	// out, err = cs.executor.ExecuteCommand("cp", "/etc/nfs.conf", "/noderoot/tmp/nfs.conf")
	// if err != nil {
	// 	log.Errorf("Couldn't copy /nfs.conf to /noderoot/tmp/nfs.conf: %s", string(out))
	// 	return err
	// }
	// out, err = cs.executor.ExecuteCommand("chroot", "/noderoot", "cp", "/tmp/nfs.conf", "/etc/nfs.conf")
	// if err != nil {
	// 	log.Errorf("Couldn't copy /tmp/nfs.conf to /etc/nfs.conf: %s", string(out))
	// 	return err
	// }

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

	log.Infof("nfs-server and nfs-mountd are now active")
	return nil
}
