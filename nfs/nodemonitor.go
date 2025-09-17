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
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"

	"github.com/dell/csm-sharednfs/nfs/proto"
)

type NodeStatus struct {
	nodeName       string
	nodeIP         string
	badPings       int
	goodPings      int
	inRecovery     bool
	online         bool
	status         string
	dumpingExports bool
}

var GetExportsTimeout = 5 * time.Second

var Pinger = struct {
	rate       time.Duration
	timeout    time.Duration
	maxBadPing int
	monitorMux *sync.Mutex
}{
	rate:       15 * time.Second,
	timeout:    10 * time.Second,
	maxBadPing: 2,
	monitorMux: &sync.Mutex{},
}

// nodeIPAddress has a map of nodeIp to it's status
var nodeIPAddress map[string]*NodeStatus

func init() {
	nodeIPAddress = make(map[string]*NodeStatus)
}

func setPingRate(rate time.Duration) {
	Pinger.monitorMux.Lock()
	defer Pinger.monitorMux.Unlock()
	Pinger.rate = rate
}

func getPingRate() time.Duration {
	Pinger.monitorMux.Lock()
	defer Pinger.monitorMux.Unlock()
	return Pinger.rate
}

func (s CsiNfsService) startNodeMonitor(node *v1.Node) {
	if isControlPlaneNode(node) {
		return
	}

	go s.pinger(node)
}

// GetNodeStatus returns the node status retrieved by the node IP address.
func (s *CsiNfsService) GetNodeStatus(address string) *NodeStatus {
	return nodeIPAddress[address]
}

func (s *CsiNfsService) ping(pingRequest *proto.PingRequest) (*proto.PingResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), Pinger.timeout)
	defer cancel()
	nodeClient, err := getNfsClient(pingRequest.NodeIpAddress, s.nfsClientServicePort)
	if err != nil {
		log.Infof("ping: unable to get nfsClient: %s", err.Error())
		return nil, err
	}

	return nodeClient.Ping(ctx, pingRequest)
}

func (s *CsiNfsService) getExports(nodeIP string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), GetExportsTimeout)
	defer cancel()
	nodeClient, err := getNfsClient(nodeIP, s.nfsClientServicePort)
	if err != nil {
		return make([]string, 0), err
	}
	req := &proto.GetExportsRequest{}
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

	log.Infof("pinger: starting on node %s", node.Name)

	nodeStatus := NodeStatus{
		nodeName:       node.Name,
		nodeIP:         node.Status.Addresses[0].Address,
		online:         false,
		status:         "",
		dumpingExports: false,
	}

	nodeIPAddress[nodeStatus.nodeIP] = &nodeStatus

	// This endless loop pings the node to determine status
	for {
		pingRequest := proto.PingRequest{
			NodeIpAddress:  nodeStatus.nodeIP,
			DumpAllExports: nodeStatus.dumpingExports,
		}

		resp, err := s.ping(&pingRequest)
		if err != nil || (resp != nil && !resp.Ready) {
			log.Warnf("pinger: failed pinging node %s - err: %s", nodeStatus.nodeIP, err.Error())
			if nodeStatus.online {
				log.Infof("pinger: Node %s transitioned to offline", pingRequest.NodeIpAddress)
			}

			if !strings.Contains(err.Error(), "network is unreachable") && !strings.Contains(err.Error(), "error while waiting for new LB policy update") && !strings.Contains(err.Error(), "deadline exceeded while waiting for connections to become ready") {
				nodeStatus.online = false
				nodeStatus.badPings++
				nodeStatus.goodPings = 0
				if !nodeStatus.inRecovery && nodeStatus.badPings >= Pinger.maxBadPing {
					log.Infof("pinger: initiating node recover actions node %s", nodeStatus.nodeIP)
					nodeStatus.inRecovery = true
					nodeStatus.dumpingExports = true

					go s.nodeRecovery(nodeStatus.nodeIP)
				}
			} else {
				log.Info("pinger: We might not be online on this node")
			}
		} else {
			if !nodeStatus.online {
				log.Infof("pinger: Node %s transitioned to online", pingRequest.NodeIpAddress)
			}
			nodeStatus.status = resp.Status
			nodeStatus.online = true
			nodeStatus.badPings = 0
			nodeStatus.goodPings++
			if nodeStatus.goodPings >= 2 {
				nodeStatus.dumpingExports = false
			}

			nodeStatus.inRecovery = false
		}
		time.Sleep(getPingRate())
	}
}

// getNodeExportCounts will return a map of Node Name to number of nfs volumes that are exported
// if the nodes are online.
func (s *CsiNfsService) getNodeExportCounts(_ context.Context) map[string]int {
	exportsMap := make(map[string]int, 0)

	log.Infof("initiating getExports")

	type exportResp struct {
		nodeName string
		quantity int
	}
	var exportChs []chan exportResp

	for _, status := range nodeIPAddress {
		// give each go routine a channel to reply on
		exportCh := make(chan exportResp)
		exportChs = append(exportChs, exportCh)

		go func() {
			defer close(exportCh)
			exports, err := s.getExports(status.nodeIP)
			if err == nil && !status.inRecovery {
				exportCh <- exportResp{status.nodeName, len(exports)}
			} else {
				log.Infof("node %s needs recovery", status.nodeIP)
			}
		}()
	}

	log.Infof("waiting on getExports completion")
	for _, exportCh := range exportChs {
		// read data from the channel until it is closed
		// if it is closed, nothing is added to the map
		for exports := range exportCh {
			exportsMap[exports.nodeName] = exports.quantity
		}
	}

	return exportsMap
}

func isControlPlaneNode(node *v1.Node) bool {
	for key := range node.Labels {
		if key == "node-role.kubernetes.io/control-plane" {
			return true
		}
	}
	return false
}
