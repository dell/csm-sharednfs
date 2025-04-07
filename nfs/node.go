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
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	log "github.com/sirupsen/logrus"
)

var nodeStageTimeout = 10 * time.Second
var nodePublishTimeout = 10 * time.Second

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
		log.Infof("NodeStageVolume %s retries %d error %s", req.VolumeId, retries, err)
		time.Sleep(nodeStageTimeout)
	}
	return resp, err
}

func (ns *CsiNfsService) nodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	resp := &csi.NodeStageVolumeResponse{}
	// Implement your logic here
	// Add a bind mount from the staging directory to the pod's target directory.
	// Get lock for concurrency
	serviceName := VolumeIDToServiceName(req.VolumeId)

	log.Infof("csi-nfs NodeStageVolume called volumeID %s StagingPath", req.VolumeId, req.StagingTargetPath)

	// First, locate the service
	// TODO are we using the vxflexos namespace?
	namespace := DriverNamespace
	service, err := ns.k8sclient.GetService(ctx, namespace, serviceName)
	if err != nil {
		return resp, fmt.Errorf("err %s/%s not found: %+v", namespace, serviceName, service)
	}
	if service.Spec.ClusterIP == "" {
		return resp, fmt.Errorf("NodeStageVolume %s failed, service IP empty", req.VolumeId)
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
		log.Infof("csi-nfs NodeStage %s umount target error: %v:\n%s", target, err, string(out))
	}

	log.Info("Changing permissions of target path")
	_, err = ns.executor.ExecuteCommand("chmod", "02777", target)
	if err != nil {
		log.Errorf("Chmod target path failed: %s: \n%s", err, string(output))
	}

	// Mounting the volume
	mountSource := service.Spec.ClusterIP + ":" + NfsExportDirectory + "/" + serviceName
	log.Infof("csi-nfs NodeStage attempting mount %s to %s", mountSource, target)

	//	mountContext, mountCancel := context.WithTimeout(context.Background(), 3*time.Second)
	//	defer mountCancel()
	//	output, err = ns.executor.ExecuteCommandContext(mountContext, "mount", "-t", "nfs4", mountSource, target)
	//	// TODO maybe put fsType nfs4 in gofsutil

	cmd := exec.Command("mount", "-t", "nfs4", mountSource, target)
	log.Infof("%s NodeStage mount mommand args: %v", req.VolumeId, cmd.Args)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	type cmdResult struct {
		outb []byte
		err  error
	}
	var result cmdResult
	cmdDone := make(chan cmdResult, 1)
	go func() {
		outb, err := cmd.CombinedOutput()
		cmdDone <- cmdResult{outb, err}
	}()
	select {
	case <-time.After(10 * time.Second):
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		log.Errorf("mount command timed out %v", cmd.Args)
		return &csi.NodeStageVolumeResponse{}, fmt.Errorf("NodeStage Mount command timeout")
	case result = <-cmdDone:
		if result.err != nil {
			if result.outb != nil {
				log.Infof("%s NodeStage Mount command returned %s", req.VolumeId, string(result.outb))
			}
			log.Infof("%s NodeStage Mount failed: %v : %s", req.VolumeId, cmd.Args, result.err.Error())
			return &csi.NodeStageVolumeResponse{}, result.err
		} else {
			log.Infof("NodeStage Mount ALL GOOD result: %v : %s", cmd.Args, string(result.outb))
		}
	}
	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *CsiNfsService) NodeUnstageVolume(_ context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
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
	out, err := ns.executor.ExecuteCommand("umount", "--force", target)
	if err != nil && !strings.Contains(err.Error(), "exit status 32") {
		log.Infof("csi-nfs NodeUnstage umount target %s: error: %s\n%s", target, err, string(out))
		return &csi.NodeUnstageVolumeResponse{}, err
	}
	log.Infof("csi-nfs NodeUnstage umount target no error: %s in %s:\n%s", target, time.Since(start), string(out))
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *CsiNfsService) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
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
	log.Infof("csi-nfs NodePublish umount target no error: %s in %s:\n%s", req.TargetPath, time.Since(start), string(out))
	return resp, nil
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

	// Unmount the target directory
	log.Infof("Attempting to unmount %s for volume %s", target, req.VolumeId)
	out, err := ns.executor.ExecuteCommand("umount", "--force", target)
	if err != nil && !strings.Contains(err.Error(), "exit status 32") {
		log.Infof("csi-nfs NodeUnpublish umount target %s: error: %s\n%s", target, err, string(out))
		return &csi.NodeUnpublishVolumeResponse{}, err
	}
	os.Remove(target)
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
