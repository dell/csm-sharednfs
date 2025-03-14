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
	"regexp"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	k8s "github.com/dell/csm-hbnfs/nfs/k8s"
	"github.com/dell/gocsi"
	csictx "github.com/dell/gocsi/context"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type Service interface {
	csi.ControllerServer
	csi.IdentityServer
	csi.NodeServer
	BeforeServe(context.Context, *gocsi.StoragePlugin, net.Listener) error
	RegisterAdditionalServers(server *grpc.Server)
	VolumeIDToArrayID(string) string
	ProcessMapSecretChange() error
	MountVolume(context.Context, string, string, string, map[string]string) (string, error)
	UnmountVolume(context.Context, string, string, map[string]string) error
}

type CsiNfsService struct {
	vcsi            Service
	md              Service
	provisionerName string
	mode            string
	nodeID          string
	nodeIPAddress   string
	podCIDR         string
	nodeName        string
	failureRetries	int

	k8sclient *k8s.K8sClient
	executor  Executor
}

func IsNFSStorageClass(parameters map[string]string) bool {
	return parameters["csi-nfs"] == "RWX"
}

func IsNFSVolumeID(volumeID string) bool {
	return strings.HasPrefix(volumeID, CsiNfsPrefixDash)
}

func IsNFSSnapshotID(volumeID string) bool {
	return strings.HasPrefix(volumeID, CsiNfsPrefixDash)
}

func NFSToArrayVolumeID(id string) string {
	return strings.TrimPrefix(id, CsiNfsPrefixDash)
}

func VolumeIDToServiceName(id string) string {
	rule := "^[a-z0-9]+([-.]?[a-z0-9]+)*$"
	match, _ := regexp.MatchString(rule, id)

	if match {
		return id
	}

	re := regexp.MustCompile("[^a-z0-9-.]")
	cleaned := re.ReplaceAllStringFunc(id, func(match string) string {
		if match == strings.ToLower(match) {
			return "-"
		}
		return strings.ToLower(match)
	})

	if len(cleaned) > 63 {
		cleaned = cleaned[:63]
	}

	return cleaned
}

func ArrayToNFSVolumeID(arrayid string) string {
	return CsiNfsPrefixDash + arrayid
}

var nfsService *CsiNfsService

func New(provisionerName string) Service {
	nfsService = &CsiNfsService{
		provisionerName: provisionerName,
		executor:        &LocalExecutor{},
		failureRetries: 10,
	}
	return nfsService
}

func PutVcsiService(vcsi Service) {
	nfsService.vcsi = vcsi
}

func PutMDService(md Service) {
	nfsService.md = md
}

func (s *CsiNfsService) ProcessMapSecretChange() error {
	return nil
}

func (s *CsiNfsService) RegisterAdditionalServers(server *grpc.Server) {
}

func (s *CsiNfsService) VolumeIDToArrayID(volID string) string {
	return ""
}

// Validate the global variables.
func (s *CsiNfsService) validateGlobalVariables() error {
	if s.mode == "" {
		return fmt.Errorf("s.mode not set to \"node\" or \"controller\"")
	}
	if s.mode == "node" {
		if NodeRoot == "" {
			return fmt.Errorf("csi-nfs NodeRoot variable must be set; used for chroot into node; validated with /noderoot/etc/exports")
		}
		_, err := os.Stat(NodeRoot + "/etc/exports")
		if err != nil {
			return fmt.Errorf("could not stat NodeRoot/etc/exports - this file must exist for kernel nfs installations")
		}
		if NfsExportDirectory == "" {
			return fmt.Errorf("NfsExportDirectory not set it is where block devices are exported via NFS")
		}
	}
	if DriverNamespace == "" {
		return fmt.Errorf("DriverNamespace variable not set; this is used to find Services and EndpointSlices in the driver namespace")
	}
	if DriverName == "" {
		return fmt.Errorf("DriverName not set. This is the value of the driver name in the csinode objects, e.g. csi-vxflexos.dellemc.com ")
	}
	log.Infof("NodeRoot %s NfsExportDirectory %s DriverNamespace %s DriverName %s", NodeRoot, NfsExportDirectory, DriverNamespace, DriverName)
	return nil
}

// Global variables that need to be setup by the driver.
var (
	NodeRoot           string // The path in the Node driver that is the root of the node (e.g. for chroot)
	NfsExportDirectory string // The path with in NodeRoot where the driver's NFS exports should be (e.g. /nfs/exports)
	DriverNamespace    string // The namespace the driver is in. Used to locate Services and EndpointSlices.
	DriverName         string // The name of the driver as reported to the CSI nodes (e.g. csi-vxflexos.dellemc.com)
)

func (s *CsiNfsService) BeforeServe(ctx context.Context, sp *gocsi.StoragePlugin, lis net.Listener) error {
	log.Infof("NFS BeforeServe called")
	var err error

	// Get the SP's operating mode and nodeName
	s.mode = csictx.Getenv(ctx, gocsi.EnvVarMode)

	s.nodeName = os.Getenv("X_CSI_NODE_NAME")
	if s.nodeName == "" {
		s.nodeName = os.Getenv("NODE_NAME")
	}
	if s.nodeName == "" {
		panic("X_CSI_NODE_NAME or NODE_NAME environment variable not set")
	}
	log.Infof("NFS Driver Mode: %s Node Name %s", s.mode, s.nodeName)

	// Validate the global variables that must be set up
	if err := s.validateGlobalVariables(); err != nil {
		return err
	}

	// Set up a connection to kubernetes
	log.Info("Connecting to k8sapi")
	s.k8sclient, err = k8s.Connect()
	if err != nil {
		log.Error(err)
	}
	log.Infof("csinfs: K8sClient connected... s.mode %s", s.mode)

	// Get the IP of our node
	log.Infof("csinfs: calling GetNode: %s", s.nodeName)
	node, err := s.k8sclient.GetNode(ctx, s.nodeName)
	if err != nil {
		log.Errorf("csinfs: Could not Get Node: %s: %s", s.nodeName, err)
		return err
	}
	nodeAddresses := node.Status.Addresses
	if len(nodeAddresses) > 0 {
		s.nodeIPAddress = nodeAddresses[0].Address
	}
	s.podCIDR = os.Getenv("PodCIDR")
	if s.podCIDR == "" {
		s.podCIDR = s.nodeIPAddress + "/8"
	}
	log.Infof("csinode nodeIPAddress %s podCIDR %s", s.nodeIPAddress, s.podCIDR)

	// Set up an NFS server for the node pods
	if s.mode == "node" {

		// Get the NfsExportDirectory if set and use it.
		if os.Getenv("NfsExportDirectory") != "" {
			NfsExportDirectory = os.Getenv("NfsExportDir")
		}
		// Process the NfsExportDirectory
		log.Infof("Looking for NFS Export Directory %s", NodeRoot+NfsExportDirectory)
		if _, err = os.Stat(NodeRoot + NfsExportDirectory); err != nil {
			err = os.MkdirAll(NodeRoot+NfsExportDirectory, 0777)
			if err != nil {
				log.Infof("MkdirAll %s failed: %s", NodeRoot+NfsExportDirectory, err.Error())
			} else {
				log.Infof("NfsExportDirecotry %s", NfsExportDirectory)
			}
		}

		if err = s.initializeNfsServer(); err != nil {
			log.Errorf("host nfs-server failed to initialize")
		}

		// Start the NFS server listener
		// TODO: make port configurable from environment
		go startNfsServiceServer(s.nodeIPAddress, nfsServerPort)
	} else {
		s.startNodeMonitors()
	}
	return nil
}

func (s *CsiNfsService) startNodeMonitors() error {
	ctx := context.Background()
	nodes, err := s.k8sclient.GetAllNodes(ctx)
	if err != nil {
		return fmt.Errorf("unable to retrieve nodes: %s", err)
	}
	for _, node := range nodes {
		s.startNodeMonitor(node)
	}
	return nil
}

// getRequestIdFromContext returns the csi.requestid if available in the context
func getRequestIdFromContext(ctx context.Context) string {
	headers, ok := metadata.FromIncomingContext(ctx)
	if ok {
		if req, ok := headers["csi.requestid"]; ok && len(req) > 0 {
			return req[0]
		}
	}
	return ""
}
