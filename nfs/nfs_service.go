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
	"path/filepath"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"

	"google.golang.org/grpc/credentials/insecure"

	"github.com/dell/csm-hbnfs/nfs/proto"
	k8serros "k8s.io/apimachinery/pkg/api/errors"
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
	timeout            = 5 * time.Second
	maxUnmountAttempts = 10
	maxGetSvcAttempts  = 3
)

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
	// TODO check if this is still applicable: add support for ipv6, add support for closing the client
	client, err := grpc.NewClient(ipaddress+":"+port, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
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
	start := time.Now()
	defer finish(ctx, "ExportNfsVolume", req.VolumeId, start)
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

	log.Infof("Calling chroot chmod %s %o", path, NfsFileMode)
	out, err := GetLocalExecutor().ExecuteCommand("chroot", "/noderoot", "chmod", NfsFileModeString, path)
	if err != nil {
		log.Errorf("failed chroot chmod output: %s %s", err, string(out))
		return resp, err
	}

	// Read the directory entry for the path (debug)
	out, err = GetLocalExecutor().ExecuteCommand("chroot", "/noderoot", "ls", "-ld", path)
	if err != nil {
		log.Errorf("failed chroot output: %s %s", err, string(out))
		return resp, err
	}

	log.Infof("ls -ld %s:\n %s", path, string(out))

	// Add entry in /etc/exports
	options := "(rw,no_subtree_check)"
	optionsString := nfsService.podCIDR + options
	// Add the link-local overlay network for OCP. TODO: add conditionally?
	optionsString = optionsString + " 169.254.0.0/17" + options

	log.Infof("ExportNfsVolume Calling AddExport %s/ %s", path, optionsString)
	generation, err = AddExport(path+"/", optionsString)
	if err != nil {
		log.Errorf("AddExport %s returned error %s", path, err)
		return resp, err
	}

	log.Infof("ExportNfsVolume Resyncing NfsMountd")
	err = ResyncNFSMountd(generation)
	if err != nil {
		log.Errorf("ResyncNFSMountd on behalf of %s returned error %s", path, err)
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
		err = nfs.UnmountVolume(unmountCtx, req.VolumeId, serviceName)
		if err == nil {
			log.Infof("UnexportNfsVolume %s %s ALL GOOD", req.VolumeId, exportPath)
			return resp, nil
		}

		log.Infof("UnmountVolume returned error %s", err)
		if strings.Contains(err.Error(), "not mounted") || strings.Contains(err.Error(), "no such file or directory") {
			log.Infof("UnexportNfsVolume %s %s ALL GOOD", req.VolumeId, exportPath)
			return resp, nil
		}

		err = ResyncNFSMountd(generation)
		if err != nil {
			log.Errorf("ResyncNfsMountd on behalf of %s returned error %s", exportPath, err)
			return resp, err
		}

		log.Errorf("UnmountVolume %s retry %d returned error %s", exportPath, i, err)
	}

	return nil, fmt.Errorf("UnmountVolume timed out after several retries")
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
	resp := &proto.PingResponse{
		Ready:  true,
		Status: "",
	}

	if req.DumpAllExports {
		var removed int
		exports, err := GetExports(NfsExportDirectory)
		if err != nil {
			return resp, err
		}

		for _, export := range exports {
			parts := strings.Split(export, " ")
			exportDir := parts[0]

			exportDir = filepath.Clean(exportDir)
			log.Infof("Attempting unmount %s", exportDir)

			serviceName := strings.Replace(exportDir, NfsExportDirectory, "", 1)
			serviceName = strings.Replace(serviceName, "/", "", 1)

			service, err := nfs.GetServiceContent(serviceName)
			if err != nil {
				log.Errorf("Ping: GetServiceContent returned error %s", err)
				continue
			}

			driverVolumeID, ok := service.Annotations["driverVolumeID"]
			if !ok {
				log.Errorf("Ping: could not find driverVolumeID in Service %s", serviceName)
				continue
			}

			// Delete the export that has an associated service.
			generation, err = DeleteExport(exportDir)
			if err != nil {
				log.Errorf("DeleteExport %s returned error %s", parts[0], err)
				continue
			}

			removed++

			err = ResyncNFSMountd(generation)
			if err != nil {
				log.Errorf("ResyncNFSMountd returned error %s", err)
				return resp, err
			}

			driverVolumeID = "nfs-" + driverVolumeID
			err = nfs.UnmountVolume(ctx, driverVolumeID, serviceName)
			if err != nil {
				log.Errorf("Ping: UnmountVolume returned error %s", err)
				resp.Ready = false
				removed--

				// Ready to try and remove the exports later on.
				optionsString := strings.Join(parts[1:], " ")
				generation, err = AddExport(parts[0], optionsString)
				if err != nil {
					log.Errorf("AddExport %s returned error %s", parts[0], err)
				}

				err = ResyncNFSMountd(generation)
				if err != nil {
					log.Errorf("ResyncNFSMountd returned error %s", err)
				}
			}
			// err = nfs.unmountAndRemove(exportDir, serviceName, parts)
			// if err != nil {
			// 	resp.Ready = false
			// 	removed--
			// }
		}

		if !resp.Ready {
			log.Errorf("Ping DumpAllExports resync failed %d exports", removed)
			return resp, fmt.Errorf("dumping all exports failed")
		}

		log.Infof("Ping DumpAllExports removed %d exports", removed)
	}
	return resp, nil
}

func (nfs *nfsServer) unmountAndRemove(exportDir string, serviceName string, options []string) error {
	// Manually unmount the export
	out, err := GetLocalExecutor().ExecuteCommand("chroot", "/noderoot", "umount", "--force", exportDir)
	if err != nil && !strings.Contains(err.Error(), "exit status 32") {
		log.Errorf("[unmountAndRemove] unable to unmount %s, %s: %s", exportDir, err, string(out))

		optionsString := strings.Join(options[1:], " ")
		generation, err = AddExport(exportDir, optionsString)
		if err != nil {
			log.Errorf("AddExport %s returned error %s", exportDir, err)
		}

		err = ResyncNFSMountd(generation)
		if err != nil {
			log.Errorf("ResyncNFSMountd returned error %s", err)
		}

		return fmt.Errorf("unable to unmount %s", exportDir)
	}

	out, err = GetLocalExecutor().ExecuteCommand("chroot", "/noderoot", "rm", "-rf", exportDir)
	if err != nil {
		log.Errorf("failed rm output: %s %s", err, string(out))
	}

	devDir := NfsExportDirectory + "/" + serviceName + "-dev"
	out, err = GetLocalExecutor().ExecuteCommand("chroot", "/noderoot", "umount", "--force", exportDir)
	if err != nil && !strings.Contains(err.Error(), "exit status 32") {
		log.Errorf("[unmountAndRemove] unable to unmount devDir %s, %s: %s", devDir, err, string(out))
		return fmt.Errorf("unable to unmount %s", exportDir)
	}

	out, err = GetLocalExecutor().ExecuteCommand("chroot", "/noderoot", "rm", "-rf", devDir)
	if err != nil {
		log.Errorf("failed rm output: %s %s", err, string(out))
	}

	return nil
}

func (nfs *nfsServer) UnmountVolume(ctx context.Context, driverVolumeID, serviceName string) error {
	for i := 0; i < maxUnmountAttempts; i++ {
		err := nfsService.vcsi.UnmountVolume(ctx, driverVolumeID, NfsExportDirectory, map[string]string{"ServiceName": serviceName})
		if err == nil {
			return nil
		}

		log.Errorf("UnmountVolume: could not Unmount %s: %s", serviceName, err)
		time.Sleep(timeout)
	}

	return fmt.Errorf("could not unmount volume %s", driverVolumeID)
}

func (nfs *nfsServer) GetServiceContent(serviceName string) (*v1.Service, error) {
	var err error
	for range maxGetSvcAttempts {
		service, err := nfsService.k8sclient.GetService(context.Background(), DriverNamespace, serviceName)
		if err != nil {
			if k8serros.IsNotFound(err) {
				log.Errorf("GetServiceContent: could not find Service %s: %s", serviceName, err)
				return nil, err
			}

			time.Sleep(timeout)
			continue
		}

		log.Infof("GetServiceContent: found Service %s", serviceName)
		return service, nil
	}

	return nil, err
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
