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
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
)

type NodeStatus struct {
	nodeName       string
	nodeIp         string
	badPings       int
	goodPings      int
	inRecovery     bool
	online         bool
	status         string
	dumpingExports bool
}

var (
	PingRate          = 15 * time.Second
	PingTimeout       = 10 * time.Second
	GetExportsTimeout = 5 * time.Second
	MaxBadPing        = 2
)

// nodeIpToStatus has a map of nodeIp to it's status
var nodeIpToStatus map[string]*NodeStatus
var monitorMux sync.Mutex

func init() {
	nodeIpToStatus = make(map[string]*NodeStatus)
}

func setPingRate(rate time.Duration) {
	monitorMux.Lock()
	defer monitorMux.Unlock()
	PingRate = rate
}

func getPingRate() time.Duration {
	monitorMux.Lock()
	defer monitorMux.Unlock()
	return PingRate

}

func (s CsiNfsService) startNodeMonitor(node *v1.Node) {
	if isControlPlaneNode(node) {
		return
	}

	go s.pinger(node)
}

// GetNodeStatus returns the node status retrieved by the node IP address.
func (s *CsiNfsService) GetNodeStatus(nodeIpAddress string) *NodeStatus {
	return nodeIpToStatus[nodeIpAddress]
}

func (s *CsiNfsService) ping(pingRequest *PingRequest) (*PingResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), PingTimeout)
	defer cancel()
	nodeClient, err := getNfsClient(pingRequest.NodeIpAddress, nfsServerPort)
	if err != nil {
		return nil, err
	}
	resp, err := nodeClient.Ping(ctx, pingRequest)
	return resp, err
}

func (s *CsiNfsService) getExports(nodeIp string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), GetExportsTimeout)
	defer cancel()
	nodeClient, err := getNfsClient(nodeIp, nfsServerPort)
	if err != nil {
		return make([]string, 0), err
	}
	req := &GetExportsRequest{}
	resp, err := nodeClient.GetExports(ctx, req)
	if err != nil {
		return make([]string, 0), err
	}
	return resp.Exports, err
}

func (s *CsiNfsService) pinger(node *v1.Node) {
	if len(node.Status.Addresses) == 0 {
		log.Errorf("pinger aborting: could not start on node %s because no IP Address", node.Name)
		return
	}
	status := &NodeStatus{
		nodeName: node.Name,
		nodeIp:   node.Status.Addresses[0].Address,
		online:   true,
		status:   "",
	}
	nodeIpToStatus[status.nodeIp] = status

	// This endless loop pings the node to determine status
	for {
		pingRequest := &PingRequest{
			NodeIpAddress:  status.nodeIp,
			DumpAllExports: status.dumpingExports,
		}
		resp, err := s.ping(pingRequest)
		if err != nil || (resp != nil && !resp.Ready) {
			if status.online {
				log.Infof("pinger: Node %s transitioned to offline", pingRequest.NodeIpAddress)
			}
			status.online = false
			status.badPings++
			status.goodPings = 0
			if !status.inRecovery && status.badPings >= MaxBadPing {
				log.Infof("pinger: initiating node recover actions node %s", status.nodeIp)
				status.inRecovery = true
				status.dumpingExports = true
				// cal nodeRecovery to initiate the recovery process
				go s.nodeRecovery(status.nodeIp)
			}
		} else {
			if !status.online {
				log.Infof("pinger: Node %s transitioned to online", pingRequest.NodeIpAddress)
			}
			status.status = resp.Status
			status.online = true
			status.badPings = 0
			status.goodPings++
			if status.goodPings >= 2 {
				status.dumpingExports = false
			}
			status.inRecovery = false
		}
		time.Sleep(getPingRate())
	}
}

// getNodeExportCounts will return a map of Node Name to number of nfs volumes that are exported
// if the nodes are online.
func (s *CsiNfsService) getNodeExportCounts(_ context.Context) (map[string]int, error) {
	numberNodes := len(nodeIpToStatus)
	done := make(chan bool, numberNodes)
	exportsMap := make(map[string]int, 0)
	var nnodes int

	log.Infof("initiating getExports")
	for _, status := range nodeIpToStatus {
		go func() {
			exports, err := s.getExports(status.nodeIp)
			if err == nil && !status.inRecovery {
				exportsMap[status.nodeName] = len(exports)
			} else {
				log.Infof("node %s needs recovery", status.nodeIp)
			}
			done <- true
		}()
		nnodes++
	}

	log.Infof("waiting on getExports completion")
	for range nnodes {
		<-done
	}

	return exportsMap, nil
}

func isControlPlaneNode(node *v1.Node) bool {
	for _, taint := range node.Spec.Taints {
		if taint.Key == "node-role.kubernetes.io/control-plane" {
			return true
		}
	}
	return false
}
