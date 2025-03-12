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

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
