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
	k8s "github.com/dell/csm-sharednfs/nfs/k8s"
	"github.com/dell/csm-sharednfs/nfs/mocks"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNodeStageVolume(t *testing.T) {
	type args struct {
		ctx context.Context
		req *csi.NodeStageVolumeRequest
	}
	tests := []struct {
		name    string
		args    args
		want    *csi.NodeStageVolumeResponse
		wantErr bool
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
			},
			want:    &csi.NodeStageVolumeResponse{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &CsiNfsService{}
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

func TestNodeUnStageVolume(t *testing.T) {
	type args struct {
		ctx context.Context
		req *csi.NodeUnstageVolumeRequest
	}
	tests := []struct {
		name    string
		args    args
		want    *csi.NodeUnstageVolumeResponse
		wantErr bool
	}{
		{
			name: "success",
			args: args{
				req: &csi.NodeUnstageVolumeRequest{
					VolumeId:          "vol1",
					StagingTargetPath: "path/to/stage",
				},
			},
			want:    &csi.NodeUnstageVolumeResponse{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &CsiNfsService{}
			got, err := service.NodeUnstageVolume(tt.args.ctx, tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("NodeUnstageVolume() error = %v, wantErr %v", err, tt.wantErr)
				return
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
					VolumeId:          "vol1",
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
	nodePublishTimeout = 100 * time.Millisecond

	cxt := context.Background()
	t.Run("no service", func(t *testing.T) {
		service := &CsiNfsService{
			k8sclient: &k8s.Client{
				Clientset: fake.NewSimpleClientset(),
			},
			failureRetries: 1,
		}
		_, err := service.NodePublishVolume(cxt, &csi.NodePublishVolumeRequest{})
		assert.Contains(t, err.Error(), "err")
	})

	t.Run("cluster IP empty", func(t *testing.T) {
		clientset := fake.NewClientset()
		clientset.CoreV1().Services("").Create(cxt, &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vol1",
			},
			Spec: v1.ServiceSpec{
				ClusterIP: "",
			},
			Status: v1.ServiceStatus{},
		}, metav1.CreateOptions{})
		service := &CsiNfsService{
			k8sclient: &k8s.Client{
				Clientset: clientset,
			},
			failureRetries: 1,
		}
		_, err := service.NodePublishVolume(cxt, &csi.NodePublishVolumeRequest{
			VolumeId: "vol1",
		})
		assert.Contains(t, err.Error(), "service IP empty")
	})

	t.Run("TargetPath empty", func(t *testing.T) {
		clientset := fake.NewClientset()
		clientset.CoreV1().Services("").Create(cxt, &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vol1",
			},
			Spec: v1.ServiceSpec{
				ClusterIP: "1.1.1.1",
			},
			Status: v1.ServiceStatus{},
		}, metav1.CreateOptions{})
		service := &CsiNfsService{
			k8sclient: &k8s.Client{
				Clientset: clientset,
			},
			failureRetries: 1,
		}
		_, err := service.NodePublishVolume(cxt, &csi.NodePublishVolumeRequest{
			VolumeId: "vol1",
		})
		assert.Contains(t, err.Error(), "TargetPath empty")
	})

	t.Run("fail mount", func(t *testing.T) {
		clientset := fake.NewClientset()
		clientset.CoreV1().Services("").Create(cxt, &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vol1",
			},
			Spec: v1.ServiceSpec{
				ClusterIP: "1.1.1.1",
			},
			Status: v1.ServiceStatus{},
		}, metav1.CreateOptions{})
		executor := mocks.NewMockExecutor(gomock.NewController(t))
		executor.EXPECT().ExecuteCommand("mkdir", "-p", gomock.Any()).AnyTimes().Return([]byte{}, errors.New("mkdir error"))
		executor.EXPECT().ExecuteCommand("umount", gomock.Any()).AnyTimes().Return([]byte{}, errors.New("mkdir error"))
		executor.EXPECT().ExecuteCommand("chmod", "02777", gomock.Any()).AnyTimes().Return([]byte{}, errors.New("chmod error"))
		executor.EXPECT().ExecuteCommandContext(gomock.Any(), "mount", "-w", "-t", "nfs4", gomock.Any(), gomock.Any()).AnyTimes().Return([]byte{}, errors.New("mount error"))
		service := &CsiNfsService{
			k8sclient: &k8s.Client{
				Clientset: clientset,
			},
			executor:       executor,
			failureRetries: 1,
		}
		_, err := service.NodePublishVolume(cxt, &csi.NodePublishVolumeRequest{
			VolumeId:   "vol1",
			TargetPath: "/test/path",
		})
		assert.Contains(t, err.Error(), "mount error")
	})

	t.Run("success", func(t *testing.T) {
		clientset := fake.NewClientset()
		clientset.CoreV1().Services("").Create(cxt, &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: "vol1",
			},
			Spec: v1.ServiceSpec{
				ClusterIP: "1.1.1.1",
			},
			Status: v1.ServiceStatus{},
		}, metav1.CreateOptions{})
		executor := mocks.NewMockExecutor(gomock.NewController(t))
		executor.EXPECT().ExecuteCommand("mkdir", "-p", gomock.Any()).Times(1).Return([]byte{}, nil)
		executor.EXPECT().ExecuteCommand("chmod", "02777", gomock.Any()).Times(1).Return([]byte{}, nil)
		executor.EXPECT().ExecuteCommandContext(gomock.Any(), "mount", "-w", "-t", "nfs4", gomock.Any(), gomock.Any()).Times(1).Return([]byte{}, nil)
		service := &CsiNfsService{
			k8sclient: &k8s.Client{
				Clientset: clientset,
			},
			executor:       executor,
			failureRetries: 1,
		}
		resp, err := service.NodePublishVolume(cxt, &csi.NodePublishVolumeRequest{
			VolumeId:   "vol1",
			TargetPath: "/test/path",
		})
		assert.Nil(t, err)
		assert.Equal(t, resp, &csi.NodePublishVolumeResponse{})
	})
}

func TestNodeUnpublishVolume(t *testing.T) {
	cxt := context.Background()
	t.Run("not nfs volume", func(t *testing.T) {
		req := &csi.NodeUnpublishVolumeRequest{
			VolumeId:   "vol1",
			TargetPath: "path/to/target",
		}
		service := &CsiNfsService{
			failureRetries: 1,
		}
		got, err := service.NodeUnpublishVolume(cxt, req)
		assert.Contains(t, err.Error(), "non NFS volume")
		assert.Equal(t, got, &csi.NodeUnpublishVolumeResponse{})
	})

	t.Run("unmount fail", func(t *testing.T) {
		executor := mocks.NewMockExecutor(gomock.NewController(t))
		executor.EXPECT().ExecuteCommand("umount", "--force", gomock.Any()).Times(1).Return([]byte{}, errors.New("umount error"))
		service := &CsiNfsService{
			executor:       executor,
			failureRetries: 1,
		}
		_, err := service.NodeUnpublishVolume(cxt, &csi.NodeUnpublishVolumeRequest{
			VolumeId:   "nfs-vol1",
			TargetPath: "/test/path",
		})
		assert.Contains(t, err.Error(), "umount error")
	})

	t.Run("success", func(t *testing.T) {
		executor := mocks.NewMockExecutor(gomock.NewController(t))
		executor.EXPECT().ExecuteCommand("umount", "--force", gomock.Any()).Times(1).Return([]byte{}, nil)
		service := &CsiNfsService{
			executor:       executor,
			failureRetries: 1,
		}
		resp, err := service.NodeUnpublishVolume(cxt, &csi.NodeUnpublishVolumeRequest{
			VolumeId:   "nfs-vol1",
			TargetPath: "/test/path",
		})
		assert.Nil(t, err)
		assert.Equal(t, resp, &csi.NodeUnpublishVolumeResponse{})
	})
}
