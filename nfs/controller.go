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
	"maps"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/dell/csm-hbnfs/nfs/proto"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	CsiNfsParameter  = "csi-nfs"
	CsiNfsPrefix     = "nfs"
	CsiNfsPrefixDash = "nfs-"
	ServiceName      = "ServiceName"
	DriverVolumeID   = "driverVolumeID"
)

// Global variables for the controller
var PVLock sync.Map

const DefaultNFSServerPort string = "2049"

func (cs *CsiNfsService) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	// Don't do anything in CreateVolume expect change the volume ID and parameters to avoid recursion
	delete(req.Parameters, CsiNfsParameter)
	subreq := req
	// TODO: consider do we ever need a different access mode
	blockVolumeCapability := &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Block{
			Block: &csi.VolumeCapability_BlockVolume{},
		},
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
		},
	}
	subreq.VolumeCapabilities = []*csi.VolumeCapability{blockVolumeCapability}

	log.Debugf("HBNFS CreateVolume: calling vcsi.CreateVolume; parameters: %+v; capabilities: %v", subreq.Parameters, subreq.VolumeCapabilities)
	resp, err := cs.vcsi.CreateVolume(ctx, subreq)
	if err != nil {
		log.Errorf("HBNFS CreateVolume: failed to create volume; err: %s", err.Error())
		return resp, err
	}

	resp.Volume.VolumeId = ArrayToNFSVolumeID(resp.Volume.VolumeId)
	log.Infof("HBNFS CreateVolume: response %+v", resp)

	return resp, nil
}

func (cs *CsiNfsService) DeleteVolume(_ context.Context, _ *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	// Implement your logic here
	return &csi.DeleteVolumeResponse{}, nil
}

func (cs *CsiNfsService) LockPV(name, requestID string, highPriority bool) {
	sleepTime := 3 * time.Second
	if highPriority {
		sleepTime = 200 * time.Millisecond
	}
	var logged bool
	for {
		holder, loaded := PVLock.LoadOrStore(name, requestID)
		if !loaded {
			break
		}
		if !logged {
			log.Infof("%s waiting on PVLock holder %s", requestID, holder)
			logged = true
		}
		time.Sleep(sleepTime)
	}
}

func (cs *CsiNfsService) UnlockPV(name string) {
	PVLock.Delete(name)
}

func (cs *CsiNfsService) ControllerPublishVolume(ctx context.Context,
	req *csi.ControllerPublishVolumeRequest,
) (*csi.ControllerPublishVolumeResponse, error) {
	resp := &csi.ControllerPublishVolumeResponse{}
	requestID := getRequestIDFromContext(ctx)
	start := time.Now()
	defer finish(ctx, "ControllerPublishVolume", requestID, start)
	// Implement your logic here
	// validate the incoming Parameters have the nfsrwx designation and the PV name of the volume.
	// optain a lock (in the controller) for ths volume
	// determine if there is a Service for this volume. Translate the PV and volume ID to a service name.
	// if there is an existing service, and it has an endpointslice, and the server is healthy, then we don't do anything, so return no error.
	// Either way, annotate the service with all the client nodes ussing the volume.
	// Otherwise, at least to start, we will make this node the NFS server, so call the driver's node publish without the nfsrwx tag and get it to do the export to the node.
	// If that finishes ok, then construct the service and service endpoint.
	// Make sure the lock is released and return good (in a way that the host driver will not add any export)
	name := req.VolumeContext["Name"]
	log.Infof("Entered nfs ControllerPublishVolume %s %s %+v", name, req.VolumeId, req)

	// Get lock for concurrency
	cs.LockPV(req.VolumeId, requestID, false)
	defer cs.UnlockPV(req.VolumeId)

	// Read the PV. This is necessary from which to determine the namespace.
	// Can only guarantee unique service name within 63 long
	serviceName := VolumeIDToServiceName(req.VolumeId)

	// TODO - confirm decision about putting the Service and Endpoint in the driver namespace
	namespace := DriverNamespace
	log.Infof("serviceName %s nfs namespace %s", serviceName, namespace)

	// TODO make the key value generic across different driver types
	node, err := cs.k8sclient.GetNodeByCSINodeID(ctx, DriverName, req.NodeId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not retrieve Node %s: %s", req.NodeId, err)
	}
	nodeIPAddress := ""
	// For now, use the first Node Address (is this always right)?
	if len(node.Status.Addresses) > 0 {
		nodeIPAddress = node.Status.Addresses[0].Address
	} else {
		return nil, status.Errorf(codes.Internal, "could not determine address of Node %s", req.NodeId)
	}
	log.Infof("nfs nodeIpAddress %s", nodeIPAddress)

	// Look to see if there is an existing endpoint slice, and then associated service.
	service, endpoint, err := cs.getServiceAndSlice(ctx, serviceName)
	if err != nil {
		log.Infof("endpointSlice %s/%s not found: %+v", namespace, serviceName, endpoint)
	}

	publishContext := map[string]string{
		"name":       name,
		CsiNfsPrefix: serviceName,
	}
	resp.PublishContext = publishContext

	if endpoint == nil && service == nil {
		// Check the Node Status before proceeding. Otherwise we need to choose another node.
		nodeStatus := cs.GetNodeStatus(nodeIPAddress)
		if nodeStatus == nil || !nodeStatus.online || nodeStatus.inRecovery {
			return nil, status.Errorf(codes.ResourceExhausted, "Node  %s (%s) is not online or is in node recovery", req.NodeId, nodeIPAddress)
		}
		// Here we have the condition no service has been established.
		log.Infof("Call makeNfsService with volume id: %s", req.VolumeId)
		// Passing originalID because the vcsi call is made inside the makeNfsService call.
		service, err := cs.makeNfsService(ctx, namespace, serviceName, name, nodeIPAddress, req)
		log.Infof("makeNfsService response %+v error %s", service, err)
		return resp, err
	}
	if endpoint != nil && service != nil {
		log.Infof("Calling addNodeToNfsService %s %s", service.Name, endpoint)
		service, err = cs.addNodeToNfsService(ctx, service, req)
		if err != nil {
			log.Infof("addNodeToNfsService failed %+v error %s", service, err)
		}
		return resp, err
	}

	log.Info("either service or endpoint slice already existed... will just exit")
	return &csi.ControllerPublishVolumeResponse{}, nil
}

func (cs *CsiNfsService) getServiceAndSlice(ctx context.Context, serviceName string) (*corev1.Service, *discoveryv1.EndpointSlice, error) {
	namespace := DriverNamespace
	// Look to see if there is an existing endpoint slice, and then associated service.
	endpoint, err := cs.k8sclient.GetEndpointSlice(ctx, namespace, serviceName)
	if err != nil {
		log.Infof("endpointSlice %s/%s not found: %+v", namespace, serviceName, endpoint)
		return nil, nil, err
	}

	service, err := cs.k8sclient.GetService(ctx, namespace, serviceName)
	if err != nil {
		log.Infof("service_err %s/%s not found: %+v", namespace, serviceName, service)
		return nil, nil, err
	}

	return service, endpoint, nil
}

func (cs *CsiNfsService) makeNfsService(ctx context.Context, namespace, name string, pvName string, nodeIPAddress string, req *csi.ControllerPublishVolumeRequest) (*corev1.Service, error) {
	nodeID := req.NodeId
	// Export the volume to this node by calling back into the host driver
	subreq := req
	subreq.VolumeId = ToArrayVolumeID(req.VolumeId)
	subreq.VolumeContext["csi-nfs"] = ""
	log.Infof("Calling host driver to publish volume %s to node %s", subreq.VolumeId, subreq.NodeId)
	// TODO: consider do we ever need a different access mode
	blockVolumeCapability := &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Block{
			Block: &csi.VolumeCapability_BlockVolume{},
		},
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
		},
	}
	subreq.VolumeCapability = blockVolumeCapability
	log.Infof("Calling vcsi.ControllerPublish %v", subreq.VolumeCapability)
	subpublishResponse, err := cs.vcsi.ControllerPublishVolume(ctx, subreq)
	log.Info()
	if err != nil {
		log.Errorf("inner ControllerPublishVolume failed: %s", err.Error())
		return nil, err
	}
	log.Infof("Host driver successfully published volume %s to node %s PublishContext %s", req.VolumeId, req.NodeId, subpublishResponse.PublishContext)

	// Send a request to the node to mount the volume
	exportNfsVolumeRequest := &proto.ExportNfsVolumeRequest{
		// volumeID was changed in subseq previous, change back
		VolumeId:         ArrayToNFSVolumeID(req.VolumeId),
		ExportNfsContext: req.VolumeContext,
	}
	exportNfsVolumeRequest.ExportNfsContext[ServiceName] = name
	maps.Copy(exportNfsVolumeRequest.ExportNfsContext, subpublishResponse.PublishContext)

	// issue the request and wait on response or context done
	// if we get a response with an error, issue one more request and wait on response or done
exportRetry:
	for {
		type exportResponse struct {
			response *proto.ExportNfsVolumeResponse
			err      error
		}
		nodeResponse := make(chan exportResponse)
		go func() {
			start := time.Now()
			log.Infof("asynchronously calling ExportNfsVolume %s", exportNfsVolumeRequest.VolumeId)
			resp, err := cs.callExportNfsVolume(ctx, nodeIPAddress, exportNfsVolumeRequest)
			nodeResponse <- exportResponse{resp, err}
			log.Infof("node ExportNfsVolume took %v error %v", time.Since(start), err)
		}()

		// Wait on the node processing to complete
		log.Infof("waiting on node done %s %s...", nodeIPAddress, name)

		select {
		case <-ctx.Done():
			log.Errorf("callExportNfsVolume timed out for volume %s", req.VolumeId)
			return nil, status.Error(codes.Canceled, ctx.Err().Error())
		case nodeExport := <-nodeResponse:
			if nodeExport.err == nil {
				log.Debugf("export response %+v", nodeExport.response)
				close(nodeResponse)
				break exportRetry
			}
			log.Infof("waiting 10s before retrying ExportNfsVolume for volume %s", exportNfsVolumeRequest.VolumeId)
			// Give the node time to catch up, e.g. have the ISCSI path available
			time.Sleep(10 * time.Second)
			log.Infof("Retrying calling ExportNfsVolume %s", exportNfsVolumeRequest.VolumeId)
		}
	}
	// if r := <-resp; r.err != nil {
	// 	// Give the node time to catch up, e.g. have the ISCSI path available
	// 	time.Sleep(10 * time.Second)
	// 	// Retry the call to ExportNfsVolume if the first attempt failed
	// 	start := time.Now()
	// 	log.Infof("Retrying calling ExportNfsVolume %s", exportNfsVolumeRequest.VolumeId)
	// 	exportNfsVolumeContext, exportNfsVolumeCancel := context.WithTimeout(context.Background(), 3*time.Minute)
	// 	nodeResponse, nodeError := cs.callExportNfsVolume(exportNfsVolumeContext, nodeIPAddress, exportNfsVolumeRequest)
	// 	log.Infof("node ExportNfsVolume took %v error %v", time.Since(start), nodeError)
	// 	exportNfsVolumeCancel()
	// 	if nodeError != nil {
	// 		return nil, nodeError
	// 	}
	// }
	log.Infof("ExportNfsVolume %s successful", exportNfsVolumeRequest.VolumeId)

	// Create the endpointslice
	portName := "nfs-server"
	port, err := strconv.Atoi(cs.nfsServerPort)
	if err != nil {
		log.Warnf("invalid port %s - err %v. Defaulting to 2049", cs.nfsServerPort, err)
		port, _ = strconv.Atoi(DefaultNFSServerPort) // default to 2049 if invalid port is parsed
	}
	log.Infof("Setting NFS server port to %d", port)
	portNumber := int32(port) // #nosec : G109,G115

	endpointSlice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"nodeID":                     nodeID,
				"nodeIP":                     nodeIPAddress,
				"kubernetes.io/service-name": name,
				"pvName":                     pvName,
			},
			Annotations: map[string]string{
				DriverVolumeID: req.VolumeId,
			},
		},
		AddressType: discoveryv1.AddressTypeIPv4,
		Endpoints: []discoveryv1.Endpoint{
			{
				Addresses: []string{nodeIPAddress},
				Conditions: discoveryv1.EndpointConditions{
					Ready: func(b bool) *bool { return &b }(true),
				},
			},
		},
		Ports: []discoveryv1.EndpointPort{
			{
				Name: &portName,
				Port: &portNumber,
				// Couldn't get this to compile, seems to default to TCP
				// Protocol: corev1.ProtocolTCP,
			},
		},
	}

	// Create the endpoint.
	log.Infof("Creating endpoint")
	_, err = cs.k8sclient.CreateEndpointSlice(ctx, namespace, endpointSlice)
	if err != nil {
		log.Infof("Could not create EndpointSlice %s: %s", name, err.Error())
		return nil, err
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"NodeId":           req.NodeId,
				"client/" + nodeID: nodeID,
				"NodeIp":           nodeIPAddress,
				"pvName":           pvName,
			},
			Annotations: map[string]string{
				DriverVolumeID: req.VolumeId,
			},
		},
		Spec: corev1.ServiceSpec{
			// This should not be needed, it causes automatic generation of EndpointSlices
			//Selector: map[string]string{
			//"ServiceName": name,
			//},
			Ports: []corev1.ServicePort{
				{
					Name:     "nfs-server",
					Port:     portNumber,
					Protocol: corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	service, err = cs.k8sclient.CreateService(ctx, namespace, service)
	if err != nil {
		log.Infof("Could not create service %s: %s", name, err.Error())
		return nil, err
	}

	//	// Wait on the node processing to complete
	//	log.Infof("waiting on node done %s %s...", nodeIPAddress, name)
	//	wg.Wait()
	//
	//	if nodeError != nil {
	//		// Retry the call to ExportNfsVolume if the first attempt failed
	//		start := time.Now()
	//		log.Infof("synchronously calling ExportNfsVolume")
	//		nodeResponse, nodeError = cs.callExportNfsVolume(ctx, nodeIPAddress, exportNfsVolumeRequest)
	//		log.Infof("node ExportNfsVolume took %v error %v", time.Since(start), nodeError)
	//		if nodeError != nil {
	//			return nil, nodeError
	//		}
	//	}
	return service, nil
}

func (cs *CsiNfsService) addNodeToNfsService(ctx context.Context, service *corev1.Service,
	req *csi.ControllerPublishVolumeRequest,
) (*corev1.Service, error) {
	nodeID := req.NodeId
	if service.Labels["client/"+nodeID] == "" {
		service.Labels["client/"+nodeID] = nodeID
		_, err := cs.k8sclient.UpdateService(ctx, service.Namespace, service)
		if err != nil {
			return nil, err
		}
	}
	return service, nil
}

// removeNodeFromNfsService removes the node label from the service when ControllerUnpublish is called for that node.
// It returns a boolean indicating whether that was the last client (meaning we can delete the service and endpoint), or whether
// other nodes are still using the NFS service.
func (cs *CsiNfsService) removeNodeFromNfsService(ctx context.Context, service *corev1.Service,
	req *csi.ControllerUnpublishVolumeRequest,
) (bool, *corev1.Service, error) {
	delete(service.Labels, "client/"+req.NodeId)
	// Determine howmany clint labels remain. Return true if zero.
	nclients := 0
	for k := range service.Labels {
		if strings.HasPrefix(k, "client/") {
			nclients++
		}
	}
	log.Infof("removeNodeFromService %s %s remaining clients %d", req.VolumeId, req.NodeId, nclients)
	service, err := cs.k8sclient.UpdateService(ctx, service.Namespace, service)
	return (nclients == 0), service, err
}

func (cs *CsiNfsService) callExportNfsVolume(ctx context.Context, nodeIPAddress string, exportNfsVolumeRequest *proto.ExportNfsVolumeRequest) (*proto.ExportNfsVolumeResponse, error) {
	requestID := getRequestIDFromContext(ctx)
	start := time.Now()
	defer finish(ctx, "callExportNfsVolume", requestID, start)
	// Call the node driver to do the NFS export.
	log.Infof("Working on calling nfsExportVolume")
	nodeClient, err := getNfsClient(nodeIPAddress, cs.nfsClientServicePort)
	if err != nil {
		log.Errorf("[callExportNfsVolume] couldn't getNfsClient: %s", err.Error())
		deleteNfsClient(nodeIPAddress)
		return nil, err
	}
	log.Info("got NFS grpc client")
	log.Info("exporting nfs volume")
	exportNfsVolumeResponse, err := nodeClient.ExportNfsVolume(ctx, exportNfsVolumeRequest)
	log.Infof("exportNfsVolume result %+v ... %v", exportNfsVolumeResponse, err)
	return exportNfsVolumeResponse, err
}

func (cs *CsiNfsService) callUnexportNfsVolume(ctx context.Context, nodeIPAddress string, unexportNfsVolumeRequest *proto.UnexportNfsVolumeRequest) (*proto.UnexportNfsVolumeResponse, error) {
	requestID := getRequestIDFromContext(ctx)
	start := time.Now()
	defer finish(ctx, "callUnexportNfsVolume", requestID, start)
	// Call the node driver to do the NFS unexport.
	log.Infof("Working on calling nfsUnexportVolume")
	nodeClient, err := getNfsClient(nodeIPAddress, cs.nfsClientServicePort)
	if err != nil {
		log.Errorf("unable to getNfsClient: %s", err.Error())
		deleteNfsClient(nodeIPAddress)
		return nil, err
	}

	unexportNfsVolumeResponse, err := nodeClient.UnexportNfsVolume(ctx, unexportNfsVolumeRequest)
	if err != nil {
		log.Errorf("callUnexportNfsVolume: returned error %s for %s", err.Error(), nodeIPAddress)
		return nil, err
	}

	return unexportNfsVolumeResponse, nil
}

func (cs *CsiNfsService) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	// If there are no annotations on the service for client nodes, unexport the volume from the node serving as the NFS server.
	// This of is determed from annotations on the service.
	// If there are no more clients, remove the service, endpointslice, and then call the array driver to unpublish the volume.
	// Also remove the Service and its EndpointSlice.
	// If there are still clients using the export, return doing nothing other than removing the node from the service annotation.

	log.Infof("Entered nfs ControllerUnpublishVolume %s %+v", req.VolumeId, req)
	serviceName := VolumeIDToServiceName(req.VolumeId)
	requestID := getRequestIDFromContext(ctx)
	start := time.Now()
	defer finish(ctx, "ControllerUnpublishVolume", requestID, start)
	resp := &csi.ControllerUnpublishVolumeResponse{}

	// Get lock for concurrency
	cs.LockPV(req.VolumeId, requestID, false)
	defer cs.UnlockPV(req.VolumeId)

	service, slice, err := cs.getServiceAndSlice(ctx, serviceName)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			log.Infof("Endpoint slice or service might not exist: %v - slice or service might be deleted.", err)
			return resp, nil
		}

		log.Errorf("failed to get service %s: %v", serviceName, err)
		return nil, err
	}

	var nodeIPAddress string
	if len(slice.Endpoints) > 0 {
		address := slice.Endpoints[0]
		nodeIPAddress = address.Addresses[0]
	}

	if nodeIPAddress == "" {
		return nil, fmt.Errorf("no IP address found for endpointslice: %v", slice)
	}

	// Remove this node from the service.
	var last bool
	last, service, err = cs.removeNodeFromNfsService(ctx, service, req)
	if err != nil {
		return nil, fmt.Errorf("removeNodeFromNfsService failed: %v", err)
	}

	// This was the last node using the service. Unexport the underlying array volume, and remove the service and slice.
	if last {
		log.Infof("ControllerUnpublish removing last node %s: %s", req.VolumeId, nodeIPAddress)
		unexportNfsVolumeContext := make(map[string]string)
		// Call the Node to unpublish the volume completely
		unexportNfsReq := &proto.UnexportNfsVolumeRequest{
			VolumeId:           req.VolumeId,
			UnexportNfsContext: unexportNfsVolumeContext,
		}
		unexportNfsReq.UnexportNfsContext[ServiceName] = serviceName
		_, err := cs.callUnexportNfsVolume(ctx, nodeIPAddress, unexportNfsReq)
		if err != nil {
			if !strings.Contains(err.Error(), "i/o timeout") && !strings.Contains(err.Error(), "no route to host") {
				log.Errorf("[ControllerUnpublish] callUnexportNfsVolume failed: IP: %s, req: %+v, err: %s", nodeIPAddress, unexportNfsReq, err.Error())
				return resp, err
			}

			log.Infof("Node %s might be down, cleaning slice and service - err %s", nodeIPAddress, err.Error())
		}

		// Delete the endpoint slice
		err = cs.k8sclient.DeleteEndpointSlice(ctx, slice.Namespace, serviceName)
		if err != nil {
			log.Errorf("could not delete EndpointSlice %s/%s - %s", slice.Namespace, serviceName, err.Error())
			return nil, err
		}

		// Delete the Service
		err = cs.k8sclient.DeleteService(ctx, service.Namespace, serviceName)
		if err != nil {
			log.Errorf("could not delete Service %s/%s - %s", service.Namespace, serviceName, err.Error())
			return nil, err
		}

		// Call the array driver to unexport the array volume. to the node
		arrayID := ToArrayVolumeID(req.VolumeId)
		req.VolumeId = arrayID
		subreq := req
		return cs.vcsi.ControllerUnpublishVolume(ctx, subreq)
	}

	return resp, nil
}

func (cs *CsiNfsService) ValidateVolumeCapabilities(_ context.Context, _ *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	// Implement your logic here
	return &csi.ValidateVolumeCapabilitiesResponse{}, nil
}

func (cs *CsiNfsService) ListVolumes(_ context.Context, _ *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	// Implement your logic here
	return &csi.ListVolumesResponse{}, nil
}

func (cs *CsiNfsService) GetCapacity(_ context.Context, _ *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	// Implement your logic here
	return &csi.GetCapacityResponse{}, nil
}

func (cs *CsiNfsService) ControllerGetCapabilities(_ context.Context, _ *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	// Implement your logic here
	return &csi.ControllerGetCapabilitiesResponse{}, nil
}

func (cs *CsiNfsService) CreateSnapshot(_ context.Context, _ *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	// Implement your logic here
	return &csi.CreateSnapshotResponse{}, nil
}

func (cs *CsiNfsService) DeleteSnapshot(_ context.Context, _ *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	// Implement your logic here
	return &csi.DeleteSnapshotResponse{}, nil
}

func (cs *CsiNfsService) ListSnapshots(_ context.Context, _ *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	// Implement your logic here
	return &csi.ListSnapshotsResponse{}, nil
}

func (cs *CsiNfsService) ControllerExpandVolume(_ context.Context, _ *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	// Implement your logic here
	return &csi.ControllerExpandVolumeResponse{}, nil
}

func (cs *CsiNfsService) ControllerGetVolume(_ context.Context, _ *csi.ControllerGetVolumeRequest) (
	*csi.ControllerGetVolumeResponse, error,
) {
	// return nil, ni
	return nil, status.Error(400, "Not implemented")
}
