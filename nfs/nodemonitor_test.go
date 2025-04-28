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
	"crypto/rand"
	"fmt"
	"math/big"
	"net"
	"reflect"
	"strconv"
	"testing"
	"time"

	k8s "github.com/dell/csm-hbnfs/nfs/k8s"
	"github.com/dell/csm-hbnfs/nfs/mocks"
	"github.com/dell/csm-hbnfs/nfs/proto"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetExports(t *testing.T) {
	port := func() string {
		nBig, _ := rand.Int(rand.Reader, big.NewInt(9000))
		return strconv.Itoa(int(nBig.Int64() + 1000))
	}()

	tests := []struct {
		name    string
		ip      string
		want    []string
		wantErr bool
	}{
		{
			name:    "Test GetExports with Valid NFS server",
			ip:      "127.0.0.1",
			want:    []string{"127.0.0.1:/export1", "127.0.0.1:/export2"},
			wantErr: false,
		},
		{
			name:    "Test GetExports with Invalid NFS server",
			ip:      "127.0.0.2",
			want:    []string{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := CsiNfsService{
				nfsClientServicePort: port,
			}

			server := mocks.NewMockNfsServer(gomock.NewController(t))
			if tt.wantErr {
				server.EXPECT().GetExports(gomock.Any(), gomock.Any()).Times(1).Return(nil, status.Errorf(codes.Internal, "failed to get exports"))
			} else {
				server.EXPECT().GetExports(gomock.Any(), gomock.Any()).Times(1).Return(&proto.GetExportsResponse{
					Exports: tt.want,
				}, nil)
			}

			createMockServer(t, tt.ip, port, server)

			exports, err := s.getExports(tt.ip)
			if (err != nil) != tt.wantErr {
				t.Errorf("getExports() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(exports, tt.want) {
				t.Errorf("getExports() = %v, want %v", exports, tt.want)
			}
		})
	}
}

func TestPing(t *testing.T) {
	port := func() string {
		nBig, _ := rand.Int(rand.Reader, big.NewInt(9000))
		return strconv.Itoa(int(nBig.Int64() + 1000))
	}()

	tests := []struct {
		name    string
		ip      string
		want    *proto.PingResponse
		wantErr bool
	}{
		{
			name:    "Test Ping with Valid NFS server",
			ip:      "127.0.0.1",
			want:    &proto.PingResponse{Ready: true},
			wantErr: false,
		},
		{
			name:    "Test Ping with Invalid NFS server",
			ip:      "127.0.0.2",
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := CsiNfsService{nfsClientServicePort: port}

			server := mocks.NewMockNfsServer(gomock.NewController(t))
			if tt.wantErr {
				server.EXPECT().Ping(gomock.Any(), gomock.Any()).Times(1).Return(nil, status.Errorf(codes.Internal, "failed to get exports"))
			} else {
				server.EXPECT().Ping(gomock.Any(), gomock.Any()).Times(1).Return(&proto.PingResponse{
					Ready: true,
				}, nil)
			}

			createMockServer(t, tt.ip, port, server)

			req := &proto.PingRequest{
				NodeIpAddress: tt.ip,
			}

			resp, err := s.ping(req)
			if (err != nil) && !tt.wantErr {
				t.Errorf("ping() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && resp.Ready != tt.want.Ready {
				t.Errorf("ping() = %v, want %v", resp, tt.want)
			}
		})
	}
}

func TestGetNodeExportCounts(t *testing.T) {
	port := func() string {
		nBig, _ := rand.Int(rand.Reader, big.NewInt(9000))
		return strconv.Itoa(int(nBig.Int64() + 1000))
	}()
	localHostIP := "127.0.0.1"

	tests := []struct {
		name         string
		ip           string
		createServer func(t *testing.T)
		want         map[string]int
	}{
		{
			name: "Success - Test GetNodeExportCounts, empty list",
			createServer: func(t *testing.T) {
				server := mocks.NewMockNfsServer(gomock.NewController(t))
				nodeIPAddress = make(map[string]*NodeStatus)
				createMockServer(t, localHostIP, port, server)
			},
			want: map[string]int{},
		},
		{
			name: "Success - Test GetNodeExportCounts, valid response",
			createServer: func(t *testing.T) {
				server := mocks.NewMockNfsServer(gomock.NewController(t))
				server.EXPECT().GetExports(gomock.Any(), gomock.Any()).Times(1).Return(&proto.GetExportsResponse{
					Exports: []string{"127.0.0.1:/export1", "127.0.0.1:/export2"},
				}, nil)
				createMockServer(t, localHostIP, port, server)
				nodeIPAddress[localHostIP] = &NodeStatus{
					nodeName: "myNode",
					nodeIP:   localHostIP,
					online:   true,
					status:   "",
				}
			},
			want: map[string]int{"myNode": 2},
		},
		{
			name: "Success - Test GetNodeExportCounts, inRecovery",
			createServer: func(t *testing.T) {
				server := mocks.NewMockNfsServer(gomock.NewController(t))
				server.EXPECT().GetExports(gomock.Any(), gomock.Any()).Times(1).Return(&proto.GetExportsResponse{
					Exports: []string{"127.0.0.1:/export1", "127.0.0.1:/export2"},
				}, nil)
				createMockServer(t, localHostIP, port, server)
				nodeIPAddress[localHostIP] = &NodeStatus{
					nodeName:   "myNode",
					nodeIP:     localHostIP,
					online:     true,
					status:     "",
					inRecovery: true,
				}
			},
			want: map[string]int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := CsiNfsService{nfsClientServicePort: port}

			tt.createServer(t)

			// Give it time for the server to setup
			time.Sleep(50 * time.Millisecond)

			resp := s.getNodeExportCounts(context.Background())

			if !reflect.DeepEqual(resp, tt.want) {
				t.Errorf("getNodeExportCounts() = %v, want %v", resp, tt.want)
			}
		})
	}
}

func TestPinger(t *testing.T) {
	port := func() string {
		nBig, _ := rand.Int(rand.Reader, big.NewInt(9000))
		return strconv.Itoa(int(nBig.Int64() + 1000))
	}()
	localHostIP := "127.0.0.1"
	tests := []struct {
		name         string
		createServer func(t *testing.T)
		request      *v1.Node
		pingRate     time.Duration
		timeout      time.Duration
	}{
		{
			name:         "Fail - No address in node",
			createServer: func(_ *testing.T) {},
			request: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "myNode",
				},
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{},
				},
			},
			pingRate: 15 * time.Second,
			timeout:  5 * time.Millisecond,
		},
		{
			name: "Success - No address in node",
			createServer: func(t *testing.T) {
				server := mocks.NewMockNfsServer(gomock.NewController(t))
				server.EXPECT().Ping(gomock.Any(), gomock.Any()).Times(1).Return(&proto.PingResponse{Ready: true}, nil)
				createMockServer(t, localHostIP, port, server)
			},
			request: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "myNode",
				},
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{
							Type:    v1.NodeInternalIP,
							Address: localHostIP,
						},
					},
				},
			},
			pingRate: 15 * time.Second,
			timeout:  1 * time.Second,
		},
		{
			name: "Success - Recovery attempt",
			createServer: func(t *testing.T) {
				server := mocks.NewMockNfsServer(gomock.NewController(t))
				server.EXPECT().Ping(gomock.Any(), gomock.Any()).Times(3).Return(&proto.PingResponse{Ready: false}, fmt.Errorf("failed to ping"))
				server.EXPECT().GetExports(gomock.Any(), gomock.Any()).Times(1).Return(nil, status.Errorf(codes.Internal, "unable to get exports"))

				createMockServer(t, localHostIP, port, server)
			},
			request: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "myNode",
				},
				Status: v1.NodeStatus{
					Addresses: []v1.NodeAddress{
						{
							Type:    v1.NodeInternalIP,
							Address: localHostIP,
						},
					},
				},
			},
			pingRate: 1 * time.Second,
			timeout:  3 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new fake clientset
			clientset := fake.NewSimpleClientset()

			s := CsiNfsService{
				k8sclient: &k8s.Client{
					Clientset: clientset,
				},
				nfsClientServicePort: port,
			}

			tt.createServer(t)

			// Give it time for the server to setup
			time.Sleep(50 * time.Millisecond)

			setPingRate(tt.pingRate)
			go s.pinger(tt.request)
			time.Sleep(tt.timeout)
		})
	}
}

func TestIsControlPlaneNode(t *testing.T) {
	tests := []struct {
		name string
		node *v1.Node
		want bool
	}{
		{
			name: "node without labels",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
			},
			want: false,
		},
		{
			name: "node with labels but not control-plane label",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"node-role.kubernetes.io/other-label": "myLabel",
					},
				},
			},
			want: false,
		},
		{
			name: "node with control-plane label",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"node-role.kubernetes.io/control-plane": "",
					},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isControlPlaneNode(tt.node); got != tt.want {
				t.Errorf("isControlPlaneNode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStartNodeMonitor(t *testing.T) {
	port := func() string {
		nBig, _ := rand.Int(rand.Reader, big.NewInt(9000))
		return strconv.Itoa(int(nBig.Int64() + 1000))
	}()
	tests := []struct {
		name      string
		nfsServer *CsiNfsService
		node      *v1.Node
	}{
		{
			name: "Success: ControlPlaneNode",
			nfsServer: func() *CsiNfsService {
				s := &CsiNfsService{nfsClientServicePort: port}
				return s
			}(),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "control-plane-node",
				},
				Spec: v1.NodeSpec{
					Taints: []v1.Taint{
						{
							Key: "node-role.kubernetes.io/control-plane",
						},
					},
				},
			},
		},
		{
			name: "Success: NonControlPlaneNode",
			nfsServer: func() *CsiNfsService {
				s := &CsiNfsService{nfsClientServicePort: port}
				return s
			}(),
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "non-control-plane-node",
				},
				Spec: v1.NodeSpec{
					Taints: []v1.Taint{
						{
							Key: "NonControlPlaneNodeTaint",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(_ *testing.T) {
			tt.nfsServer.startNodeMonitor(tt.node)
		})
	}
}

func TestGetNodeStatus(t *testing.T) {
	tests := []struct {
		name       string
		nfsServer  *CsiNfsService
		nodeIP     string
		wantStatus *NodeStatus
	}{
		{
			name: "Success: Valid IPs",
			nfsServer: func() *CsiNfsService {
				s := &CsiNfsService{}
				nodeStatus := &NodeStatus{
					nodeName: "myNode",
					nodeIP:   "127.0.0.1",
				}

				nodeIPAddress["127.0.0.1"] = nodeStatus

				return s
			}(),
			nodeIP: "127.0.0.1",
			wantStatus: &NodeStatus{
				nodeName: "myNode",
				nodeIP:   "127.0.0.1",
			},
		},
		{
			name: "Success: IP not found",
			nfsServer: func() *CsiNfsService {
				s := &CsiNfsService{}
				nodeStatus := &NodeStatus{
					nodeName: "myNode",
					nodeIP:   "127.0.0.1",
				}

				nodeIPAddress[nodeStatus.nodeIP] = nodeStatus

				return s
			}(),
			nodeIP:     "127.0.0.2",
			wantStatus: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := tt.nfsServer.GetNodeStatus(tt.nodeIP)

			if !reflect.DeepEqual(status, tt.wantStatus) {
				t.Errorf("GetNodeStatus() = %v, want %v", status, tt.wantStatus)
			}
		})
	}
}

func createMockServer(t *testing.T, ip string, port string, mockServer *mocks.MockNfsServer) {
	lis, err := net.Listen("tcp", ip+":"+port)
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}

	t.Cleanup(func() { lis.Close() })

	baseServer := grpc.NewServer()
	t.Cleanup(func() { baseServer.Stop() })

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	proto.RegisterNfsServer(baseServer, mockServer)
	go func() {
		select {
		case <-ctx.Done():
			return
		default:
			if err := baseServer.Serve(lis); err != nil {
				t.Errorf("Failed to serve: %v", err)
			}
		}
	}()
}
