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
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	log "github.com/sirupsen/logrus"
)

var (
	defaultTimeout       = 10 * time.Second
	nodeStageRetryWait   = defaultTimeout
	nodeStageTimeout     = 15 * time.Second
	nodeUnpublishTimeout = defaultTimeout
)

func (ns *CsiNfsService) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	ns.LockPV(req.VolumeId, req.VolumeId, false)
	defer ns.UnlockPV(req.VolumeId)
	resp := &csi.NodeStageVolumeResponse{}
	var err error
	retries := 0
	startTime := time.Now()
	defer func() {
		log.Infof("NodeStageVolume %s completed in %s %d retries error: %v", req.VolumeId, time.Since(startTime), retries, err)
	}()
	for retries := range ns.failureRetries {
		resp, err = ns.nodeStageVolume(ctx, req)
		if err == nil {
			return resp, err
		}
		if !strings.Contains(err.Error(), "timeout") {
			return resp, err
		}
		log.Infof("NodeStageVolume %s retries %d error %s", req.VolumeId, retries, err)
		time.Sleep(nodeStageRetryWait)
	}
	return resp, err
}

var mountMutex = sync.Mutex{}

func (ns *CsiNfsService) nodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	resp := &csi.NodeStageVolumeResponse{}
	// Implement your logic here
	// Add a bind mount from the staging directory to the pod's target directory.
	// Get lock for concurrency
	serviceName := VolumeIDToServiceName(req.VolumeId)

	log.Infof("shared-nfs NodeStageVolume called volumeID: %s, StagingPath: %s", req.VolumeId, req.StagingTargetPath)

	// First, locate the service
	namespace := DriverNamespace
	service, err := ns.k8sclient.GetService(ctx, namespace, serviceName)
	if err != nil {
		return resp, fmt.Errorf("could not get service Namespace: %s, ServiceName: %s, Service: %+v", namespace, serviceName, service)
	}
	if service.Spec.ClusterIP == "" {
		return resp, fmt.Errorf("NodeStageVolume %s failed, service IP empty", req.VolumeId)
	}
	// Check if already mounted
	mountSource := service.Spec.ClusterIP + ":" + NfsExportDirectory + "/" + serviceName
	if ns.isAlreadyMounted(mountSource) {
		log.Infof("mountSource %s is already mounted- not remounting", mountSource)
		return resp, nil
	}

	if req.StagingTargetPath == "" {
		return resp, fmt.Errorf("NodeStageVolume %s failed, TargetPath empty", req.VolumeId)
	}
	target := req.StagingTargetPath
	// Make sure the target exists
	log.Infof("Making directory for stagingtarget path %s", target)
	output, err := ns.executor.ExecuteCommand("mkdir", "-p", target)
	if err != nil {
		log.Errorf("StagingTarget path %s not created: %s ... proceeding anyway: %s \n", target, err, string(output))
		// Unmount the target directory
		out, err := ns.executor.ExecuteCommand("umount", target)
		log.Infof("shared-nfs NodeStageVolume %s umount target error: %v:\n%s", target, err, string(out))
	}

	log.Info("Changing permissions of target path")
	_, err = ns.executor.ExecuteCommand("chmod", "02777", target)
	if err != nil {
		log.Errorf("Chmod target path failed: %s: \n%s", err, string(output))
	}

	// Mounting the volume
	log.Infof("shared-nfs NodeStage attempting mount %s to %s", mountSource, target)

	cmd := exec.CommandContext(ctx, "mount", "-t", "nfs4", "-o", "max_connect=2", mountSource, target) // #nosec : G204
	log.Infof("%s NodeStage mount command args: %v", req.VolumeId, cmd.Args)

	type cmdResult struct {
		outb []byte
		err  error
	}
	var result cmdResult
	mountCh := make(chan cmdResult, 1)

	// get a lock on mount cmd
	mountMutex.Lock()
	defer mountMutex.Unlock()

	go func() {
		log.Infof("ExportNfsVolume calling mount for volume %s", req.VolumeId)
		outb, err := ns.executor.GetCombinedOutput(cmd)
		mountCh <- cmdResult{outb, err}
	}()

	select {
	case <-ctx.Done():
		log.Errorf("NodeStageVolume timed out while trying to mount volume %s. cmd: %v", req.VolumeId, cmd.Args)
		return nil, fmt.Errorf("NodeStage Mount command timeout")
	case result = <-mountCh:
		if result.err != nil {
			if result.outb != nil {
				log.Infof("%s NodeStage Mount command returned %s", req.VolumeId, string(result.outb))
			}
			log.Infof("%s NodeStage Mount failed: %v : %s", req.VolumeId, cmd.Args, result.err.Error())
			return nil, result.err
		} else {
			log.Infof("NodeStage Mount ALL GOOD result: %v : %s", cmd.Args, string(result.outb))
			return &csi.NodeStageVolumeResponse{}, nil
		}
	}
}

// isAlreadyMounted returns true if there is already a mount for that device
func (ns *CsiNfsService) isAlreadyMounted(device string) bool {
	out, err := ns.executor.ExecuteCommand("mount")
	if err != nil {
		log.Errorf("mount command failed while checking if already mounted: %s", err.Error())
	} else {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.Contains(line, device) {
				log.Infof("%s isAlreadyMounted", device)
				return true
			}
		}
	}
	return false
}

func (ns *CsiNfsService) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	start := time.Now()
	// Get lock for concurrency
	ns.LockPV(req.VolumeId, req.VolumeId, false)
	defer ns.UnlockPV(req.VolumeId)
	target := req.StagingTargetPath
	if !IsNFSVolumeID(req.VolumeId) {
		return &csi.NodeUnstageVolumeResponse{}, fmt.Errorf("nfs NodeUnstageVolume called on non NFS volume %s", req.VolumeId)
	}

	// Unmount the target directory
	log.Infof("Attempting to unmount %s for volume %s", target, req.VolumeId)
	out, err := ns.executor.ExecuteCommandContext(ctx, "umount", "--force", target)
	if err != nil && !strings.Contains(err.Error(), "exit status 32") {
		log.Infof("shared-nfs NodeUnstage umount target %s: error: %s\n%s", target, err, string(out))
		return &csi.NodeUnstageVolumeResponse{}, err
	}
	log.Infof("shared-nfs NodeUnstage umount target succeeded: %s in %s:\n%s", target, time.Since(start), string(out))
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *CsiNfsService) NodePublishVolume(_ context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	resp := &csi.NodePublishVolumeResponse{}
	var err error
	start := time.Now()
	// Get lock for concurrency
	ns.LockPV(req.VolumeId, req.VolumeId, false)
	defer ns.UnlockPV(req.VolumeId)

	log.Infof("Making directory for NodePublish target path %s", req.TargetPath)
	_, err = ns.executor.ExecuteCommand("mkdir", "-p", req.TargetPath)
	if err != nil && !strings.Contains(err.Error(), "exists") {
		return resp, fmt.Errorf("%s: Couldn't create target directory: %s", req.VolumeId, err.Error())
	}

	// Do a bind mount from the staging target to the target
	log.Infof("NodePublishVolume %s attempting bind mount %s -> %s", req.VolumeId, req.StagingTargetPath, req.TargetPath)
	out, err := ns.executor.ExecuteCommand("mount", "--bind", req.StagingTargetPath, req.TargetPath)
	if err != nil {
		log.Errorf("NodePublishVolume %s failed %s", req.VolumeId, err.Error())
		if out != nil {
			log.Infof("NodePublishVolume mount output: %s:", string(out))
		}
		return resp, err
	}
	log.Infof("shared-nfs NodePublishVolume mount bind target succeeded: %s in %s:\n%s", req.TargetPath, time.Since(start), string(out))
	return resp, nil
}

func (ns *CsiNfsService) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	start := time.Now()
	target := req.TargetPath
	// Get lock for concurrency
	ns.LockPV(req.VolumeId, req.VolumeId, false)
	defer ns.UnlockPV(req.VolumeId)
	if !IsNFSVolumeID(req.VolumeId) {
		return &csi.NodeUnpublishVolumeResponse{}, fmt.Errorf("nfs NodeUnpublishVolume called on non NFS volume %s", req.VolumeId)
	}

	// Unmount the target directory
	log.Infof("Attempting to unmount %s for volume %s", target, req.VolumeId)
	// --force is used in case we lose connection to the nfs client,
	// -l (lazy) is used to defer dir cleanup and allow unmounting now, ignoring if the target is busy.
	// TODO: use ExecuteCommandContext to cancel the request if the context times out
	out, err := ns.executor.ExecuteCommandContext(ctx, "umount", "--force", "-l", target)
	if err != nil && !strings.Contains(err.Error(), "exit status 32") {
		log.Infof("shared-nfs NodeUnpublish umount target %s: error: %s\n%s", target, err, string(out))
		return &csi.NodeUnpublishVolumeResponse{}, err
	}
	log.Infof("shared-nfs NodeUnpublish umount target succeeded: %s in %s:\n%s", target, time.Since(start), string(out))
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *CsiNfsService) NodeGetVolumeStats(_ context.Context, _ *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	// Implement your logic here
	return &csi.NodeGetVolumeStatsResponse{}, nil
}

func (ns *CsiNfsService) NodeExpandVolume(_ context.Context, _ *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	// Implement your logic here
	return &csi.NodeExpandVolumeResponse{}, nil
}

func (ns *CsiNfsService) NodeGetCapabilities(_ context.Context, _ *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	// Implement your logic here
	return &csi.NodeGetCapabilitiesResponse{}, nil
}

func (ns *CsiNfsService) NodeGetInfo(_ context.Context, _ *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	// Implement your logic here
	return &csi.NodeGetInfoResponse{}, nil
}

func (ns *CsiNfsService) MountVolume(_ context.Context, _ string, _ string, _ string, _ map[string]string) (string, error) {
	// Implement your logic here
	return "", nil
}

func (ns *CsiNfsService) UnmountVolume(_ context.Context, _ string, _ string, _ map[string]string) error {
	return fmt.Errorf("nfs UnmountVolume not implemented")
}
