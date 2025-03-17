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
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	"google.golang.org/grpc/credentials/insecure"

	"github.com/dell/csm-hbnfs/nfs/proto"
)

type nfsServer struct {
	proto.UnimplementedNfsServer
	// Embed the unimplemented server
}

// TODO: make these externally configurable
const (
	RootUid           = 0
	NfsGroupId        = 100
	NfsFileMode       = 02777
	NfsFileModeString = "02777"
)

var (
	serverPort    = "2050"
	serverPortMux = &sync.Mutex{}
)

func setServerPort(port string) {
	serverPortMux.Lock()
	defer serverPortMux.Unlock()
	serverPort = port
}

func getServerPort() string {
	serverPortMux.Lock()
	defer serverPortMux.Unlock()
	return serverPort
}

// Starts an NFS server on the specified string port
func startNfsServiceServer(ipAddress, port string) error {
	log.Infof("csinfs: Calling Listen on %s", ipAddress+":"+port)
	lis, err := net.Listen("tcp", ipAddress+":"+port)
	if err != nil {
		log.Infof("csinfs: Listen on ip:port %s:%s failed: %s", ipAddress, port, err.Error())
		return err
	}
	grpcServer := grpc.NewServer()
	log.Infof("csinfs: Calling RegisterNfsServer")
	proto.RegisterNfsServer(grpcServer, &nfsServer{})
	if err := grpcServer.Serve(lis); err != nil {
		log.Errorf("csinfs: grpcServer.Serve failed: %s", err.Error())
		return err
	}
	log.Infof("csinfs: nfsServer running on port %s:%s", ipAddress, port)
	return nil
}

func getNfsClient(ipaddress, port string) (proto.NfsClient, error) {
	// TODO: add support for ipv6, add support for closing the client
	client, err := grpc.NewClient(ipaddress+":"+port, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	// below line is AI generated with deprecations
	// conn, err := grpc.Dial(ipaddress+":"+port, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Errorf("Could not connect to nfsService %s:%s", ipaddress, port)
		return nil, err
	}

	nfsClient := proto.NewNfsClient(client)
	return nfsClient, nil
}

func deleteNfsClient(ipaddress string) {
}

var (
	exportNfsLock sync.Mutex // TBD not currently used remove if possible
	nfsPVLock     sync.Map
)

func (nfs *nfsServer) nfsLockPV(requestID string) {
	for {
		holder, loaded := nfsPVLock.LoadOrStore(requestID, requestID)
		if loaded == false {
			break
		}
		log.Infof("Waiting on PVLock holder %s", holder)
		time.Sleep(1 * time.Second)
	}
}

func (nfs *nfsServer) nfsUnlockPV(requestID string) {
	nfsPVLock.Delete(requestID)
}

// ExportNfsVolume finds the array volume and then add an export entry in /etc/exports and then restart the NFS server.
// The trick is in finding the volume. You actually need some array specific code to do that...
func (nfs *nfsServer) ExportNfsVolume(ctx context.Context, req *proto.ExportNfsVolumeRequest) (*proto.ExportNfsVolumeResponse, error) {
	resp := &proto.ExportNfsVolumeResponse{}
	start := time.Now()
	defer finish("ExportNfsVolume", req.VolumeId, start, ctx)
	log.Infof("Received ExportNfsVolume request %s: %+v", req.VolumeId, req)
	nfs.nfsLockPV(req.VolumeId)
	defer nfs.nfsUnlockPV(req.VolumeId)
	path, err := nfsService.vcsi.MountVolume(ctx, req.VolumeId, "", NfsExportDirectory, req.ExportNfsContext)
	resp.VolumeId = req.VolumeId
	context := req.ExportNfsContext
	context["MountPath"] = path
	if err != nil {
		return resp, err
	}
	log.Infof("Calling Chown %s %d %d", path, RootUid, NfsGroupId)
	err = os.Chown(path, RootUid, NfsGroupId)
	if err != nil {
		log.Errorf("Chown path %s error %s", path, err)
		return resp, err
	}

	// This code is required (doesn't work without it), not sure if the chrooot chmod is needed also.
	log.Infof("Calling Chmod %s mode %o", path, NfsFileMode)
	err = os.Chmod(path, NfsFileMode)
	if err != nil {
		log.Errorf("Chmod of %s failed: %s", path, err)
		return resp, err
	}

	log.Infof("Calling chroot chmod %s %o", path, NfsFileMode)
	cmd := exec.Command("chroot", "/noderoot", "chmod", NfsFileModeString, path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("failed chroot chmod output: %s %s", err, string(output))
		return resp, err
	}

	// Read the directory entry for the path (debug)
	cmd = exec.Command("chroot", "/noderoot", "ls", "-ld", path)
	output, _ = cmd.CombinedOutput()
	log.Infof("ls -ld %s:\n %s", path, string(output))

	// Add entry in /etc/exports
	options := fmt.Sprintf("(rw)")
	optionsString := nfsService.podCIDR + options
	// Add the link-local overlay network for OCP. TODO: add conditionally?
	optionsString = optionsString + " 169.254.0.0/17(rw)"

	log.Infof("ExportNfsVolume Calling AddExport %s/ %s", path, optionsString)
	generation, err = AddExport(path+"/", optionsString)
	if err != nil {
		log.Errorf("AddExport %s returned error %s", path, err)
		return resp, err
	}
	// Restart the NfsServer
	log.Infof("ExportNfsVolume Resyncing NfsMountd")
	err = ResyncNFSMountd(generation)
	// err = restartNFSMountd()
	if err != nil {
		log.Errorf("RestartNfsMountd on behalf of %s returned error %s", path, err)
		return resp, err
	}
	log.Infof("ExportNfsVolume %s %s ALL GOOD", req.VolumeId, path)
	return resp, err
}

// UnexportNfsVolume is called to remove an export from the NFS mount table and remove the directory.
// The req.VolumeId is the array volume ID, not the nfs volume ID.
func (nfs *nfsServer) UnexportNfsVolume(ctx context.Context, req *proto.UnexportNfsVolumeRequest) (*proto.UnexportNfsVolumeResponse, error) {
	start := time.Now()
	defer finish("UnexportNfsVolume", req.VolumeId, start, ctx)
	log.Infof("Received UnexportNfsVolume request: %+v", req)
	unexportContext := make(map[string]string)
	// exportNfsLock.Lock()
	// defer exportNfsLock.Unlock()
	nfs.nfsLockPV(req.VolumeId)
	defer nfs.nfsUnlockPV(req.VolumeId)
	serviceName := req.UnexportNfsContext["ServiceName"]

	// Remove the mount entry from /etc/exports.
	resp := &proto.UnexportNfsVolumeResponse{
		VolumeId:           req.VolumeId,
		UnexportNfsContext: unexportContext,
	}

	log.Infof("NfsExportDirectory %s VolumeId %s serviceName %s", NfsExportDirectory, req.VolumeId, serviceName)
	exportPath := NfsExportDirectory + "/" + req.VolumeId
	if serviceName != "" {
		exportPath = NfsExportDirectory + "/" + serviceName
	}
	exportPath = strings.Replace(exportPath, "//", "/", -1)
	log.Infof("Calling DeleteExport %s", exportPath)
	generation, err := DeleteExport(exportPath)
	if err != nil {
		log.Errorf("RemoveExport %s returned error %s", exportPath, err)
		return resp, err
	}
	err = ResyncNFSMountd(generation)
	// err = restartNFSMountd()
	if err != nil {
		log.Errorf("ResyncNfsMountd on behalf of %s returned error %s", exportPath, err)
		return resp, err
	}

	for i := 1; i <= 3; i++ {
		duration := time.Duration(i) * time.Second
		time.Sleep(duration)
		unmountCtx, unmountCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer unmountCancel()
		log.Infof("Calling UnmountVolume VolumeId %s exportPath %s context %+v", req.VolumeId, exportPath, req.UnexportNfsContext)
		err = nfsService.vcsi.UnmountVolume(unmountCtx, req.VolumeId, NfsExportDirectory, req.UnexportNfsContext)
		if err == nil {
			break
		}
		log.Infof("UnmountVolume returned error %s", err)
		if strings.Contains(err.Error(), "not mounted") {
			err = nil
			break
		}
		// Restart the nfs-mountd. It may be out of sync.
		log.Infof("restarting the NFS service as it may be out of sync")
		restartNFSMountd()
		log.Errorf("UnmountVolume %s retry %d returned error %s", exportPath, i, err)
	}
	if err != nil {
		log.Infof("UnmountVolume timed out after several retries")
	} else {
		log.Infof("UnexportNfsVolume %s %s ALL GOOD", req.VolumeId, exportPath)
	}
	return resp, err
}

func finish(method, requestId string, start time.Time, ctx context.Context) {
	now := time.Now()
	actual := now.Sub(start)
	deadline, ok := ctx.Deadline()
	if ok {
		maxTime := deadline.Sub(start)
		log.Infof("%s id %s actual time %v max time %v", method, requestId, actual, maxTime)
	} else {
		log.Infof("%s id %s actual time %v", method, requestId, actual)
	}
}

// Ping just returns a response, with Ready boolean and an optional Status. Used by the controller
// to check that nodes are in a good state.
// Note: a problem
func (nfs *nfsServer) Ping(ctx context.Context, req *proto.PingRequest) (*proto.PingResponse, error) {
	requestId := getRequestIdFromContext(ctx)
	start := time.Now()
	defer finish("Ping", requestId, start, ctx)
	resp := &proto.PingResponse{}
	log.Debugf("received ping nodeIpAddress %s dumpAllExports %t", req.NodeIpAddress, req.DumpAllExports)
	resp.Ready = true
	resp.Status = ""
	if req.DumpAllExports {
		var removed int
		exports, err := GetExports(NfsExportDirectory)
		if err != nil {
			return resp, err
		}
		log.Infof("Ping received dumpAllExports for %d volumes", len(exports))
		// var generation int64
		// Remove the exports from the kernel
		for _, export := range exports {
			parts := strings.Split(export, " ")
			if len(parts) >= 1 {
				generation, _ = DeleteExport(parts[0])
				removed++
			}
		}
		// err = ResyncNFSMountd(generation)
		err = restartNFSMountd()
		// Unmount the exports
		for _, export := range exports {
			exportDir := noderoot + "/" + export
			log.Infof("Attempting unmount %s", NfsExportDirectory)
			err := syscall.Unmount(exportDir, 0)
			if err != nil {
				log.Errorf("Error unmounting %s: %s", NfsExportDirectory, err)
			} else {
				err := syscall.Rmdir(exportDir)
				if err != nil {
					log.Errorf("Error removing directory %s: %s", NfsExportDirectory, err)
				}
			}
		}

		if err != nil {
			log.Errorf("Ping DumpAllExports resync failed %d exports", removed)
			return resp, err
		} else {
			log.Infof("Ping DumpAllExports removed %d exports", removed)
		}
	}
	return resp, nil
}

// GetExports returns all exports matching the NfsExortDir
func (nfs *nfsServer) GetExports(ctx context.Context, req *proto.GetExportsRequest) (*proto.GetExportsResponse, error) {
	requestId := getRequestIdFromContext(ctx)
	start := time.Now()
	defer finish("GetExports", requestId, start, ctx)
	resp := &proto.GetExportsResponse{}
	exports, err := GetExports(NfsExportDirectory)
	resp.Exports = exports
	return resp, err
}

// ExportMulitpleNfsVolumes will export multiple NFS volumes, using the same context for each.
func (nfs *nfsServer) ExportMultipleNfsVolumes(ctx context.Context,
	req *proto.ExportMultipleNfsVolumesRequest,
) (*proto.ExportMultipleNfsVolumesResponse, error) {
	requestId := getRequestIdFromContext(ctx)
	start := time.Now()
	defer finish("ExportMultipleNfsVOlumes", requestId, start, ctx)
	successful := make([]string, 0)
	unsuccessful := make([]string, 0)
	var lastError error
	for _, volumeId := range req.VolumeIds {
		exportReq := &proto.ExportNfsVolumeRequest{
			VolumeId:         volumeId,
			ExportNfsContext: req.ExportNfsContext,
		}
		_, err := nfs.ExportNfsVolume(ctx, exportReq)
		if err != nil {
			unsuccessful = append(unsuccessful, volumeId)
			log.Infof("ExportNfsVolume failed for id %s: %s", volumeId, err.Error())
			lastError = err
		} else {
			successful = append(successful, volumeId)
		}
	}
	resp := &proto.ExportMultipleNfsVolumesResponse{}
	resp.SuccessfulIds = successful
	resp.UnsuccessfulIds = unsuccessful
	resp.ExportNfsContext = req.ExportNfsContext
	return resp, lastError
}

func (nfs *nfsServer) UnexportMultipleNfsVolumes(ctx context.Context,
	req *proto.UnexportMultipleNfsVolumesRequest,
) (*proto.UnexportMultipleNfsVolumesResponse, error) {
	requestId := getRequestIdFromContext(ctx)
	start := time.Now()
	defer finish("UnexportMultipleNfsVolumes", requestId, start, ctx)
	successful := make([]string, 0)
	unsuccessful := make([]string, 0)
	var lastError error
	for _, volumeId := range req.VolumeIds {
		unexportReq := &proto.UnexportNfsVolumeRequest{
			VolumeId:           volumeId,
			UnexportNfsContext: req.ExportNfsContext,
		}
		_, err := nfs.UnexportNfsVolume(ctx, unexportReq)
		if err != nil {
			unsuccessful = append(unsuccessful, volumeId)
			log.Infof("UnexportNfsVolume failed for id %s: %s", volumeId, err.Error())
			lastError = err
		} else {
			successful = append(successful, volumeId)
		}
	}
	resp := &proto.UnexportMultipleNfsVolumesResponse{}
	resp.SuccessfulIds = successful
	resp.UnsuccessfulIds = unsuccessful
	resp.ExportNfsContext = req.ExportNfsContext
	return resp, lastError
}
