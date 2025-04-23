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
	"net"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	"google.golang.org/grpc/credentials/insecure"

	"github.com/dell/csm-hbnfs/nfs/proto"
)

type (
	ListenFunc func(network, address string) (net.Listener, error)
	ServeFunc  func(s *grpc.Server, lis net.Listener) error
)

type nfsServer struct {
	proto.UnimplementedNfsServer
	executor  Executor
	unmounter Unmounter
	// Embed the unimplemented server
}

// TODO: make these externally configurable
const (
	RootUID           = 0
	nfsGroupID        = 100
	NfsFileMode       = 0o2777
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
func startNfsServiceServer(ipAddress, port string, listenFunc ListenFunc, serveFunc ServeFunc) error {
	log.Infof("csinfs: Calling Listen on %s", ipAddress+":"+port)
	lis, err := listenFunc(ipAddress, port)
	if err != nil {
		log.Infof("csinfs: Listen on ip:port %s:%s failed: %s", ipAddress, port, err.Error())
		return err
	}
	grpcServer := grpc.NewServer()
	log.Infof("csinfs: Calling RegisterNfsServer")
	proto.RegisterNfsServer(grpcServer,
		&nfsServer{
			executor:  &LocalExecutor{},
			unmounter: &SyscallUnmount{},
		})
	if err := serveFunc(grpcServer, lis); err != nil {
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

// TODO: Add cleanup
func deleteNfsClient(_ string) {}

var nfsPVLock sync.Map

func (nfs *nfsServer) nfsLockPV(requestID string) {
	for {
		holder, loaded := nfsPVLock.LoadOrStore(requestID, requestID)
		if !loaded {
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
	resp.VolumeId = req.VolumeId
	start := time.Now()
	defer finish(ctx, "ExportNfsVolume", req.VolumeId, start)
	log.Infof("Received ExportNfsVolume request %s: %+v", req.VolumeId, req)
	nfs.nfsLockPV(req.VolumeId)
	defer nfs.nfsUnlockPV(req.VolumeId)

	// Check for idempotent request
	log.Infof("ExportNfsVolume checking for idenpotent request: %s", req.VolumeId)
	statResult, _ := os.Stat(NfsExportDirectory + "/" + req.VolumeId)
	target := NfsExportDirectory + "/" + req.VolumeId
	exists, err := CheckExport(target + "/")
	if statResult != nil && exists {
		log.Infof("ExportNfsVolume %s already exported")
		if resp.ExportNfsContext == nil {
			resp.ExportNfsContext = make(map[string]string)
		}
		resp.ExportNfsContext["idenpotent"] = "true"
		log.Info("ExportNfsVolume idempotent request volume %s", req.VolumeId)
		return resp, nil
	}

	path, err := nfsService.vcsi.MountVolume(ctx, req.VolumeId, "", NfsExportDirectory, req.ExportNfsContext)
	resp.VolumeId = req.VolumeId
	context := req.ExportNfsContext
	context["MountPath"] = path
	if err != nil {
		return resp, err
	}
	log.Infof("Calling Chown %s %d %d", path, RootUID, nfsGroupID)
	err = opSys.Chown(path, RootUID, nfsGroupID)
	if err != nil {
		log.Errorf("failed chown output: %s", err)
		return resp, err
	}

	// This code is required (doesn't work without it), not sure if the chrooot chmod is needed also.
	log.Infof("Calling Chmod %s mode %o", path, NfsFileMode)
	err = opSys.Chmod(path, NfsFileMode)
	if err != nil {
		log.Errorf("failed chmod output: %s", err)
		return resp, err
	}

	//log.Infof("Calling chroot chmod %s %o", path, NfsFileMode)
	//out, err := GetLocalExecutor().ExecuteCommand("chroot", "/noderoot", "chmod", NfsFileModeString, path)
	//if err != nil {
	//	log.Errorf("failed chroot chmod output: %s %s", err, string(out))
	//	return resp, err
	//}

	// Read the directory entry for the path (debug)
	out, err := GetLocalExecutor().ExecuteCommand("chroot", "/noderoot", "ls", "-ld", path)
	if err != nil {
		log.Errorf("failed chroot output: %s %s", err, string(out))
		return resp, err
	}

	log.Infof("ls -ld %s:\n %s", path, string(out))

	// Add entry in /etc/exports
	options := "(rw)"
	optionsString := nfsService.podCIDR + options
	// Add the link-local overlay network for OCP. TODO: add conditionally?
	optionsString = optionsString + " 169.254.0.0/17" + options
	optionsString = optionsString + " 127.0.0.1/32" + options

	log.Infof("ExportNfsVolume Calling AddExport %s/ %s", path, optionsString)
	generation, err = AddExport(path+"/", optionsString)
	if err != nil {
		log.Errorf("AddExport %s returned error %s", path, err)
		return resp, err
	}
	// Restart the NfsServer
	log.Infof("ExportNfsVolume Resyncing NfsMountd")
	err = ResyncNFSMountd(generation)
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
	defer finish(ctx, "UnexportNfsVolume", req.VolumeId, start)
	log.Infof("Received UnexportNfsVolume request: %+v", req)
	unexportContext := make(map[string]string)

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
		err = restartNFSMountd()
		if err != nil {
			log.Errorf("restartNFSMountd returned error %s", err)
			return resp, err
		}
		log.Errorf("UnmountVolume %s retry %d returned error %s", exportPath, i, err)
	}
	if err != nil {
		log.Infof("UnmountVolume timed out after several retries")
	} else {
		log.Infof("UnexportNfsVolume %s %s ALL GOOD", req.VolumeId, exportPath)
	}
	return resp, err
}

func finish(ctx context.Context, method, requestID string, start time.Time) {
	now := time.Now()
	actual := now.Sub(start)
	deadline, ok := ctx.Deadline()
	if ok {
		maxTime := deadline.Sub(start)
		log.Infof("%s id %s actual time %v max time %v", method, requestID, actual, maxTime)
	} else {
		log.Infof("%s id %s actual time %v", method, requestID, actual)
	}
}

// Ping just returns a response, with Ready boolean and an optional Status. Used by the controller
// to check that nodes are in a good state.
// Note: a problem
func (nfs *nfsServer) Ping(ctx context.Context, req *proto.PingRequest) (*proto.PingResponse, error) {
	requestID := getRequestIDFromContext(ctx)
	start := time.Now()
	defer finish(ctx, "Ping", requestID, start)
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

		err = restartNFSMountd()
		for _, export := range exports {
			exportDir := nodeRoot + "/" + export
			log.Infof("Attempting unmount %s", NfsExportDirectory)
			err := nfs.unmounter.Unmount(exportDir, 0)
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
		}

		log.Infof("Ping DumpAllExports removed %d exports", removed)
	}
	return resp, nil
}

// GetExports returns all exports matching the NfsExortDir
func (nfs *nfsServer) GetExports(ctx context.Context, _ *proto.GetExportsRequest) (*proto.GetExportsResponse, error) {
	requestID := getRequestIDFromContext(ctx)
	start := time.Now()
	defer finish(ctx, "GetExports", requestID, start)
	resp := &proto.GetExportsResponse{}
	exports, err := GetExports(NfsExportDirectory)
	resp.Exports = exports
	return resp, err
}

// ExportMulitpleNfsVolumes will export multiple NFS volumes, using the same context for each.
func (nfs *nfsServer) ExportMultipleNfsVolumes(ctx context.Context,
	req *proto.ExportMultipleNfsVolumesRequest,
) (*proto.ExportMultipleNfsVolumesResponse, error) {
	requestID := getRequestIDFromContext(ctx)
	start := time.Now()
	defer finish(ctx, "ExportMultipleNfsVOlumes", requestID, start)
	successful := make([]string, 0)
	unsuccessful := make([]string, 0)
	var lastError error
	for _, volumeID := range req.VolumeIds {
		exportReq := &proto.ExportNfsVolumeRequest{
			VolumeId:         volumeID,
			ExportNfsContext: req.ExportNfsContext,
		}
		_, err := nfs.ExportNfsVolume(ctx, exportReq)
		if err != nil {
			unsuccessful = append(unsuccessful, volumeID)
			log.Infof("ExportNfsVolume failed for id %s: %s", volumeID, err.Error())
			lastError = err
		} else {
			successful = append(successful, volumeID)
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
	requestID := getRequestIDFromContext(ctx)
	start := time.Now()
	defer finish(ctx, "UnexportMultipleNfsVolumes", requestID, start)
	successful := make([]string, 0)
	unsuccessful := make([]string, 0)
	var lastError error
	for _, volumeID := range req.VolumeIds {
		unexportReq := &proto.UnexportNfsVolumeRequest{
			VolumeId:           volumeID,
			UnexportNfsContext: req.ExportNfsContext,
		}
		_, err := nfs.UnexportNfsVolume(ctx, unexportReq)
		if err != nil {
			unsuccessful = append(unsuccessful, volumeID)
			log.Infof("UnexportNfsVolume failed for id %s: %s", volumeID, err.Error())
			lastError = err
		} else {
			successful = append(successful, volumeID)
		}
	}
	resp := &proto.UnexportMultipleNfsVolumesResponse{}
	resp.SuccessfulIds = successful
	resp.UnsuccessfulIds = unsuccessful
	resp.ExportNfsContext = req.ExportNfsContext
	return resp, lastError
}

func listen(ipAddress string, port string) (net.Listener, error) {
	return net.Listen("tcp", ipAddress+":"+port)
}

func serve(grpcServer *grpc.Server, lis net.Listener) error {
	return grpcServer.Serve(lis)
}
