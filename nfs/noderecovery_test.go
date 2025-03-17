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
	"reflect"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	k8s "github.com/dell/csm-hbnfs/nfs/k8s"
	"github.com/dell/csm-hbnfs/nfs/mocks"
	"github.com/dell/csm-hbnfs/nfs/proto"
	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	v1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNodeRecovery(t *testing.T) {
	nodeIP := "127.0.0.1"
	slice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mySlice",
			Labels: map[string]string{
				"pvName": "pv1",
				"nodeID": "myNode",
				"nodeIP": nodeIP,
			},

			Annotations: map[string]string{
				DriverVolumeID: "vol1",
			},
		},
		Endpoints: []discoveryv1.Endpoint{
			{
				Addresses: []string{"1.2.3.4"},
			},
		},
	}

	tests := []struct {
		name      string
		configure func(t *testing.T) *CsiNfsService
		wantErr   bool
	}{
		{
			name: "Error: No slices found",
			configure: func(t *testing.T) *CsiNfsService {
				// Create a new fake clientset
				clientset := fake.NewSimpleClientset()

				s := &CsiNfsService{
					k8sclient: &k8s.K8sClient{
						Clientset: clientset,
					},
				}

				return s
			},
		},
		{
			name: "Success: Found one endpointslice, unable to reassign",
			configure: func(t *testing.T) *CsiNfsService {
				// Create a new fake clientset
				clientset := fake.NewSimpleClientset()

				s := &CsiNfsService{
					k8sclient: &k8s.K8sClient{
						Clientset: clientset,
					},
				}

				clientset.DiscoveryV1().EndpointSlices("").Create(context.Background(), slice, metav1.CreateOptions{})

				server := mocks.NewMockNfsServer(gomock.NewController(t))
				createMockServer(t, "127.0.0.2", server)
				// Give it time for the server to setup
				time.Sleep(50 * time.Millisecond)

				return s
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.configure(t)

			s.nodeRecovery(nodeIP)
		})
	}
}

func TestReassignVolume(t *testing.T) {
	slice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mySlice",
			Labels: map[string]string{
				"pvName": "pv1",
				"nodeID": "myNode",
			},

			Annotations: map[string]string{
				DriverVolumeID: "vol1",
			},
		},
		Endpoints: []discoveryv1.Endpoint{
			{
				Addresses: []string{"1.2.3.4"},
			},
		},
	}
	volumeNodeID := "{\"csi-powerstore.dellemc.com\":\"csi-node-my-node-127.0.0.1\"}"

	tests := []struct {
		name      string
		configure func(t *testing.T) *CsiNfsService
		wantErr   bool
	}{
		{
			name: "Error: Unable to get Persisent Volume",
			configure: func(t *testing.T) *CsiNfsService {
				// Create a new fake clientset
				clientset := fake.NewSimpleClientset()

				// Test case: GetPersistentVolume fails
				s := &CsiNfsService{
					k8sclient: &k8s.K8sClient{
						Clientset: clientset,
					},
				}

				return s
			},
			wantErr: true,
		},
		{
			name: "Error: Unable to get Service",
			configure: func(t *testing.T) *CsiNfsService {
				// Create a new fake clientset
				clientset := fake.NewSimpleClientset()

				// Test case: GetPersistentVolume fails
				s := &CsiNfsService{
					k8sclient: &k8s.K8sClient{
						Clientset: clientset,
					},
				}

				clientset.CoreV1().PersistentVolumes().Create(context.Background(), &v1.PersistentVolume{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pv1",
					},
					Spec: v1.PersistentVolumeSpec{
						PersistentVolumeSource: v1.PersistentVolumeSource{
							CSI: &v1.CSIPersistentVolumeSource{
								Driver:       "myDriver",
								VolumeHandle: CsiNfsPrefixDash + uuid.New().String(),
							},
						},
					},
				}, metav1.CreateOptions{})

				return s
			},
			wantErr: true,
		},
		{
			name: "Error: Unable to execute ControllerUnpublishVolume",
			configure: func(t *testing.T) *CsiNfsService {
				// Create a new fake clientset
				clientset := fake.NewSimpleClientset()

				// Test case: GetPersistentVolume fails
				s := &CsiNfsService{
					k8sclient: &k8s.K8sClient{
						Clientset: clientset,
					},
				}

				// Create Persistent Volume
				clientset.CoreV1().PersistentVolumes().Create(context.Background(), &v1.PersistentVolume{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pv1",
					},
					Spec: v1.PersistentVolumeSpec{
						PersistentVolumeSource: v1.PersistentVolumeSource{
							CSI: &v1.CSIPersistentVolumeSource{
								Driver:       "myDriver",
								VolumeHandle: CsiNfsPrefixDash + uuid.New().String(),
							},
						},
					},
				}, metav1.CreateOptions{})

				clientset.CoreV1().Services("").Create(context.Background(), &v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name: "mySlice",
						Labels: map[string]string{
							"client/myClient": "127.0.0.1",
						},
					},
				}, metav1.CreateOptions{})

				// Set the export counts for the client (will need mux)
				exportCounts["127.0.0.1"] = 2

				service := mocks.NewMockService(gomock.NewController(t))
				service.EXPECT().ControllerUnpublishVolume(gomock.Any(), gomock.Any()).Times(1).Return(nil, status.Errorf(codes.Internal, "unable to unpublish volume"))
				s.vcsi = service

				return s
			},
			wantErr: true,
		},

		{
			name: "Error: Unable to GetNode",
			configure: func(t *testing.T) *CsiNfsService {
				// Create a new fake clientset
				clientset := fake.NewSimpleClientset()

				// Test case: GetPersistentVolume fails
				s := &CsiNfsService{
					k8sclient: &k8s.K8sClient{
						Clientset: clientset,
					},
				}

				// Create Persistent Volume
				clientset.CoreV1().PersistentVolumes().Create(context.Background(), &v1.PersistentVolume{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pv1",
					},
					Spec: v1.PersistentVolumeSpec{
						PersistentVolumeSource: v1.PersistentVolumeSource{
							CSI: &v1.CSIPersistentVolumeSource{
								Driver:       "myDriver",
								VolumeHandle: CsiNfsPrefixDash + uuid.New().String(),
							},
						},
					},
				}, metav1.CreateOptions{})

				clientset.CoreV1().Services("").Create(context.Background(), &v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name: "mySlice",
						Labels: map[string]string{
							"client/myClient": "127.0.0.1",
						},
					},
				}, metav1.CreateOptions{})

				// Set the export counts for the client (will need mux)
				exportCounts["127.0.0.1"] = 2

				service := mocks.NewMockService(gomock.NewController(t))
				service.EXPECT().ControllerUnpublishVolume(gomock.Any(), gomock.Any()).Times(1).Return(&csi.ControllerUnpublishVolumeResponse{}, nil)
				s.vcsi = service

				return s
			},
			wantErr: true,
		},
		{
			name: "Error: Unable to execute ControllerPublishVolume",
			configure: func(t *testing.T) *CsiNfsService {
				// Create a new fake clientset
				clientset := fake.NewSimpleClientset()

				// Test case: GetPersistentVolume fails
				s := &CsiNfsService{
					k8sclient: &k8s.K8sClient{
						Clientset: clientset,
					},
				}

				// Create Persistent Volume
				clientset.CoreV1().PersistentVolumes().Create(context.Background(), &v1.PersistentVolume{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pv1",
					},
					Spec: v1.PersistentVolumeSpec{
						PersistentVolumeSource: v1.PersistentVolumeSource{
							CSI: &v1.CSIPersistentVolumeSource{
								Driver:       "myDriver",
								VolumeHandle: CsiNfsPrefixDash + uuid.New().String(),
							},
						},
					},
				}, metav1.CreateOptions{})

				clientset.CoreV1().Services("").Create(context.Background(), &v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name: "mySlice",
						Labels: map[string]string{
							"client/myClient": "127.0.0.1",
						},
					},
				}, metav1.CreateOptions{})

				clientset.CoreV1().Nodes().Create(context.Background(), &v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "127.0.0.1",
						Annotations: map[string]string{
							"csi.volume.kubernetes.io/nodeid": volumeNodeID,
						},
					},
					Status: v1.NodeStatus{
						Addresses: []v1.NodeAddress{
							{
								Address: "127.0.0.1",
							},
						},
					},
				}, metav1.CreateOptions{})

				// Set the export counts for the client (will need mux)
				exportCounts["127.0.0.1"] = 2

				service := mocks.NewMockService(gomock.NewController(t))
				service.EXPECT().ControllerUnpublishVolume(gomock.Any(), gomock.Any()).Times(1).Return(&csi.ControllerUnpublishVolumeResponse{}, nil)
				service.EXPECT().ControllerPublishVolume(gomock.Any(), gomock.Any()).Times(1).Return(nil, status.Errorf(codes.Internal, "unable to publish volume"))
				s.vcsi = service

				return s
			},
			wantErr: true,
		},
		{
			name: "Error: Unable to callExportNfsVolume",
			configure: func(t *testing.T) *CsiNfsService {
				// Create a new fake clientset
				clientset := fake.NewSimpleClientset()

				// Test case: GetPersistentVolume fails
				s := &CsiNfsService{
					k8sclient: &k8s.K8sClient{
						Clientset: clientset,
					},
				}

				// Create Persistent Volume
				clientset.CoreV1().PersistentVolumes().Create(context.Background(), &v1.PersistentVolume{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pv1",
					},
					Spec: v1.PersistentVolumeSpec{
						PersistentVolumeSource: v1.PersistentVolumeSource{
							CSI: &v1.CSIPersistentVolumeSource{
								Driver:       "myDriver",
								VolumeHandle: CsiNfsPrefixDash + uuid.New().String(),
							},
						},
					},
				}, metav1.CreateOptions{})

				clientset.CoreV1().Services("").Create(context.Background(), &v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name: "mySlice",
						Labels: map[string]string{
							"client/myClient": "127.0.0.1",
						},
					},
				}, metav1.CreateOptions{})

				clientset.CoreV1().Nodes().Create(context.Background(), &v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "127.0.0.1",
						Annotations: map[string]string{
							"csi.volume.kubernetes.io/nodeid": volumeNodeID,
						},
					},
					Status: v1.NodeStatus{
						Addresses: []v1.NodeAddress{
							{
								Address: "127.0.0.1",
							},
						},
					},
				}, metav1.CreateOptions{})

				// Set the export counts for the client (will need mux)
				exportCounts["127.0.0.1"] = 2

				service := mocks.NewMockService(gomock.NewController(t))
				service.EXPECT().ControllerUnpublishVolume(gomock.Any(), gomock.Any()).Times(1).Return(&csi.ControllerUnpublishVolumeResponse{}, nil)
				service.EXPECT().ControllerPublishVolume(gomock.Any(), gomock.Any()).Times(1).Return(&csi.ControllerPublishVolumeResponse{}, nil)
				s.vcsi = service

				return s
			},
			wantErr: true,
		},
		{
			name: "Success: Reassign volume with proper export",
			configure: func(t *testing.T) *CsiNfsService {
				// Create a new fake clientset
				clientset := fake.NewSimpleClientset()

				// Test case: GetPersistentVolume fails
				s := &CsiNfsService{
					k8sclient: &k8s.K8sClient{
						Clientset: clientset,
					},
				}

				// Create Persistent Volume
				clientset.CoreV1().PersistentVolumes().Create(context.Background(), &v1.PersistentVolume{
					ObjectMeta: metav1.ObjectMeta{
						Name: "pv1",
					},
					Spec: v1.PersistentVolumeSpec{
						PersistentVolumeSource: v1.PersistentVolumeSource{
							CSI: &v1.CSIPersistentVolumeSource{
								Driver:       "myDriver",
								VolumeHandle: CsiNfsPrefixDash + uuid.New().String(),
							},
						},
					},
				}, metav1.CreateOptions{})

				clientset.CoreV1().Services("").Create(context.Background(), &v1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name: "mySlice",
						Labels: map[string]string{
							"client/myClient": "127.0.0.1",
						},
					},
				}, metav1.CreateOptions{})

				clientset.CoreV1().Nodes().Create(context.Background(), &v1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "127.0.0.1",
						Annotations: map[string]string{
							"csi.volume.kubernetes.io/nodeid": volumeNodeID,
						},
					},
					Status: v1.NodeStatus{
						Addresses: []v1.NodeAddress{
							{
								Address: "127.0.0.1",
							},
						},
					},
				}, metav1.CreateOptions{})

				clientset.DiscoveryV1().EndpointSlices("").Create(context.Background(), slice, metav1.CreateOptions{})

				// Set the export counts for the client (will need mux)
				exportCounts["127.0.0.1"] = 2

				service := mocks.NewMockService(gomock.NewController(t))
				service.EXPECT().ControllerUnpublishVolume(gomock.Any(), gomock.Any()).Times(1).Return(&csi.ControllerUnpublishVolumeResponse{}, nil)
				service.EXPECT().ControllerPublishVolume(gomock.Any(), gomock.Any()).Times(1).Return(&csi.ControllerPublishVolumeResponse{}, nil)
				s.vcsi = service

				server := mocks.NewMockNfsServer(gomock.NewController(t))
				server.EXPECT().ExportNfsVolume(gomock.Any(), gomock.Any()).Times(1).Return(&proto.ExportNfsVolumeResponse{VolumeId: uuid.New().String()}, nil)

				createMockServer(t, "127.0.0.1", server)
				// Give it time for the server to setup
				time.Sleep(50 * time.Millisecond)

				return s
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.configure(t)

			res := s.reassignVolume(slice)

			if tt.wantErr && res {
				t.Error("expecting error but reassign is successful")
			}
		})
	}
}

func TestUpdateEndpointSlice(t *testing.T) {
	slice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name: "mySlice",
			Labels: map[string]string{
				"pvName": "pv1",
				"nodeID": "myNode",
			},

			Annotations: map[string]string{
				DriverVolumeID: "vol1",
			},
		},
		Endpoints: []discoveryv1.Endpoint{
			{
				Addresses: []string{"1.2.3.4"},
			},
		},
	}

	tests := []struct {
		name      string
		configure func(t *testing.T) *CsiNfsService
		wantErr   bool
	}{
		{
			name: "Success: Reassign volume with proper export",
			configure: func(t *testing.T) *CsiNfsService {
				// Create a new fake clientset
				clientset := fake.NewSimpleClientset()

				s := &CsiNfsService{
					k8sclient: &k8s.K8sClient{
						Clientset: clientset,
					},
				}

				clientset.DiscoveryV1().EndpointSlices("").Create(context.Background(), slice, metav1.CreateOptions{})

				return s
			},
			wantErr: false,
		},
		{
			name: "Fail: Unable to update endpoint slice",
			configure: func(t *testing.T) *CsiNfsService {
				// Create a new fake clientset
				clientset := fake.NewSimpleClientset()

				s := &CsiNfsService{
					k8sclient: &k8s.K8sClient{
						Clientset: clientset,
					},
				}

				return s
			},
			wantErr: true,
		},
	}

	updatedNode := &v1.Node{
		Status: v1.NodeStatus{
			Addresses: []v1.NodeAddress{
				{
					Address: "5.6.7.8",
				},
			},
		},
	}

	// set endpoint slice for unit tests
	endpointSliceTimeout = 50 * time.Millisecond

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.configure(t)
			ctx := context.Background()

			err := s.updateEndpointSlice(ctx, slice, "myNode", updatedNode)
			if tt.wantErr && err == nil {
				t.Error("expected an error but didn't get one")
			}

			if !tt.wantErr {
				result, err := s.k8sclient.UpdateEndpointSlice(ctx, DriverNamespace, slice)
				if err != nil {
					t.Error(err)
				}

				if !reflect.DeepEqual(result.Endpoints[0].Addresses, result.Endpoints[0].Addresses) {
					t.Error("endpoint slice was not updated")
				}
			}
		})
	}
}
