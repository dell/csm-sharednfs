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

package mocks

import (
	"context"

	"github.com/dell/csm-hbnfs/nfs/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type MockNfsServer struct {
	proto.UnimplementedNfsServer

	IsError bool
}

func (m *MockNfsServer) Ping(ctx context.Context, req *proto.PingRequest) (*proto.PingResponse, error) {
	if m.IsError {
		return nil, status.Errorf(codes.Internal, "unable to ping node")
	}
	return &proto.PingResponse{Ready: true}, nil
}

func (m *MockNfsServer) GetExports(ctx context.Context, req *proto.GetExportsRequest) (*proto.GetExportsResponse, error) {
	if m.IsError {
		return nil, status.Errorf(codes.Internal, "unable to get exports")
	}

	exports := []string{
		"127.0.0.1:/export1",
		"127.0.0.1:/export2",
	}

	return &proto.GetExportsResponse{Exports: exports}, nil
}
