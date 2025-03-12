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

// import (
// 	"context"

// 	"github.com/dell/csm-hbnfs/nfs"
// 	"google.golang.org/grpc/codes"
// 	"google.golang.org/grpc/status"
// )

// type MockNfsServer struct {
// 	nfs.UnimplementedNfsServer

// 	isError bool
// }

// func (m *MockNfsServer) ExportNfsVolume(ctx context.Context, req *nfs.ExportNfsVolumeRequest) (*nfs.ExportNfsVolumeResponse, error) {
// 	return nil, status.Errorf(codes.Unimplemented, "method ExportNfsVolume not implemented")
// }
// func (m *MockNfsServer) UnexportNfsVolume(ctx context.Context, req *nfs.UnexportNfsVolumeRequest) (*nfs.UnexportNfsVolumeResponse, error) {
// 	return nil, status.Errorf(codes.Unimplemented, "method UnexportNfsVolume not implemented")
// }
// func (m *MockNfsServer) Ping(ctx context.Context, req *nfs.PingRequest) (*nfs.PingResponse, error) {
// 	if m.isError {
// 		return nil, status.Errorf(codes.Internal, "unable to ping node")
// 	}
// 	return &nfs.PingResponse{Ready: true}, nil
// }
// func (m *MockNfsServer) GetExports(ctx context.Context, req *nfs.GetExportsRequest) (*nfs.GetExportsResponse, error) {
// 	exports := []string{
// 		"127.0.0.1:/export1",
// 		"127.0.0.1:/export2",
// 	}

// 	return &nfs.GetExportsResponse{Exports: exports}, nil
// }
// func (m *MockNfsServer) ExportMultipleNfsVolumes(ctx context.Context, req *nfs.ExportMultipleNfsVolumesRequest) (*nfs.ExportMultipleNfsVolumesResponse, error) {
// 	return nil, status.Errorf(codes.Unimplemented, "method ExportMultipleNfsVolumes not implemented")
// }
// func (m *MockNfsServer) UnexportMultipleNfsVolumes(ctx context.Context, req *nfs.UnexportMultipleNfsVolumesRequest) (*nfs.UnexportMultipleNfsVolumesResponse, error) {
// 	return nil, status.Errorf(codes.Unimplemented, "method UnexportMultipleNfsVolumes not implemented")
// }
