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
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/dell/csm-sharednfs/nfs/k8s"
	"github.com/dell/csm-sharednfs/nfs/mocks"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

var (
	localVolUUID            = "aaaaaaaa-0000-bbbb-1111-cccccccccccc"
	powerstoreLocalSystemID = "PS000000000001"
	scsi                    = "scsi"
	blockVolumeID           = localVolUUID + "/" + powerstoreLocalSystemID + "/" + scsi
	sharedNFSVolumeID       = CsiNfsPrefixDash + blockVolumeID
)

func TestNodeStageVolume(t *testing.T) {
	type args struct {
		ctx context.Context
		req *csi.NodeStageVolumeRequest
	}
	tests := []struct {
		name             string
		args             args
		getCsiNFSService func() *CsiNfsService
		want             *csi.NodeStageVolumeResponse
		wantErr          bool
	}{
		{
			name: "success",
			args: args{
				req: &csi.NodeStageVolumeRequest{
					VolumeId: "vol1",
					VolumeCapability: &csi.VolumeCapability{
						AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
					},
					StagingTargetPath: "path/to/stage",
				},
				ctx: context.Background(),
			},
			getCsiNFSService: func() *CsiNfsService {

				k8sService := &v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "",
						Name:      "vol1",
					},
					Spec: v1.ServiceSpec{
						ClusterIP: "1.2.3.4",
					},
				}

				clientset := fake.NewSimpleClientset(k8sService)

				client := &k8s.Client{
					Clientset: clientset,
				}

				executor := mocks.NewMockExecutor(gomock.NewController(t))
				executor.EXPECT().ExecuteCommand("mount").Times(1).Return([]byte(string("")), nil)
				executor.EXPECT().ExecuteCommand("mkdir", "-p", "path/to/stage").Times(1).Return([]byte(string("")), nil)
				executor.EXPECT().ExecuteCommand("chmod", "02777", "path/to/stage").Times(1).Return([]byte(string("")), nil)
				executor.EXPECT().GetCombinedOutput(gomock.Any()).Times(1).Return([]byte(string("")), nil)

				return &CsiNfsService{
					failureRetries: 10,
					k8sclient:      client,
					executor:       executor,
				}
			},
			want:    &csi.NodeStageVolumeResponse{},
			wantErr: false,
		},
		{
			name: "success - mkdir fails but still passes",
			args: args{
				req: &csi.NodeStageVolumeRequest{
					VolumeId: "vol1",
					VolumeCapability: &csi.VolumeCapability{
						AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
					},
					StagingTargetPath: "path/to/stage",
				},
				ctx: context.Background(),
			},
			getCsiNFSService: func() *CsiNfsService {

				k8sService := &v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "",
						Name:      "vol1",
					},
					Spec: v1.ServiceSpec{
						ClusterIP: "1.2.3.4",
					},
				}

				clientset := fake.NewSimpleClientset(k8sService)

				client := &k8s.Client{
					Clientset: clientset,
				}

				executor := mocks.NewMockExecutor(gomock.NewController(t))
				executor.EXPECT().ExecuteCommand("mount").Times(1).Return([]byte(string("")), nil)

				// mkdir fails but continues with success
				executor.EXPECT().ExecuteCommand("mkdir", "-p", "path/to/stage").Times(1).Return([]byte(string("")), errors.New("mkdir error"))
				executor.EXPECT().ExecuteCommand("umount", "path/to/stage").Times(1).Return([]byte(string("")), nil)
				executor.EXPECT().ExecuteCommand("chmod", "02777", "path/to/stage").Times(1).Return([]byte(string("")), nil)
				executor.EXPECT().GetCombinedOutput(gomock.Any()).Times(1).Return([]byte(string("")), nil)

				return &CsiNfsService{
					failureRetries: 10,
					k8sclient:      client,
					executor:       executor,
				}
			},
			want:    &csi.NodeStageVolumeResponse{},
			wantErr: false,
		},
		{
			name: "error",
			args: args{
				req: &csi.NodeStageVolumeRequest{
					VolumeId: "vol1",
					VolumeCapability: &csi.VolumeCapability{
						AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER},
					},
					StagingTargetPath: "path/to/stage",
				},
				ctx: context.Background(),
			},
			getCsiNFSService: func() *CsiNfsService {

				k8sService := &v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "",
						Name:      "vol1",
					},
					Spec: v1.ServiceSpec{
						ClusterIP: "1.2.3.4",
					},
				}

				clientset := fake.NewSimpleClientset(k8sService)

				client := &k8s.Client{
					Clientset: clientset,
				}

				executor := mocks.NewMockExecutor(gomock.NewController(t))
				executor.EXPECT().ExecuteCommand("mount").Times(1).Return([]byte(string("")), nil)
				executor.EXPECT().ExecuteCommand("mkdir", "-p", "path/to/stage").Times(1).Return([]byte(string("")), nil)
				executor.EXPECT().ExecuteCommand("chmod", "02777", "path/to/stage").Times(1).Return([]byte(string("")), nil)
				executor.EXPECT().GetCombinedOutput(gomock.Any()).Times(1).Return([]byte(string("command output")), errors.New("error getting command output"))

				return &CsiNfsService{
					failureRetries: 10,
					k8sclient:      client,
					executor:       executor,
				}
			},
			want:    &csi.NodeStageVolumeResponse{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := tt.getCsiNFSService()
			got, err := service.NodeStageVolume(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("NodeStageVolume() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NodeStageVolume() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNodeUnstageVolume(t *testing.T) {
	type args struct {
		ctx context.Context
		req *csi.NodeUnstageVolumeRequest
	}
	tests := []struct {
		name             string
		args             args
		getCsiNFSService func() *CsiNfsService
		want             *csi.NodeUnstageVolumeResponse
		wantErr          bool
		errMsg           string
	}{
		{
			name: "success",
			args: args{
				req: &csi.NodeUnstageVolumeRequest{
					VolumeId:          sharedNFSVolumeID,
					StagingTargetPath: "path/to/stage",
				},
			},
			getCsiNFSService: func() *CsiNfsService {
				executor := mocks.NewMockExecutor(gomock.NewController(t))
				executor.EXPECT().ExecuteCommand("umount", "--force", gomock.Any()).Times(1).Return([]byte{}, nil)
				return &CsiNfsService{
					executor: executor,
				}
			},
			want:    &csi.NodeUnstageVolumeResponse{},
			wantErr: false,
		},
		{
			name: "with a regular block volume ID",
			args: args{
				req: &csi.NodeUnstageVolumeRequest{
					VolumeId:          blockVolumeID,
					StagingTargetPath: "path/to/stage",
				},
			},
			getCsiNFSService: func() *CsiNfsService { return &CsiNfsService{} },
			want:             &csi.NodeUnstageVolumeResponse{},
			wantErr:          true,
			errMsg:           "nfs NodeUnstageVolume called on non NFS volume",
		},
		{
			name: "when umount fails",
			args: args{
				req: &csi.NodeUnstageVolumeRequest{
					VolumeId:          sharedNFSVolumeID,
					StagingTargetPath: "path/to/stage",
				},
			},
			getCsiNFSService: func() *CsiNfsService {
				executor := mocks.NewMockExecutor(gomock.NewController(t))
				executor.EXPECT().ExecuteCommand("umount", "--force", gomock.Any()).Times(1).Return([]byte{}, errors.New("umount error"))
				return &CsiNfsService{
					executor: executor,
				}
			},
			want:    &csi.NodeUnstageVolumeResponse{},
			wantErr: true,
			errMsg:  "umount error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := tt.getCsiNFSService()
			got, err := service.NodeUnstageVolume(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("NodeUnstageVolume() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				assert.Contains(t, err.Error(), tt.errMsg)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NodeUnstageVolume() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNodeGetVolumeStats(t *testing.T) {
	type args struct {
		ctx context.Context
		req *csi.NodeGetVolumeStatsRequest
	}
	tests := []struct {
		name    string
		args    args
		want    *csi.NodeGetVolumeStatsResponse
		wantErr bool
	}{
		{
			name: "success",
			args: args{
				req: &csi.NodeGetVolumeStatsRequest{
					VolumeId:          sharedNFSVolumeID,
					StagingTargetPath: "path/to/stage",
				},
			},
			want:    &csi.NodeGetVolumeStatsResponse{},
			wantErr: false,
		},
		{
			name: "with regular block volume ID",
			args: args{
				req: &csi.NodeGetVolumeStatsRequest{
					// a regular block volume ID is not supported
					VolumeId:          sharedNFSVolumeID,
					StagingTargetPath: "path/to/stage",
				},
			},
			want:    &csi.NodeGetVolumeStatsResponse{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &CsiNfsService{}
			got, err := service.NodeGetVolumeStats(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("NodeGetVolumeStats() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NodeGetVolumeStats() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNodeExpandVolume(t *testing.T) {
	type args struct {
		ctx context.Context
		req *csi.NodeExpandVolumeRequest
	}
	tests := []struct {
		name    string
		args    args
		want    *csi.NodeExpandVolumeResponse
		wantErr bool
	}{
		{
			name: "success",
			args: args{
				req: &csi.NodeExpandVolumeRequest{
					VolumeId:   "vol1",
					VolumePath: "path/to/volume",
				},
			},
			want:    &csi.NodeExpandVolumeResponse{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &CsiNfsService{}
			got, err := service.NodeExpandVolume(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("NodeExpandVolume() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NodeExpandVolume() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNodeGetCapabilities(t *testing.T) {
	type args struct {
		ctx context.Context
		req *csi.NodeGetCapabilitiesRequest
	}
	tests := []struct {
		name    string
		args    args
		want    *csi.NodeGetCapabilitiesResponse
		wantErr bool
	}{
		{
			name: "success",
			args: args{
				req: &csi.NodeGetCapabilitiesRequest{},
			},
			want:    &csi.NodeGetCapabilitiesResponse{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &CsiNfsService{}
			got, err := service.NodeGetCapabilities(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("NodeGetCapabilities() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NodeGetCapabilities() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNodeGetInfo(t *testing.T) {
	type args struct {
		ctx context.Context
		req *csi.NodeGetInfoRequest
	}
	tests := []struct {
		name    string
		args    args
		want    *csi.NodeGetInfoResponse
		wantErr bool
	}{
		{
			name: "success",
			args: args{
				req: &csi.NodeGetInfoRequest{},
			},
			want:    &csi.NodeGetInfoResponse{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &CsiNfsService{}
			got, err := service.NodeGetInfo(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("NodeGetInfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NodeGetInfo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMountVolume(t *testing.T) {
	type args struct {
		context      context.Context
		volumeID     string
		fsType       string
		nfsExportDir string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "success",
			args: args{
				volumeID:     "vol1",
				fsType:       "ext4",
				nfsExportDir: "/export",
			},
			want:    "",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		service := &CsiNfsService{}
		got, err := service.MountVolume(tt.args.context, tt.args.volumeID, tt.args.fsType, tt.args.nfsExportDir, nil)
		if (err != nil) != tt.wantErr {
			t.Errorf("MountVolume() error = %v, wantErr %v", err, tt.wantErr)
			return
		}
		if got != tt.want {
			t.Errorf("MountVolume() = %v, want %v", got, tt.want)
		}
	}
}

func TestUnmountVolume(t *testing.T) {
	type args struct {
		context         context.Context
		volumeID        string
		exportDirectory string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "fail",
			args: args{
				volumeID:        "vol1",
				exportDirectory: "/export",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		service := &CsiNfsService{}
		err := service.UnmountVolume(tt.args.context, tt.args.volumeID, tt.args.exportDirectory, nil)
		if (err != nil) != tt.wantErr {
			t.Errorf("UnmountVolume() error = %v, wantErr %v", err, tt.wantErr)
			return
		}
	}
}

func TestNodePublishVolume(t *testing.T) {
	type args struct {
		ctx context.Context
		req *csi.NodePublishVolumeRequest
	}
	tests := []struct {
		name             string
		args             args
		getCsiNFSService func() *CsiNfsService
		want             *csi.NodePublishVolumeResponse
		wantErr          bool
		errMsg           string
	}{
		{
			name: "success",
			args: args{
				ctx: context.Background(),
				req: &csi.NodePublishVolumeRequest{
					VolumeId:          sharedNFSVolumeID,
					TargetPath:        "/data0",
					StagingTargetPath: "/var/lib/dell/nfs",
				},
			},
			getCsiNFSService: func() *CsiNfsService {
				executor := mocks.NewMockExecutor(gomock.NewController(t))
				executor.EXPECT().ExecuteCommand("mkdir", "-p", gomock.Any()).Times(1).Return([]byte{}, nil)
				executor.EXPECT().ExecuteCommand("mount", "--bind", gomock.Any(), gomock.Any()).Times(1).Return([]byte{}, nil)
				return &CsiNfsService{
					executor: executor,
				}
			},
			want:    &csi.NodePublishVolumeResponse{},
			wantErr: false,
		},
		{
			name: "mkdir fails to create the directory",
			args: args{
				ctx: context.Background(),
				req: &csi.NodePublishVolumeRequest{
					VolumeId:          sharedNFSVolumeID,
					TargetPath:        "/data0",
					StagingTargetPath: "/var/lib/dell/nfs",
				},
			},
			getCsiNFSService: func() *CsiNfsService {
				executor := mocks.NewMockExecutor(gomock.NewController(t))
				executor.EXPECT().ExecuteCommand("mkdir", "-p", gomock.Any()).Times(1).Return([]byte{}, errors.New("failed to create the directory"))
				return &CsiNfsService{
					executor: executor,
				}
			},
			want:    &csi.NodePublishVolumeResponse{},
			wantErr: true,
			errMsg:  "failed to create the directory",
		},
		{
			name: "bind mount fails",
			args: args{
				ctx: context.Background(),
				req: &csi.NodePublishVolumeRequest{
					VolumeId:          sharedNFSVolumeID,
					TargetPath:        "/data0",
					StagingTargetPath: "/var/lib/dell/nfs",
				},
			},
			getCsiNFSService: func() *CsiNfsService {
				executor := mocks.NewMockExecutor(gomock.NewController(t))
				executor.EXPECT().ExecuteCommand("mkdir", "-p", gomock.Any()).Times(1).Return([]byte{}, nil)
				executor.EXPECT().ExecuteCommand("mount", "--bind", gomock.Any(), gomock.Any()).Times(1).Return(
					[]byte("mount attempted"),
					errors.New("failed to bind mount"))
				return &CsiNfsService{
					executor: executor,
				}
			},
			want:    &csi.NodePublishVolumeResponse{},
			wantErr: true,
			errMsg:  "failed to bind mount",
		},
	}
	for _, tt := range tests {
		service := tt.getCsiNFSService()
		resp, err := service.NodePublishVolume(tt.args.ctx, tt.args.req)
		if (err != nil) != tt.wantErr {
			t.Errorf("NodePublishVolume() error = %v, wantErr %v", err, tt.wantErr)
			return
		}
		if tt.wantErr {
			assert.Contains(t, err.Error(), tt.errMsg)
		}
		if !reflect.DeepEqual(resp, tt.want) {
			t.Errorf("NodePublishVolume() = %v, want %v", resp, tt.args.req)
		}
	}
}

func TestCsiNfsService_NodeUnpublishVolume(t *testing.T) {
	type fields struct {
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
	type args struct {
		ctx context.Context
		req *csi.NodeUnpublishVolumeRequest
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *csi.NodeUnpublishVolumeResponse
		wantErr bool
		errMsg  string
	}{
		{
			name: "not an NFS volume",
			fields: fields{
				failureRetries: 1,
			},
			args: args{
				ctx: context.Background(),
				req: &csi.NodeUnpublishVolumeRequest{
					VolumeId:   blockVolumeID,
					TargetPath: "path/to/target",
				},
			},
			want:    &csi.NodeUnpublishVolumeResponse{},
			wantErr: true,
			errMsg:  "nfs NodeUnpublishVolume called on non NFS volume",
		},
		{
			name: "unmount fails",
			fields: fields{
				executor: func() Executor {
					executor := mocks.NewMockExecutor(gomock.NewController(t))
					executor.EXPECT().ExecuteCommandContext(gomock.Any(), "sync", "-f", gomock.Any()).Times(1).Return([]byte("sync success"), nil)
					executor.EXPECT().ExecuteCommand("umount", "--force", gomock.Any()).Times(1).Return([]byte{}, errors.New("umount error"))
					return executor
				}(),
				failureRetries: 1,
			},
			args: args{
				ctx: context.Background(),
				req: &csi.NodeUnpublishVolumeRequest{
					VolumeId:   sharedNFSVolumeID,
					TargetPath: "path/to/target",
				},
			},
			want:    &csi.NodeUnpublishVolumeResponse{},
			wantErr: true,
			errMsg:  "umount error",
		},
		{
			name: "unmount times out",
			fields: fields{
				executor: func() Executor {
					executor := mocks.NewMockExecutor(gomock.NewController(t))
					// do nothing for longer than the context timeout
					executor.EXPECT().ExecuteCommandContext(gomock.Any(), "sync", "-f", gomock.Any()).Times(1).Do(
						func(_ context.Context, _ string, _ ...string) {
							time.Sleep(5 * time.Second)
						},
					)
					executor.EXPECT().ExecuteCommand("umount", "--force", gomock.Any()).Times(1).Return([]byte{}, nil)
					return executor
				}(),
				failureRetries: 1,
			},
			args: args{
				ctx: func() context.Context {
					ctx, cancelFn := context.WithCancel(context.Background())
					// cancel the context immediately to trigger the ctx.Done channel
					defer cancelFn()
					return ctx
				}(),
				req: &csi.NodeUnpublishVolumeRequest{
					VolumeId:   sharedNFSVolumeID,
					TargetPath: "path/to/target",
				},
			},
			want:    &csi.NodeUnpublishVolumeResponse{},
			wantErr: false,
		},
		{
			name: "success",
			fields: fields{
				executor: func() Executor {
					executor := mocks.NewMockExecutor(gomock.NewController(t))
					executor.EXPECT().ExecuteCommandContext(gomock.Any(), "sync", "-f", gomock.Any()).Times(1).Return([]byte("sync success"), nil)
					executor.EXPECT().ExecuteCommand("umount", "--force", gomock.Any()).Times(1).Return([]byte{}, nil)
					return executor
				}(),
				failureRetries: 1,
			},
			args: args{
				ctx: context.Background(),
				req: &csi.NodeUnpublishVolumeRequest{
					VolumeId:   sharedNFSVolumeID,
					TargetPath: "path/to/target",
				},
			},
			want:    &csi.NodeUnpublishVolumeResponse{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := &CsiNfsService{
				vcsi:                         tt.fields.vcsi,
				md:                           tt.fields.md,
				provisionerName:              tt.fields.provisionerName,
				mode:                         tt.fields.mode,
				nodeID:                       tt.fields.nodeID,
				nodeIPAddress:                tt.fields.nodeIPAddress,
				podCIDR:                      tt.fields.podCIDR,
				nodeName:                     tt.fields.nodeName,
				failureRetries:               tt.fields.failureRetries,
				k8sclient:                    tt.fields.k8sclient,
				executor:                     tt.fields.executor,
				waitCreateNfsServiceInterval: tt.fields.waitCreateNfsServiceInterval,
				nfsServerPort:                tt.fields.nfsServerPort,
				nfsClientServicePort:         tt.fields.nfsClientServicePort,
			}
			got, err := ns.NodeUnpublishVolume(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("CsiNfsService.NodeUnpublishVolume() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				assert.Contains(t, err.Error(), tt.errMsg)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CsiNfsService.NodeUnpublishVolume() = %v, want %v", got, tt.want)
			}
		})
	}
}
