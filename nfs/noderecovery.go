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
	"math"
	"strings"
	"sync"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"

	"github.com/dell/csm-hbnfs/nfs/proto"
)

var (
	VolumeReassignTimeout = 180 * time.Second
	exportCountsLock      sync.Mutex
	endpointSliceTimeout  = 3 * time.Second
)

// exportCounts is a map of node name to number of mounts
var exportCounts = make(map[string]int)

// nodeRecovery is called as a goroutine from the pinger when it determines a node is down.
// nodeRecovery is responsible for reassigning all the nfs volumes on the failed node to new servers.
// The algorithm is this:
//  1. Determine the volumes that are on the failed node by reading the endpointslices with a label
//     nodeIp= the failed nodeIp. Fetch the associated services.
//  2. Determine the number of exports used by each node.
//  2. Loop through each endpointslice/service and determine what other nodes are using this volume.
//     Ideally we would like to reassing the volume to a node that is using it.
func (s *CsiNfsService) nodeRecovery(nodeIP string) {
	ctx := context.Background()

	selector := fmt.Sprintf("nodeIP=%s", nodeIP)
	endpointSlices, err := s.k8sclient.GetEndpointSlices(ctx, DriverNamespace, selector)
	if err != nil {
		log.Errorf("pinger: error retrieving endpointSlices: %s: %s", selector, err.Error())
		return
	}

	exportCounts = s.getNodeExportCounts(ctx)

	// Process each volume to be moved in a go routine to move it.
	start := time.Now()
	done := make(chan bool, len(endpointSlices))
	for _, slice := range endpointSlices {
		go func() {
			err := s.reassignVolume(slice)
			if err != nil {
				log.Errorf("reassignVolume failed %s, error %s - retrying", slice.Name, err.Error())
				err = s.reassignVolume(slice)
				if err != nil {
					log.Errorf("reassignVolume failed %s, error %s - retrying", slice.Name, err.Error())
				}
			}
			done <- (err == nil)
		}()
	}

	var successes int
	for range endpointSlices {
		success := <-done
		if success {
			successes++
		}
	}

	log.Infof("reassignVolumes %d successful out of %d in %s", successes, len(endpointSlices), time.Since(start))
}

// reassignVolume recovers a Volume determined from the EndpointSlice and returns true if successful.
// in reassigning it to a new node.
// It is intended to be called by goroutines for multiple volumes in parallel.
func (s *CsiNfsService) reassignVolume(slice *discoveryv1.EndpointSlice) error {
	ctx, cancel := context.WithTimeout(context.Background(), VolumeReassignTimeout)
	defer cancel()
	volumeID := slice.Annotations[DriverVolumeID]
	s.LockPV(volumeID, "reassign", true)
	defer s.UnlockPV(volumeID)

	pvName := slice.Labels["pvName"]
	startTime := time.Now()
	message := fmt.Sprintf("reassignVolume time %s %s", slice.Name, pvName)
	defer logDuration(message, startTime)

	pv, err := s.k8sclient.GetPersistentVolume(ctx, pvName)
	if err != nil {
		return fmt.Errorf("reassignVolume: couldn't Get volume %s: %s", pvName, err)
	}

	service, err := s.k8sclient.GetService(ctx, DriverNamespace, slice.Name)
	if err != nil {
		return fmt.Errorf("reassignVolume: could not Get Service %s: %s", pv.Name, err)
	}

	// Determine the client nodes using the volume and pick a possible target node
	clients := make([]string, 0)
	for key, value := range service.Labels {
		if strings.HasPrefix(key, "client/") {
			clients = append(clients, value)
		}
	}
	log.Infof("reassignVolume %s clients %v", pv.Name, clients)

	selectedNode := findNextAvailableNode(slice, pv)

	// Unexport the volume from the node's NFS server
	// This is done asynchronously just in case the server came back or all exports weren't done
	go func() {
		unexportNfsVolumeContext := make(map[string]string)
		unexportNfsVolumeContext["csi.requestid"] = pvName
		unexportNfsVolumeRequest := &proto.UnexportNfsVolumeRequest{
			VolumeId:           ToArrayVolumeID(pv.Spec.CSI.VolumeHandle),
			UnexportNfsContext: unexportNfsVolumeContext,
		}
		unexportNfsVolumeRequest.UnexportNfsContext[ServiceName] = slice.Name

		// You can't UnexportNfsVolume if the node is still down.
		// However we will try in case it has come up, but no error if this fails.
		_, err = s.callUnexportNfsVolume(ctx, slice.Labels["nodeIP"], unexportNfsVolumeRequest)
		if err != nil {
			log.Debugf("reassigningVolume %s callUnexportNfsVolume failed ... continuing req %v: error %s",
				slice.Name, unexportNfsVolumeRequest, err)
		}
	}()

	// Unpublish the volume from the node
	start := time.Now()
	controllerUnpublishVolumeRequest := &csi.ControllerUnpublishVolumeRequest{
		VolumeId: ToArrayVolumeID(pv.Spec.CSI.VolumeHandle),
		NodeId:   slice.Labels["nodeID"],
	}
	log.Infof("reassignVolume %s calling controllerUnpublishVolume req %v",
		pv.Spec.CSI.VolumeHandle, controllerUnpublishVolumeRequest)
	_, err = s.vcsi.ControllerUnpublishVolume(ctx, controllerUnpublishVolumeRequest)
	if err != nil {
		return fmt.Errorf("reassignVolume %s ControllerUnpublishVolume failed error: %s", pv.Name, err)
	}
	log.Infof("reassignVolume %s ControllerUnpublishVolume complete %s", pv.Name, time.Since(start))

	// Publish the volume to the new node
	node, err := s.k8sclient.GetNode(ctx, selectedNode)
	if err != nil {
		return fmt.Errorf("reassignVolume could not Get the selected node %s: %s", selectedNode, err)
	}
	nodeIPAddress := node.Status.Addresses[0].Address
	// Export the volume from the new node's NFS server
	csiNodeNames := node.Annotations["csi.volume.kubernetes.io/nodeid"]
	csiNodeNames = strings.ReplaceAll(csiNodeNames, "{", "")
	csiNodeNames = strings.ReplaceAll(csiNodeNames, "}", "")
	csiNodeNames = strings.ReplaceAll(csiNodeNames, "\"", "")
	csiNodeNameParts := strings.Split(csiNodeNames, ",")
	driverNodeName := ""
	for i := range csiNodeNameParts {
		if strings.HasPrefix(csiNodeNameParts[i], DriverName) {
			csiNodeNameSubparts := strings.Split(csiNodeNameParts[i], ":")
			if len(csiNodeNameSubparts) > 1 {
				driverNodeName = csiNodeNameSubparts[1]
			}
		}
	}
	if driverNodeName == "" {
		return fmt.Errorf("reassignVolume couldn't identify driverNodeName: %s", csiNodeNames)
	}
	log.Infof("reassignVolume driver NodeName for selected node %s", driverNodeName)

	volumeCapability := &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Block{
			Block: &csi.VolumeCapability_BlockVolume{},
		},
		AccessMode: &csi.VolumeCapability_AccessMode{
			Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
		},
	}
	controllerPublishVolumeRequest := &csi.ControllerPublishVolumeRequest{
		VolumeId:         ToArrayVolumeID(pv.Spec.CSI.VolumeHandle),
		NodeId:           driverNodeName,
		VolumeCapability: volumeCapability,
	}

	start = time.Now()
	log.Infof("reassignVolume %s calling vcsi.ControllerPublishVolume %v", pv.Name, controllerPublishVolumeRequest)
	controllerPublishVolumeResponse, err := s.vcsi.ControllerPublishVolume(ctx,
		controllerPublishVolumeRequest)
	if err != nil {
		return fmt.Errorf("reassignVolume %s got error on ControllerPublishVolume: %s", pv.Name, err)
	}
	log.Infof("contollerPublishVolumes completed %v %s", controllerPublishVolumeResponse, time.Since(start))

	// Send a request to the node to mount the volume
	start = time.Now()
	exportNfsVolumeContext := make(map[string]string)
	exportNfsVolumeContext["csi.requestid"] = pvName
	exportNfsVolumeRequest := &proto.ExportNfsVolumeRequest{
		VolumeId:         pv.Spec.CSI.VolumeHandle,
		ExportNfsContext: exportNfsVolumeContext,
	}
	exportNfsVolumeRequest.ExportNfsContext[ServiceName] = slice.Name
	maps.Copy(exportNfsVolumeRequest.ExportNfsContext, controllerPublishVolumeResponse.PublishContext)
	var nodeError error
	_, nodeError = s.callExportNfsVolume(ctx, nodeIPAddress, exportNfsVolumeRequest)
	if nodeError != nil {
		return fmt.Errorf("callExportNfsVolume failed %s %s: %s", exportNfsVolumeRequest.VolumeId, nodeIPAddress, nodeError)
	}
	log.Infof("ExportNfsVolume %s %s completed successfully %s", slice.Name, nodeIPAddress, time.Since(start))

	err = s.updateEndpointSlice(ctx, slice, driverNodeName, node)
	if err != nil {
		log.Errorf("unable to update endpoint slice %s", slice.Name)
	}

	return nil
}

func (s *CsiNfsService) updateEndpointSlice(ctx context.Context, slice *discoveryv1.EndpointSlice, nodeName string, node *v1.Node) error {
	slice.Labels["nodeID"] = nodeName
	slice.Labels["nodeIP"] = node.Status.Addresses[0].Address
	slice.Endpoints[0].Addresses[0] = node.Status.Addresses[0].Address
	for retries := range 3 {
		log.Infof("Updating EndpointSlice %s to address %s: %v", slice.Name, slice.Labels["nodeIP"], slice)
		_, err := s.k8sclient.UpdateEndpointSlice(ctx, DriverNamespace, slice)
		if err == nil {
			return nil
		}

		log.Errorf("[updateEndpointSlice] slice %s, address %s, retries %d failed: %s", slice.Name, slice.Labels["nodeIP"], retries, err)
		time.Sleep(endpointSliceTimeout)
	}

	return fmt.Errorf("unable to update endpoint slice %s", slice.Name)
}

// Loop through the available nodes and see which one has the lowest count
func findNextAvailableNode(slice *discoveryv1.EndpointSlice, pv *v1.PersistentVolume) string {
	lowest := math.MaxInt
	selectedNode := ""
	aboveThreshold := false

	exportCountsLock.Lock()
	defer exportCountsLock.Unlock()

	for nodeName, exportCount := range exportCounts {
		if exportCount < lowest {
			selectedNode = nodeName
			lowest = exportCount
		}
	}

	// Reserve a slot
	exportCounts[selectedNode] = exportCounts[selectedNode] + 1
	if exportCounts[selectedNode] >= 50 {
		aboveThreshold = true
	}

	log.Infof("rassignVolume %s (%s) selected node %s above threshold %t",
		slice.Name, pv.Spec.CSI.VolumeHandle, selectedNode, aboveThreshold)

	return selectedNode
}

func logDuration(message string, start time.Time) {
	duration := time.Since(start)
	log.Infof("%s %v", message, duration)
}
