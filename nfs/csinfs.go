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
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	k8s "github.com/dell/csm-hbnfs/nfs/k8s"
	"github.com/dell/gocsi"
	csictx "github.com/dell/gocsi/context"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

//go:generate mockgen -destination=mocks/service.go -package=mocks github.com/dell/csm-hbnfs/nfs Service
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
	vcsi                         Service
	md                           Service
	provisionerName              string
	mode                         string
	nodeID                       string
	nodeIPAddress                string
	podCIDR                      string
	nodeName                     string
	failureRetries               int
	k8sclient                    *k8s.Client
	executor                     Executor
	waitCreateNfsServiceInterval time.Duration
	nfsServerPort                string
	nfsClientServicePort         string
}

// OSInterface is an interface that is satisfied by various functions from the
// stdlib os package. It is used to mock out OS calls in unit tests to avoid
// interacting with the OS directly.
//
//go:generate mockgen -destination=mocks/os.go -package=mocks . OSInterface
type OSInterface interface {
	// os.Stat()
	Stat(string) (os.FileInfo, error)
	// os.Getenv()
	Getenv(string) string
	// os.MkdirAll()
	MkdirAll(string, os.FileMode) error
	// os.Open()
	Open(string) (*os.File, error)
	// os.OpenFile()
	OpenFile(string, int, os.FileMode) (*os.File, error)
	// os.Chown()
	Chown(string, int, int) error
	// os.Chmod()
	Chmod(string, os.FileMode) error
}

type OSImpl struct{}

var opSys OSInterface = &OSImpl{}

func (o *OSImpl) Stat(filepath string) (os.FileInfo, error) {
	return os.Stat(filepath)
}

func (o *OSImpl) Getenv(envName string) string {
	return os.Getenv(envName)
}

func (o *OSImpl) MkdirAll(fileName string, perm os.FileMode) error {
	return os.MkdirAll(fileName, perm)
}

func (o *OSImpl) Open(fileName string) (*os.File, error) {
	return os.Open(filepath.Clean(fileName))
}

func (o *OSImpl) OpenFile(fileName string, flag int, perm os.FileMode) (*os.File, error) {
	// #nosec G304 -- This should already be cleaned. False positive.
	return os.OpenFile(filepath.Clean(fileName), flag, perm)
}

func (o *OSImpl) Chown(fileName string, uid, gid int) error {
	return os.Chown(fileName, uid, gid)
}

func (o *OSImpl) Chmod(fileName string, mode os.FileMode) error {
	return os.Chmod(fileName, mode)
}

func IsNFSStorageClass(parameters map[string]string) bool {
	return parameters["csi-nfs"] == "RWX"
}

func hasNFSPrefix(id string) bool {
	return strings.HasPrefix(id, CsiNfsPrefixDash)
}

var (
	IsNFSVolumeID   = hasNFSPrefix
	IsNFSSnapshotID = hasNFSPrefix
)

func ToArrayVolumeID(id string) string {
	return strings.TrimPrefix(id, CsiNfsPrefixDash)
}

// VolumeIDToServiceName takes an id and converts it to a valid DNS name,
// replacing all instances of characters that are not alphanumeric, '-', or '.'
// with the '-' character, removing any leading or trailing characters that are
// not alphanumeric, converting any uppercase alpha characters to lowercase,
// and truncating any strings that exceed 63 chars by removing excess characters.
func VolumeIDToServiceName(id string) string {
	// matches any single lowercase alphanumeric, '-', and '.' chars.
	validTerminalCharRegex := regexp.MustCompile("[a-z0-9]")
	// matches any single character that is not lowercase alphanumeric, '-', or '.'
	illegalCharRegex := regexp.MustCompile("[^a-z0-9-.]")
	// matches a whole word that consists of lowercase alphanumerics, '-', or '.',
	// and both begins and ends with a lowercase alphanumeric,
	validIDRegex := "^[a-z0-9]+([-.]?[a-z0-9]+)*$"

	id = strings.ToLower(id)

	// remove any leading or trailing illegal chars
	validIndexes := validTerminalCharRegex.FindAllIndex([]byte(id), -1)
	if validIndexes != nil {
		// find the index of the first and last valid char and remove everything before the first
		// valid char, and everything after the last valid char
		id = id[validIndexes[0][0]:validIndexes[len(validIndexes)-1][1]]
	}

	// confirm the length does not exceed 63 chars
	if len(id) > 63 {
		id = id[:63]
	}

	// check the format of the string.
	// if it passes the check, return it.
	match, _ := regexp.MatchString(validIDRegex, id)
	if match {
		return id
	}

	// if it fails the check, replace any illegal chars with '-'
	cleaned := illegalCharRegex.ReplaceAllStringFunc(id, func(_ string) string {
		// replace all characters that are not alpha chars with '-'
		return "-"
	})

	return cleaned
}

func ArrayToNFSVolumeID(arrayid string) string {
	return CsiNfsPrefixDash + arrayid
}

var nfsService *CsiNfsService

func New(provisionerName string) Service {
	nfsService = &CsiNfsService{
		provisionerName:              provisionerName,
		executor:                     &LocalExecutor{},
		failureRetries:               10,
		waitCreateNfsServiceInterval: 10 * time.Second,
	}
	return nfsService
}

func PutVcsiService(vcsi Service) {
	nfsService.vcsi = vcsi
}

// ProcessMapSecretChange is not implemented.
func (s *CsiNfsService) ProcessMapSecretChange() error {
	return nil
}

// RegisterAdditionalServers is not implemented.
func (s *CsiNfsService) RegisterAdditionalServers(_ *grpc.Server) {
}

// VolumeIDToArrayID is not implemented.
func (s *CsiNfsService) VolumeIDToArrayID(_ string) string {
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
		_, err := opSys.Stat(NodeRoot + "/etc/exports")
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
		return fmt.Errorf("DriverName not set. This is the value of the driver name in the csinode objects, e.g. csi-vxflexos.dellemc.com")
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

func (s *CsiNfsService) BeforeServe(ctx context.Context, _ *gocsi.StoragePlugin, _ net.Listener) error {
	log.Infof("NFS BeforeServe called")
	var err error

	// Get the SP's operating mode and nodeName
	s.mode = csictx.Getenv(ctx, gocsi.EnvVarMode)

	s.nodeName = opSys.Getenv(EnvCSINodeName)
	if s.nodeName == "" {
		s.nodeName = opSys.Getenv(EnvNodeName)
	}

	if s.nodeName == "" {
		return fmt.Errorf("X_CSI_NODE_NAME or NODE_NAME environment variable not set")
	}

	log.Infof("NFS Driver Mode: %s Node Name %s", s.mode, s.nodeName)

	s.nfsServerPort = os.Getenv(EnvNFSServerPort)
	s.nfsClientServicePort = os.Getenv(EnvNFSClientServicePort)

	// Validate the global variables that must be set up
	if err := s.validateGlobalVariables(); err != nil {
		return err
	}

	// Set up a connection to kubernetes
	log.Info("Connecting to k8sapi")
	s.k8sclient, err = k8s.Connect()
	if err != nil {
		log.Errorf("failed to initialize the kubernetes client in BeforeServe. err: %s", err.Error())
		return err
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
	s.podCIDR = opSys.Getenv(EnvPodCIDR)
	if s.podCIDR == "" {
		s.podCIDR = s.nodeIPAddress + "/8"
	}
	log.Infof("csinode nodeIPAddress %s podCIDR %s", s.nodeIPAddress, s.podCIDR)

	if s.mode != "node" {
		return s.startNodeMonitors()
	}

	// Set up an NFS server for the node pods
	// Process the NfsExportDirectory
	log.Infof("Looking for NFS Export Directory %s", NodeRoot+NfsExportDirectory)
	if _, err = opSys.Stat(NodeRoot + NfsExportDirectory); err != nil {
		err = opSys.MkdirAll(NodeRoot+NfsExportDirectory, 0o777)
		if err != nil {
			log.Infof("MkdirAll %s failed: %s", NodeRoot+NfsExportDirectory, err.Error())
		} else {
			log.Infof("NfsExportDirecotry %s", NfsExportDirectory)
		}
	}

	// Start the NFS server listener
	go func() {
		err := startNfsServiceServer(s.nodeIPAddress, s.nfsClientServicePort, listen, serve)
		if err != nil {
			log.Errorf("failed to start nfs service. err: %s", err.Error())
		}
	}()

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

// getRequestIDFromContext returns the csi.requestid if available in the context
func getRequestIDFromContext(ctx context.Context) string {
	headers, ok := metadata.FromIncomingContext(ctx)
	if ok {
		if req, ok := headers["csi.requestid"]; ok && len(req) > 0 {
			return req[0]
		}
	}
	return ""
}
