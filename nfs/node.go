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
	"strings"
	"time"

	//"os"
	"os/exec"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	log "github.com/sirupsen/logrus"
)

var nodePublishTimeout = 10 * time.Second

func (ns *CsiNfsService) NodeStageVolume(_ context.Context, _ *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	// Implement your logic here
	// Add an nfs mount of the volume to the staging directory
	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *CsiNfsService) NodeUnstageVolume(_ context.Context, _ *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	// Implement your logic here
	// Ensure there are no remaining mounts using the staging directory.
	// Remove the NFS mount from the staging directory.
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *CsiNfsService) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	// Run the nodePublishVolume in a retry loop. This is necessary because after a server failover
	// it make take time for the new server to come online and begin serving the volume. This can form a
	// race condition between the new server coming up exporting the volume
	// and existing pods (on a restarted node) trying to reconnect.
	var resp *csi.NodePublishVolumeResponse
	var err error
	retries := 0
	startTime := time.Now()
	defer log.Infof("NodePublishVolume %s completed in %s %d retries error: %v", req.VolumeId, time.Now().Sub(startTime), retries, err)
	for retries := 0; retries < ns.failureRetries; retries++ {
		resp, err = ns.nodePublishVolume(ctx, req)
		if err == nil {
			return resp, err
		}
		log.Infof("NodePublishVolume %s retries %d error %s", req.VolumeId, retries, err)
		time.Sleep(nodePublishTimeout)
	}
	return resp, err
}

func (ns *CsiNfsService) nodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	resp := &csi.NodePublishVolumeResponse{}
	// Implement your logic here
	// Add a bind mount from the staging directory to the pod's target directory.
	// Get lock for concurrency
	serviceName := VolumeIDToServiceName(req.VolumeId)
	ns.LockPV(req.VolumeId, req.VolumeId, false)
	defer ns.UnlockPV(req.VolumeId)

	log.Infof("csi-nfs NodePublishVolume called volumeID %s", req.VolumeId)

	// First, locate the service
	// TODO are we using the vxflexos namespace?
	namespace := DriverNamespace
	service, err := ns.k8sclient.GetService(ctx, namespace, serviceName)
	if err != nil {
		return resp, fmt.Errorf("err %s/%s not found: %+v", namespace, serviceName, service)
	}
	if service.Spec.ClusterIP == "" {
		return resp, fmt.Errorf("NodePublishVolume %s failed, service IP empty", req.VolumeId)
	}

	if req.TargetPath == "" {
		return resp, fmt.Errorf("NodePublishVolume %s failed, TargetPath empty", req.VolumeId)
	}
	target := req.TargetPath
	// Make sure the target exists
	log.Infof("Making directory for target path %s", target)
	output, err := ns.executor.ExecuteCommand("mkdir", "-p", target)
	if err != nil {
		log.Errorf("Target path %s not created: %s ... proceeding anyway: %s \n", target, err, string(output))
		// Unmount the target directory
		cmd := exec.Command("umount", target)
		out, err := cmd.CombinedOutput()
		log.Infof("csi-nfs NodeUnpublish %s umount target error: %v:\n%s", target, err, string(out))
	}

	log.Info("Changing permissions of target path")
	_, err = ns.executor.ExecuteCommand("chmod", "02777", target)
	if err != nil {
		log.Errorf("Chmod target path failed: %s: \n%s", err, string(output))
	}

	// Mounting the volume
	mountSource := service.Spec.ClusterIP + ":" + NfsExportDirectory + "/" + serviceName
	log.Infof("csi-nfs NodePublish attempting mount %s to %s", mountSource, target)
	// cmd := exec.Command("chroot", "/noderoot", "mount", "-t", "nfs4", mountSource, req.TargetPath)
	// cmd := exec.Command("mount", "-t", "nfs4", mountSource, target)
	mountContext, mountCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer mountCancel()
	output, err = ns.executor.ExecuteCommandContext(mountContext, "mount", "-t", "nfs4", mountSource, target)
	// TODO maybe put fsType nfs4 in gofsutil
	// err := gofsutil.Mount(ctx, mountSource, req.TargetPath, "nfs4")
	if err != nil {
		log.Errorf("csi-nfs NodePublish mount %s failed %s", mountSource, err)
		log.Infof("mount command output:\n%s", string(output))
		return resp, err
	}
	log.Infof("csi-nfs NodePublish %s target %s ALL GOOD", req.VolumeId, req.TargetPath)
	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *CsiNfsService) NodeUnpublishVolume(_ context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	start := time.Now()
	target := req.TargetPath
	// Get lock for concurrency
	ns.LockPV(req.VolumeId, req.VolumeId, false)
	defer ns.UnlockPV(req.VolumeId)
	if !IsNFSVolumeID(req.VolumeId) {
		return &csi.NodeUnpublishVolumeResponse{}, fmt.Errorf("nfs NodeUnpublishVolume called on non NFS volume %s", req.VolumeId)
	}
	// Flock, then funlock, in an attempt to flush everything back
	// Although I'd like this code to do a flush, unfortunately the flock can hang.
	// I have not determined a good way around it.
	// flockContext, flockCancel := context.WithTimeout(context.Background(), 3*time.Second)
	// defer flockCancel()
	// cmd := exec.CommandContext(flockContext, "flock", "-s", target, "sync")
	// out, err := cmd.CombinedOutput()
	// if err == nil {
	// 	log.Infof("flock %s \n%s", target, string(out))
	// 	cmd = exec.Command("flock", "-u", target, "sync")
	// 	out, err = cmd.CombinedOutput()
	// 	log.Infof("funlock %s %s\n%s", target, string(out), err)
	// } else if os.IsNotExist(err) {
	// 	return &csi.NodeUnpublishVolumeResponse{}, nil
	// } else {
	// 	log.Infof("flock failed %s- proceeding anyway: %s", req.VolumeId, err)
	// }

	// Unmount the target directory
	log.Infof("Attempting to unmount %s for volume %s", target, req.VolumeId)
	out, err := ns.executor.ExecuteCommand("umount", "--force", target)
	if err != nil && !strings.Contains(err.Error(), "exit status 32") {
		log.Infof("csi-nfs NodeUnpublish umount target %s: error: %s\n%s", target, err, string(out))
		return &csi.NodeUnpublishVolumeResponse{}, err
	}
	log.Infof("csi-nfs NodeUnpublish umount target no error: %s in %s:\n%s", target, time.Since(start), string(out))
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
