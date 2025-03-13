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
	"testing"
	"time"

	k8s "github.com/dell/csm-hbnfs/nfs/k8s"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestGetExports(t *testing.T) {
	t.Run("Test GetExports with Valid NFS server", func(t *testing.T) {
		s := CsiNfsService{}

		ip := "127.0.0.1"

		createMockServer(t, ip, false)

		exports, err := s.getExports(ip)
		if err != nil {
			t.Error(err)
		}

		if len(exports) == 0 {
			t.Error("no exports found")
		}
	})

	t.Run("Test GetExports with Valid NFS server", func(t *testing.T) {
		s := CsiNfsService{}

		ip := "127.0.0.1"

		createMockServer(t, ip, true)

		_, err := s.getExports(ip)
		if err == nil {
			t.Error(err)
		}
	})
}

func TestPing(t *testing.T) {
	t.Run("Test Ping with Valid NFS server", func(t *testing.T) {
		s := CsiNfsService{}

		ip := "127.0.0.1"

		createMockServer(t, ip, false)

		req := &PingRequest{
			NodeIpAddress: ip,
		}

		resp, err := s.ping(req)
		if err != nil {
			t.Error(err)
		}

		if !resp.Ready {
			t.Fatal("node not ready")
		}

	})

	t.Run("Fail - Test Ping with Invalid NFS server, invalid response", func(t *testing.T) {
		s := CsiNfsService{}

		ip := "127.0.0.1"

		createMockServer(t, ip, true)

		req := &PingRequest{
			NodeIpAddress: ip,
		}

		_, err := s.ping(req)
		if err == nil {
			t.Fatal(err)
		}

	})
}

func TestGetNnodeExportCounts(t *testing.T) {
	t.Run("Success - Test GetNnodeExportCounts, empty list", func(t *testing.T) {
		s := CsiNfsService{}

		ip := "127.0.0.1"

		createMockServer(t, ip, false)

		resp, err := s.getNodeExportCounts(context.Background())
		if err != nil {
			t.Error(err)
		}

		if len(resp) != 0 {
			t.Error("exports located when they shouldn't have been.")
		}
	})

	t.Run("Success - Test GetNnodeExportCounts, valid response", func(t *testing.T) {
		s := CsiNfsService{}

		ip := "127.0.0.1"

		createMockServer(t, ip, false)
		nodeIpToStatus[ip] = &NodeStatus{
			nodeName: "myNode",
			nodeIp:   ip,
			online:   true,
			status:   "",
		}

		resp, err := s.getNodeExportCounts(context.Background())
		if err != nil {
			t.Error(err)
		}

		if len(resp) == 0 {
			t.Error("no exports found")
		}
	})

	t.Run("Success - Test GetNnodeExportCounts, inRecovery", func(t *testing.T) {
		s := CsiNfsService{}

		ip := "127.0.0.1"

		createMockServer(t, ip, false)
		nodeIpToStatus[ip] = &NodeStatus{
			nodeName:   "myNode",
			nodeIp:     ip,
			online:     true,
			status:     "",
			inRecovery: true,
		}

		resp, err := s.getNodeExportCounts(context.Background())
		if err != nil {
			t.Error(err)
		}

		// Since it is inRecovery, no exports should be found
		if len(resp) != 0 {
			t.Error("inRecovery - no exports should be found")
		}
	})
}

func TestPinger(t *testing.T) {
	t.Run("Fail - No address in node", func(t *testing.T) {
		s := CsiNfsService{}

		// No need to create a server since no calls are made.
		req := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "myNode",
			},
			Status: v1.NodeStatus{
				Addresses: []v1.NodeAddress{},
			},
		}

		// No return value but since there is no addresses, it should cover the error case.
		go s.pinger(req)
	})

	t.Run("Success - No address in node", func(t *testing.T) {
		s := CsiNfsService{}

		ip := "127.0.0.1"

		createMockServer(t, ip, false)
		req := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "myNode",
			},
			Status: v1.NodeStatus{
				Addresses: []v1.NodeAddress{
					{
						Type:    v1.NodeInternalIP,
						Address: ip,
					},
				},
			},
		}

		// No return value but since there is no addresses, it should cover the error case.
		go s.pinger(req)
		time.Sleep(1 * time.Second)
	})

	t.Run("Success - Recovery attempt", func(t *testing.T) {
		// Create a new fake clientset
		clientset := fake.NewSimpleClientset()

		s := CsiNfsService{
			k8sclient: &k8s.K8sClient{
				Clientset: clientset,
			},
		}

		ip := "127.0.0.1"

		createMockServer(t, ip, true)

		req := &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "myNode",
			},
			Status: v1.NodeStatus{
				Addresses: []v1.NodeAddress{
					{
						Type:    v1.NodeInternalIP,
						Address: ip,
					},
				},
			},
		}

		// PingRate = 1 * time.Second
		setPingRate(1 * time.Second)
		go s.pinger(req)
		time.Sleep(4 * time.Second)
	})
}

func TestIsControlPlaneNode(t *testing.T) {
	tests := []struct {
		name string
		node *v1.Node
		want bool
	}{
		{
			name: "node without taints",
			node: &v1.Node{
				Spec: v1.NodeSpec{
					Taints: []v1.Taint{},
				},
			},
			want: false,
		},
		{
			name: "node with taint but not control-plane taint",
			node: &v1.Node{
				Spec: v1.NodeSpec{
					Taints: []v1.Taint{
						{
							Key: "node-role.kubernetes.io/other-taint",
						},
					},
				},
			},
			want: false,
		},
		{
			name: "node with control-plane taint",
			node: &v1.Node{
				Spec: v1.NodeSpec{
					Taints: []v1.Taint{
						{
							Key: "node-role.kubernetes.io/control-plane",
						},
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
	// Test case: startNodeMonitor should not do anything if the node is a control plane node
	t.Run("ControlPlaneNode", func(t *testing.T) {
		s := CsiNfsService{}
		node := &v1.Node{
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
		}

		s.startNodeMonitor(node)
	})
	// Test case: startNodeMonitor should start the pinger goroutine if the node is not a control plane node
	t.Run("NonControlPlaneNode", func(t *testing.T) {
		s := CsiNfsService{}
		node := &v1.Node{
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
		}

		s.startNodeMonitor(node)
	})
}

func TestGetNodeStatus(t *testing.T) {
	t.Run("Success: Valid IPs", func(t *testing.T) {
		s := &CsiNfsService{}
		nodeStatus := &NodeStatus{
			nodeName: "myNode",
			nodeIp:   "127.0.0.1",
		}

		nodeIpToStatus["127.0.0.1"] = nodeStatus

		status := s.GetNodeStatus("127.0.0.1")

		if status != nodeStatus {
			t.Errorf("Expected same status, got %v", status)
		}
	})

	t.Run("Success: IP not found", func(t *testing.T) {
		s := &CsiNfsService{}
		nodeStatus := &NodeStatus{
			nodeName: "myNode",
			nodeIp:   "127.0.0.1",
		}

		nodeIpToStatus[nodeStatus.nodeIp] = nodeStatus

		// Look for a different node IP
		status := s.GetNodeStatus("127.0.0.2")

		if status != nil {
			t.Errorf("Expected nil status, got %v", status)
		}
	})
}

func createMockServer(t *testing.T, ip string, isError bool) {
	lis, err := net.Listen("tcp", ip+":"+nfsServerPort)
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}

	t.Cleanup(func() { lis.Close() })

	baseServer := grpc.NewServer()
	t.Cleanup(func() { baseServer.Stop() })

	RegisterNfsServer(baseServer, &mockNfsServer{
		isError: isError,
	})
	go func() {
		if err := baseServer.Serve(lis); err != nil {
			t.Errorf("Failed to serve: %v", err)
		}
	}()

}

type mockNfsServer struct {
	UnimplementedNfsServer

	isError bool
}

func (m *mockNfsServer) Ping(ctx context.Context, req *PingRequest) (*PingResponse, error) {
	if m.isError {
		return nil, status.Errorf(codes.Internal, "unable to ping node")
	}
	return &PingResponse{Ready: true}, nil
}

func (m *mockNfsServer) GetExports(ctx context.Context, req *GetExportsRequest) (*GetExportsResponse, error) {
	if m.isError {
		return nil, status.Errorf(codes.Internal, "unable to get exports")
	}

	exports := []string{
		"127.0.0.1:/export1",
		"127.0.0.1:/export2",
	}

	return &GetExportsResponse{Exports: exports}, nil
}
