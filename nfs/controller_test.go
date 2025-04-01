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
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"math/big"
	reflect "reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	k8s "github.com/dell/csm-hbnfs/nfs/k8s"
	"github.com/dell/csm-hbnfs/nfs/mocks"
	"github.com/dell/csm-hbnfs/nfs/proto"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clientgotesting "k8s.io/client-go/testing"
)

func TestCreateVolume(t *testing.T) {
	tests := []struct {
		name          string
		csiNfsService *CsiNfsService
		req           *csi.CreateVolumeRequest
		expectedRes   *csi.CreateVolumeResponse
		expectedErr   error
	}{
		{
			name: "Valid volume request",
			csiNfsService: func() *CsiNfsService {
				mockService := mocks.NewMockService(gomock.NewController(t))
				mockService.EXPECT().CreateVolume(gomock.Any(), gomock.Any()).Times(1).Return(&csi.CreateVolumeResponse{
					Volume: &csi.Volume{
						VolumeId: "123",
					},
				}, nil)
				csiNfsServce := &CsiNfsService{
					vcsi: mockService,
				}
				return csiNfsServce
			}(),
			req: &csi.CreateVolumeRequest{
				Name: "test-volume",
				VolumeCapabilities: []*csi.VolumeCapability{
					{
						AccessType: &csi.VolumeCapability_Block{
							Block: &csi.VolumeCapability_BlockVolume{},
						},
						AccessMode: &csi.VolumeCapability_AccessMode{
							Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
						},
					},
				},
			},
			expectedRes: &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					VolumeId: "nfs-123",
				},
			},
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resp, err := test.csiNfsService.CreateVolume(context.Background(), test.req)
			if !reflect.DeepEqual(resp, test.expectedRes) {
				t.Errorf("expected response %+v, got %+v", test.expectedRes, resp)
			}
			assert.Equal(t, test.expectedErr, err)
		})
	}
}

func TestDeleteVolume(t *testing.T) {
	tests := []struct {
		name        string
		req         *csi.DeleteVolumeRequest
		expectedRes *csi.DeleteVolumeResponse
		expectedErr error
	}{
		{
			name: "Valid volume request",
			req: &csi.DeleteVolumeRequest{
				VolumeId: "test-volume",
			},
			expectedRes: &csi.DeleteVolumeResponse{},
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cs := &CsiNfsService{}
			resp, err := cs.DeleteVolume(context.Background(), test.req)
			if !reflect.DeepEqual(resp, test.expectedRes) {
				t.Errorf("expected response %+v, got %+v", test.expectedRes, resp)
			}
			assert.Equal(t, test.expectedErr, err)
		})
	}
}

func TestLockPV(t *testing.T) {
	tests := []struct {
		name         string
		pvName       string
		init         func()
		requestID    string
		expectedLog  string
		highPriority bool
	}{
		{
			name:         "Acquire lock normal priority",
			pvName:       "test-pv",
			requestID:    "test-request",
			highPriority: false,
		},
		{
			name:         "Acquire lock high priority",
			pvName:       "test-pv",
			requestID:    "test-request",
			highPriority: true,
		},
		{
			name:      "Acquire lock high priority and wait",
			pvName:    "test-pv",
			requestID: "test-request",
			init: func() {
				PVLock.Store("test-pv", "test-request")
				defer PVLock.Clear()
				time.Sleep(2 * time.Second)
			},
			highPriority: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Set up the test
			buf := new(bytes.Buffer)
			log.SetOutput(buf)

			// Call the function
			cs := &CsiNfsService{}

			if test.init != nil {
				// Start init in a new goroutine, this will simulate a store and then clear the lock to be acquired again
				go test.init()
			}
			time.Sleep(1 * time.Second)
			cs.LockPV(test.pvName, test.requestID, test.highPriority)

			defer PVLock.Clear()

			// Check the output
			if test.expectedLog != "" {
				if !strings.Contains(buf.String(), test.expectedLog) {
					t.Errorf("expected log %q, got %q", test.expectedLog, buf.String())
				}
			} else {
				if buf.String() != "" {
					t.Errorf("expected no log, got %q", buf.String())
				}
			}
		})
	}
}

func TestUnlockPV(t *testing.T) {
	tests := []struct {
		name   string
		pvName string
	}{
		{
			name:   "Acquire lock",
			pvName: "test-pv",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cs := &CsiNfsService{}

			PVLock.Store(test.pvName, "")
			defer PVLock.Clear()

			cs.UnlockPV(test.pvName)

			// Don't expect to find the value
			_, ok := PVLock.Load(test.pvName)
			if ok {
				t.Errorf("expected PVLock to not contain value for key %s, but it was not found", test.pvName)
			}
		})
	}
}

func TestControllerPublishVolume(t *testing.T) {
	port := func() string {
		nBig, _ := rand.Int(rand.Reader, big.NewInt(9000))
		return strconv.Itoa(int(nBig.Int64() + 1000))
	}()
	tests := []struct {
		name          string
		csiNfsService *CsiNfsService
		req           *csi.ControllerPublishVolumeRequest
		expectedRes   *csi.ControllerPublishVolumeResponse
		expectedErr   error
		createServer  func(*testing.T)
		port          func()
	}{
		{
			name: "Valid volume request",
			createServer: func(t *testing.T) {
				server := mocks.NewMockNfsServer(gomock.NewController(t))
				server.EXPECT().ExportNfsVolume(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.ExportNfsVolumeResponse{}, nil)
				createMockServer(t, "127.0.0.1", port, server)
				nodeIPAddress["127.0.0.1"] = &NodeStatus{
					online:     true,
					inRecovery: false,
				}
			},
			csiNfsService: func() *CsiNfsService {
				mockService := mocks.NewMockService(gomock.NewController(t))
				mockService.EXPECT().ControllerPublishVolume(gomock.Any(), gomock.Any()).AnyTimes().Return(&csi.ControllerPublishVolumeResponse{
					PublishContext: map[string]string{
						"csi-nfs": "test-node",
					},
				}, nil)
				fakeK8sClient := fake.NewSimpleClientset()

				fakeK8sClient.AddReactor("get", "services", func(_ clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, nil
				})

				fakeK8sClient.AddReactor("get", "endpointslices", func(_ clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, nil
				})

				fakeK8sClient.PrependReactor("list", "nodes", func(_ clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, &v1.NodeList{
						Items: []v1.Node{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name: "worker-node-1",
									Annotations: map[string]string{
										"csi.volume.kubernetes.io/nodeid": "{\"csi-powerstore.dellemc.com\":\"csi-node-123-127.0.0.1\"}",
									},
								},
								Status: v1.NodeStatus{
									Addresses: []v1.NodeAddress{
										{
											Address: "127.0.0.1",
										},
									},
								},
							},
						},
					}, nil
				})

				csiNfsServce := &CsiNfsService{
					vcsi: mockService,
					k8sclient: &k8s.Client{
						Clientset: fakeK8sClient,
					},
					waitCreateNfsServiceInterval: 10 * time.Millisecond,
					nfsClientServicePort:         port,
				}
				return csiNfsServce
			}(),
			req: &csi.ControllerPublishVolumeRequest{
				VolumeId: "test-volume",
				NodeId:   "csi-node-123-127.0.0.1",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Block{
						Block: &csi.VolumeCapability_BlockVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
					},
				},
				VolumeContext: map[string]string{
					"Name": "volume-name",
				},
			},
			expectedRes: &csi.ControllerPublishVolumeResponse{
				PublishContext: map[string]string{
					"name": "volume-name",
					"nfs":  "test-volume",
				},
			},
			expectedErr: nil,
		},
		{
			name: "Error calling ControllerPublishVolume on vcsi",
			createServer: func(t *testing.T) {
				server := mocks.NewMockNfsServer(gomock.NewController(t))
				server.EXPECT().ExportNfsVolume(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.ExportNfsVolumeResponse{}, nil)
				createMockServer(t, "127.0.0.1", port, server)
				nodeIPAddress["127.0.0.1"] = &NodeStatus{
					online:     true,
					inRecovery: false,
				}
			},
			csiNfsService: func() *CsiNfsService {
				mockService := mocks.NewMockService(gomock.NewController(t))
				mockService.EXPECT().ControllerPublishVolume(gomock.Any(), gomock.Any()).AnyTimes().Return(nil, errors.New("error calling ControllerPublishVolume on vcsi"))
				fakeK8sClient := fake.NewSimpleClientset()

				fakeK8sClient.PrependReactor("list", "nodes", func(_ clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, &v1.NodeList{
						Items: []v1.Node{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name: "worker-node-1",
									Annotations: map[string]string{
										"csi.volume.kubernetes.io/nodeid": "{\"csi-powerstore.dellemc.com\":\"csi-node-123-127.0.0.1\"}",
									},
								},
								Status: v1.NodeStatus{
									Addresses: []v1.NodeAddress{
										{
											Address: "127.0.0.1",
										},
									},
								},
							},
						},
					}, nil
				})

				csiNfsService := &CsiNfsService{
					vcsi: mockService,
					k8sclient: &k8s.Client{
						Clientset: fakeK8sClient,
					},
					waitCreateNfsServiceInterval: 10 * time.Millisecond,
					nfsClientServicePort:         port,
				}
				return csiNfsService
			}(),
			req: &csi.ControllerPublishVolumeRequest{
				VolumeId: "test-volume",
				NodeId:   "csi-node-123-127.0.0.1",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Block{
						Block: &csi.VolumeCapability_BlockVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
					},
				},
				VolumeContext: map[string]string{
					"Name": "volume-name",
				},
			},
			expectedRes: &csi.ControllerPublishVolumeResponse{
				PublishContext: map[string]string{
					"name": "volume-name",
					"nfs":  "test-volume",
				},
			},
			expectedErr: errors.New("error calling ControllerPublishVolume on vcsi"),
		},
		{
			name: "Error calling CreateEndpointSlice",
			createServer: func(t *testing.T) {
				server := mocks.NewMockNfsServer(gomock.NewController(t))
				server.EXPECT().ExportNfsVolume(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.ExportNfsVolumeResponse{}, nil)
				createMockServer(t, "127.0.0.1", port, server)
				nodeIPAddress["127.0.0.1"] = &NodeStatus{
					online:     true,
					inRecovery: false,
				}
			},
			csiNfsService: func() *CsiNfsService {
				mockService := mocks.NewMockService(gomock.NewController(t))
				mockService.EXPECT().ControllerPublishVolume(gomock.Any(), gomock.Any()).AnyTimes().Return(&csi.ControllerPublishVolumeResponse{
					PublishContext: map[string]string{
						"csi-nfs": "test-node",
					},
				}, nil)
				fakeK8sClient := fake.NewSimpleClientset()

				fakeK8sClient.PrependReactor("create", "endpointslices", func(_ clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, errors.New("error creating EndpointSlice")
				})

				fakeK8sClient.PrependReactor("list", "nodes", func(_ clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, &v1.NodeList{
						Items: []v1.Node{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name: "worker-node-1",
									Annotations: map[string]string{
										"csi.volume.kubernetes.io/nodeid": "{\"csi-powerstore.dellemc.com\":\"csi-node-123-127.0.0.1\"}",
									},
								},
								Status: v1.NodeStatus{
									Addresses: []v1.NodeAddress{
										{
											Address: "127.0.0.1",
										},
									},
								},
							},
						},
					}, nil
				})

				csiNfsService := &CsiNfsService{
					vcsi: mockService,
					k8sclient: &k8s.Client{
						Clientset: fakeK8sClient,
					},
					waitCreateNfsServiceInterval: 10 * time.Millisecond,
					nfsClientServicePort:         port,
				}

				return csiNfsService
			}(),
			req: &csi.ControllerPublishVolumeRequest{
				VolumeId: "test-volume",
				NodeId:   "csi-node-123-127.0.0.1",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Block{
						Block: &csi.VolumeCapability_BlockVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
					},
				},
				VolumeContext: map[string]string{
					"Name": "volume-name",
				},
			},
			expectedRes: &csi.ControllerPublishVolumeResponse{
				PublishContext: map[string]string{
					"name": "volume-name",
					"nfs":  "test-volume",
				},
			},
			expectedErr: errors.New("error creating EndpointSlice"),
		},
		{
			name: "could not find node",
			createServer: func(t *testing.T) {
				server := mocks.NewMockNfsServer(gomock.NewController(t))
				server.EXPECT().ExportNfsVolume(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.ExportNfsVolumeResponse{}, nil)
				createMockServer(t, "127.0.0.1", port, server)
				nodeIPAddress["127.0.0.1"] = &NodeStatus{
					online:     true,
					inRecovery: false,
				}
			},
			csiNfsService: func() *CsiNfsService {
				mockService := mocks.NewMockService(gomock.NewController(t))
				mockService.EXPECT().ControllerPublishVolume(gomock.Any(), gomock.Any()).AnyTimes().Return(&csi.ControllerPublishVolumeResponse{
					PublishContext: map[string]string{
						"csi-nfs": "test-node",
					},
				}, nil)
				fakeK8sClient := fake.NewSimpleClientset()

				fakeK8sClient.AddReactor("get", "services", func(_ clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, nil
				})

				fakeK8sClient.AddReactor("get", "endpointslices", func(_ clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, nil
				})

				fakeK8sClient.PrependReactor("list", "nodes", func(_ clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, nil
				})

				csiNfsService := &CsiNfsService{
					vcsi: mockService,
					k8sclient: &k8s.Client{
						Clientset: fakeK8sClient,
					},
					waitCreateNfsServiceInterval: 10 * time.Millisecond,
					nfsClientServicePort:         port,
				}
				return csiNfsService
			}(),
			req: &csi.ControllerPublishVolumeRequest{
				VolumeId: "test-volume",
				NodeId:   "csi-node-123-127.0.0.1",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Block{
						Block: &csi.VolumeCapability_BlockVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
					},
				},
				VolumeContext: map[string]string{
					"Name": "volume-name",
				},
			},
			expectedRes: nil,
			expectedErr: status.Errorf(codes.Internal, "could not retrieve Node"),
		},
		{
			name: "Publish volume request when endpoint exists",
			createServer: func(t *testing.T) {
				server := mocks.NewMockNfsServer(gomock.NewController(t))
				server.EXPECT().ExportNfsVolume(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.ExportNfsVolumeResponse{}, nil)
				createMockServer(t, "127.0.0.1", port, server)
				nodeIPAddress["127.0.0.1"] = &NodeStatus{
					online:     true,
					inRecovery: false,
				}
			},
			csiNfsService: func() *CsiNfsService {
				mockService := mocks.NewMockService(gomock.NewController(t))
				mockService.EXPECT().ControllerPublishVolume(gomock.Any(), gomock.Any()).AnyTimes().Return(&csi.ControllerPublishVolumeResponse{
					PublishContext: map[string]string{
						"csi-nfs": "test-node",
					},
				}, nil)
				fakeK8sClient := fake.NewSimpleClientset()

				fakeK8sClient.AddReactor("get", "services", func(_ clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, nil, nil
				})

				fakeK8sClient.AddReactor("get", "endpointslices", func(_ clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, &discoveryv1.EndpointSlice{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-volume",
								Namespace: "nfs",
							},
							Endpoints: []discoveryv1.Endpoint{
								{
									Addresses: []string{"127.0.0.1"},
								},
							},
						},
						nil
				})

				fakeK8sClient.PrependReactor("list", "nodes", func(_ clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, &v1.NodeList{
						Items: []v1.Node{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name: "worker-node-1",
									Annotations: map[string]string{
										"csi.volume.kubernetes.io/nodeid": "{\"csi-powerstore.dellemc.com\":\"csi-node-123-127.0.0.1\"}",
									},
								},
								Status: v1.NodeStatus{
									Addresses: []v1.NodeAddress{
										{
											Address: "127.0.0.1",
										},
									},
								},
							},
						},
					}, nil
				})

				csiNfsService := &CsiNfsService{
					vcsi: mockService,
					k8sclient: &k8s.Client{
						Clientset: fakeK8sClient,
					},
					waitCreateNfsServiceInterval: 10 * time.Millisecond,
					nfsClientServicePort:         port,
				}
				return csiNfsService
			}(),
			req: &csi.ControllerPublishVolumeRequest{
				VolumeId: "test-volume",
				NodeId:   "csi-node-123-127.0.0.1",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Block{
						Block: &csi.VolumeCapability_BlockVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
					},
				},
				VolumeContext: map[string]string{
					"Name": "volume-name",
				},
			},
			expectedRes: &csi.ControllerPublishVolumeResponse{
				PublishContext: map[string]string{
					"name": "volume-name",
					"nfs":  "test-volume",
				},
			},
			expectedErr: nil,
		},
		{
			name: "Publish volume request when endpoint and service exists",
			createServer: func(t *testing.T) {
				server := mocks.NewMockNfsServer(gomock.NewController(t))
				server.EXPECT().ExportNfsVolume(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.ExportNfsVolumeResponse{}, nil)
				createMockServer(t, "127.0.0.1", port, server)
				nodeIPAddress["127.0.0.1"] = &NodeStatus{
					online:     true,
					inRecovery: false,
				}
			},
			csiNfsService: func() *CsiNfsService {
				mockService := mocks.NewMockService(gomock.NewController(t))
				mockService.EXPECT().ControllerPublishVolume(gomock.Any(), gomock.Any()).AnyTimes().Return(&csi.ControllerPublishVolumeResponse{
					PublishContext: map[string]string{
						"csi-nfs": "test-node",
					},
				}, nil)
				fakeK8sClient := fake.NewSimpleClientset()

				fakeK8sClient.PrependReactor("get", "services", func(_ clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, &v1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-volume",
							Namespace: "nfs",
							Labels: map[string]string{
								"client/" + "csi-node-123-127.0.0.1": "csi-node-123-127.0.0.1",
							},
						},
					}, nil
				})

				fakeK8sClient.PrependReactor("get", "endpointslices", func(_ clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, &discoveryv1.EndpointSlice{
							ObjectMeta: metav1.ObjectMeta{
								Name:      "test-volume",
								Namespace: "nfs",
							},
							Endpoints: []discoveryv1.Endpoint{
								{
									Addresses: []string{"127.0.0.1"},
								},
							},
						},
						nil
				})

				fakeK8sClient.PrependReactor("list", "nodes", func(_ clientgotesting.Action) (handled bool, ret runtime.Object, err error) {
					return true, &v1.NodeList{
						Items: []v1.Node{
							{
								ObjectMeta: metav1.ObjectMeta{
									Name: "worker-node-1",
									Annotations: map[string]string{
										"csi.volume.kubernetes.io/nodeid": "{\"csi-powerstore.dellemc.com\":\"csi-node-123-127.0.0.1\"}",
									},
								},
								Status: v1.NodeStatus{
									Addresses: []v1.NodeAddress{
										{
											Address: "127.0.0.1",
										},
									},
								},
							},
						},
					}, nil
				})

				csiNfsService := &CsiNfsService{
					vcsi: mockService,
					k8sclient: &k8s.Client{
						Clientset: fakeK8sClient,
					},
					waitCreateNfsServiceInterval: 10 * time.Millisecond,
					nfsClientServicePort:         port,
				}
				return csiNfsService
			}(),
			req: &csi.ControllerPublishVolumeRequest{
				VolumeId: "test-volume",
				NodeId:   "csi-node-123-127.0.0.1",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Block{
						Block: &csi.VolumeCapability_BlockVolume{},
					},
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
					},
				},
				VolumeContext: map[string]string{
					"Name": "volume-name",
				},
			},
			expectedRes: &csi.ControllerPublishVolumeResponse{
				PublishContext: map[string]string{
					"name": "volume-name",
					"nfs":  "test-volume",
				},
			},
			expectedErr: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			DriverName = "csi-powerstore.dellemc.com"

			test.createServer(t)

			time.Sleep(50 * time.Millisecond)

			defer func() { nodeIPAddress = make(map[string]*NodeStatus) }()

			resp, err := test.csiNfsService.ControllerPublishVolume(context.Background(), test.req)
			if !reflect.DeepEqual(resp, test.expectedRes) {
				t.Errorf("expected response %+v, got %+v", test.expectedRes, resp)
			}
			if test.expectedErr != nil {
				assert.Contains(t, err.Error(), test.expectedErr.Error())
			} else {
				assert.Equal(t, test.expectedErr, err)
			}
		})
	}
}

func TestControllerUnpublishVolume(t *testing.T) {
	port := func() string {
		nBig, _ := rand.Int(rand.Reader, big.NewInt(9000))
		return strconv.Itoa(int(nBig.Int64() + 1000))
	}()

	t.Run("endpoint error", func(t *testing.T) {
		ctx := context.Background()
		fakeK8sClient := fake.NewClientset()

		csiNfsServce := &CsiNfsService{
			k8sclient: &k8s.Client{
				Clientset: fakeK8sClient,
			},
		}

		req := csi.ControllerUnpublishVolumeRequest{
			VolumeId: "test-volume",
			NodeId:   "test-node",
		}

		_, err := csiNfsServce.ControllerUnpublishVolume(ctx, &req)
		assert.Nil(t, err)
	})

	t.Run("service not found", func(t *testing.T) {
		ctx := context.Background()
		fakeK8sClient := fake.NewClientset()
		fakeK8sClient.DiscoveryV1().EndpointSlices("").Create(ctx, &discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-volume",
			},
			Endpoints: []discoveryv1.Endpoint{
				{
					Addresses: []string{
						"127.0.0.1",
					},
				},
			},
		}, metav1.CreateOptions{})
		csiNfsServce := &CsiNfsService{
			k8sclient: &k8s.Client{
				Clientset: fakeK8sClient,
			},
		}

		req := csi.ControllerUnpublishVolumeRequest{
			VolumeId: "test-volume",
			NodeId:   "test-node",
		}

		_, err := csiNfsServce.ControllerUnpublishVolume(ctx, &req)
		assert.Nil(t, err)
	})

	t.Run("slice has no IP addr", func(t *testing.T) {
		ctx := context.Background()
		fakeK8sClient := fake.NewClientset()
		fakeK8sClient.DiscoveryV1().EndpointSlices("").Create(ctx, &discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-volume",
			},
			Endpoints: []discoveryv1.Endpoint{
				{
					Addresses: []string{""},
				},
			},
		}, metav1.CreateOptions{})
		fakeK8sClient.CoreV1().Services("").Create(ctx, &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-volume",
			},
		}, metav1.CreateOptions{})
		csiNfsServce := &CsiNfsService{
			k8sclient: &k8s.Client{
				Clientset: fakeK8sClient,
			},
		}

		req := csi.ControllerUnpublishVolumeRequest{
			VolumeId: "test-volume",
			NodeId:   "test-node",
		}

		_, err := csiNfsServce.ControllerUnpublishVolume(ctx, &req)
		assert.Contains(t, err.Error(), "endpointslice apparaently had no IP addresses")
	})

	t.Run("remove last client", func(t *testing.T) {
		ctx := context.Background()
		fakeK8sClient := fake.NewClientset()
		fakeK8sClient.DiscoveryV1().EndpointSlices("").Create(ctx, &discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-volume",
			},
			Endpoints: []discoveryv1.Endpoint{
				{
					Addresses: []string{"127.0.0.1"},
				},
			},
		}, metav1.CreateOptions{})
		fakeK8sClient.CoreV1().Services("").Create(ctx, &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-volume",
				Labels: map[string]string{
					"client/" + "test-node": "test-node",
				},
			},
		}, metav1.CreateOptions{})
		mockNfsServer := mocks.NewMockNfsServer(gomock.NewController(t))
		mockNfsServer.EXPECT().UnexportNfsVolume(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.UnexportNfsVolumeResponse{}, nil)

		mockVcsiService := mocks.NewMockService(gomock.NewController(t))
		mockVcsiService.EXPECT().ControllerUnpublishVolume(gomock.Any(), gomock.Any()).Times(1).Return(&csi.ControllerUnpublishVolumeResponse{}, nil)

		createMockServer(t, "127.0.0.1", port, mockNfsServer)
		csiNfsService := &CsiNfsService{
			vcsi: mockVcsiService,
			k8sclient: &k8s.Client{
				Clientset: fakeK8sClient,
			},
			nfsClientServicePort: port,
		}

		req := csi.ControllerUnpublishVolumeRequest{
			VolumeId: "test-volume",
			NodeId:   "test-node",
		}

		_, err := csiNfsService.ControllerUnpublishVolume(ctx, &req)
		assert.Equal(t, nil, err)
	})

	t.Run("remove non-last client", func(t *testing.T) {
		ctx := context.Background()
		fakeK8sClient := fake.NewClientset()
		fakeK8sClient.DiscoveryV1().EndpointSlices("").Create(ctx, &discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-volume",
			},
			Endpoints: []discoveryv1.Endpoint{
				{
					Addresses: []string{"127.0.0.1"},
				},
			},
		}, metav1.CreateOptions{})
		fakeK8sClient.CoreV1().Services("").Create(ctx, &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-volume",
				Labels: map[string]string{
					"client/" + "test-node":  "test-node",
					"client/" + "test-node2": "test-node2",
				},
			},
		}, metav1.CreateOptions{})
		mockServer := mocks.NewMockNfsServer(gomock.NewController(t))
		mockServer.EXPECT().UnexportNfsVolume(gomock.Any(), gomock.Any()).AnyTimes().Return(&proto.UnexportNfsVolumeResponse{}, nil)
		createMockServer(t, "127.0.0.1", port, mockServer)
		mockVcsiService := mocks.NewMockService(gomock.NewController(t))
		mockVcsiService.EXPECT().ControllerUnpublishVolume(gomock.Any(), gomock.Any()).Times(0).Return(&csi.ControllerUnpublishVolumeResponse{}, nil)
		csiNfsServce := &CsiNfsService{
			vcsi: mockVcsiService,
			k8sclient: &k8s.Client{
				Clientset: fakeK8sClient,
			},
			nfsClientServicePort: port,
		}

		req := csi.ControllerUnpublishVolumeRequest{
			VolumeId: "test-volume",
			NodeId:   "test-node",
		}

		_, err := csiNfsServce.ControllerUnpublishVolume(ctx, &req)
		assert.Equal(t, nil, err)
	})

	t.Run("error: callUnexportNfsVolume", func(t *testing.T) {
		ctx := context.Background()
		fakeK8sClient := fake.NewClientset()
		fakeK8sClient.DiscoveryV1().EndpointSlices("").Create(ctx, &discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-volume",
			},
			Endpoints: []discoveryv1.Endpoint{
				{
					Addresses: []string{"127.0.0.1"},
				},
			},
		}, metav1.CreateOptions{})
		fakeK8sClient.CoreV1().Services("").Create(ctx, &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-volume",
				Labels: map[string]string{
					"client/" + "test-node": "test-node",
				},
			},
		}, metav1.CreateOptions{})
		mockNfsServer := mocks.NewMockNfsServer(gomock.NewController(t))
		mockNfsServer.EXPECT().UnexportNfsVolume(gomock.Any(), gomock.Any()).AnyTimes().Return(nil, fmt.Errorf("error"))

		mockVcsiService := mocks.NewMockService(gomock.NewController(t))

		createMockServer(t, "127.0.0.1", mockNfsServer)
		csiNfsServce := &CsiNfsService{
			vcsi: mockVcsiService,
			k8sclient: &k8s.Client{
				Clientset: fakeK8sClient,
			},
		}

		req := csi.ControllerUnpublishVolumeRequest{
			VolumeId: "test-volume",
			NodeId:   "test-node",
		}

		_, err := csiNfsServce.ControllerUnpublishVolume(ctx, &req)
		assert.NotNil(t, err)
	})

	t.Run("success: node down, continue", func(t *testing.T) {
		ctx := context.Background()
		fakeK8sClient := fake.NewClientset()
		fakeK8sClient.DiscoveryV1().EndpointSlices("").Create(ctx, &discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-volume",
			},
			Endpoints: []discoveryv1.Endpoint{
				{
					Addresses: []string{"127.0.0.1"},
				},
			},
		}, metav1.CreateOptions{})
		fakeK8sClient.CoreV1().Services("").Create(ctx, &v1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-volume",
				Labels: map[string]string{
					"client/" + "test-node": "test-node",
				},
			},
		}, metav1.CreateOptions{})
		mockNfsServer := mocks.NewMockNfsServer(gomock.NewController(t))
		mockNfsServer.EXPECT().UnexportNfsVolume(gomock.Any(), gomock.Any()).AnyTimes().Return(nil, fmt.Errorf("no route to host"))

		mockVcsiService := mocks.NewMockService(gomock.NewController(t))
		mockVcsiService.EXPECT().ControllerUnpublishVolume(gomock.Any(), gomock.Any()).Times(1).Return(&csi.ControllerUnpublishVolumeResponse{}, nil)

		createMockServer(t, "127.0.0.1", mockNfsServer)
		csiNfsServce := &CsiNfsService{
			vcsi: mockVcsiService,
			k8sclient: &k8s.Client{
				Clientset: fakeK8sClient,
			},
		}

		req := csi.ControllerUnpublishVolumeRequest{
			VolumeId: "test-volume",
			NodeId:   "test-node",
		}

		_, err := csiNfsServce.ControllerUnpublishVolume(ctx, &req)
		assert.Equal(t, nil, err)
	})
}

func TestValidateVolumeCapabilities(t *testing.T) {
	tests := []struct {
		name          string
		csiNfsService *CsiNfsService
		req           *csi.ValidateVolumeCapabilitiesRequest
		expectedRes   *csi.ValidateVolumeCapabilitiesResponse
		expectedErr   error
	}{
		{
			name:          "Valid volume request",
			csiNfsService: &CsiNfsService{},
			req:           &csi.ValidateVolumeCapabilitiesRequest{},
			expectedRes:   &csi.ValidateVolumeCapabilitiesResponse{},
			expectedErr:   nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resp, err := test.csiNfsService.ValidateVolumeCapabilities(context.Background(), test.req)
			if !reflect.DeepEqual(resp, test.expectedRes) {
				t.Errorf("expected response %+v, got %+v", test.expectedRes, resp)
			}
			assert.Equal(t, test.expectedErr, err)
		})
	}
}

func TestListVolumes(t *testing.T) {
	tests := []struct {
		name          string
		csiNfsService *CsiNfsService
		req           *csi.ListVolumesRequest
		expectedRes   *csi.ListVolumesResponse
		expectedErr   error
	}{
		{
			name:          "Valid volume request",
			csiNfsService: &CsiNfsService{},
			req:           &csi.ListVolumesRequest{},
			expectedRes:   &csi.ListVolumesResponse{},
			expectedErr:   nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resp, err := test.csiNfsService.ListVolumes(context.Background(), test.req)
			if !reflect.DeepEqual(resp, test.expectedRes) {
				t.Errorf("expected response %+v, got %+v", test.expectedRes, resp)
			}
			assert.Equal(t, test.expectedErr, err)
		})
	}
}

func TestGetCapacity(t *testing.T) {
	tests := []struct {
		name          string
		csiNfsService *CsiNfsService
		req           *csi.GetCapacityRequest
		expectedRes   *csi.GetCapacityResponse
		expectedErr   error
	}{
		{
			name:          "Valid volume request",
			csiNfsService: &CsiNfsService{},
			req:           &csi.GetCapacityRequest{},
			expectedRes:   &csi.GetCapacityResponse{},
			expectedErr:   nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resp, err := test.csiNfsService.GetCapacity(context.Background(), test.req)
			if !reflect.DeepEqual(resp, test.expectedRes) {
				t.Errorf("expected response %+v, got %+v", test.expectedRes, resp)
			}
			assert.Equal(t, test.expectedErr, err)
		})
	}
}

func TestControllerGetCapabilities(t *testing.T) {
	tests := []struct {
		name          string
		csiNfsService *CsiNfsService
		req           *csi.ControllerGetCapabilitiesRequest
		expectedRes   *csi.ControllerGetCapabilitiesResponse
		expectedErr   error
	}{
		{
			name:          "Valid request",
			csiNfsService: &CsiNfsService{},
			req:           &csi.ControllerGetCapabilitiesRequest{},
			expectedRes:   &csi.ControllerGetCapabilitiesResponse{},
			expectedErr:   nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resp, err := test.csiNfsService.ControllerGetCapabilities(context.Background(), test.req)
			if !reflect.DeepEqual(resp, test.expectedRes) {
				t.Errorf("expected response %+v, got %+v", test.expectedRes, resp)
			}
			assert.Equal(t, test.expectedErr, err)
		})
	}
}

func TestCreateSnapshot(t *testing.T) {
	tests := []struct {
		name          string
		csiNfsService *CsiNfsService
		req           *csi.CreateSnapshotRequest
		expectedRes   *csi.CreateSnapshotResponse
		expectedErr   error
	}{
		{
			name:          "Valid snapshot request",
			csiNfsService: &CsiNfsService{},
			req:           &csi.CreateSnapshotRequest{},
			expectedRes:   &csi.CreateSnapshotResponse{},
			expectedErr:   nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resp, err := test.csiNfsService.CreateSnapshot(context.Background(), test.req)
			if !reflect.DeepEqual(resp, test.expectedRes) {
				t.Errorf("expected response %+v, got %+v", test.expectedRes, resp)
			}
			assert.Equal(t, test.expectedErr, err)
		})
	}
}

func TestDeleteSnapshot(t *testing.T) {
	tests := []struct {
		name          string
		csiNfsService *CsiNfsService
		req           *csi.DeleteSnapshotRequest
		expectedErr   error
	}{
		{
			name:          "Valid snapshot request",
			csiNfsService: &CsiNfsService{},
			req:           &csi.DeleteSnapshotRequest{},
			expectedErr:   nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.csiNfsService.DeleteSnapshot(context.Background(), test.req)
			assert.Equal(t, test.expectedErr, err)
		})
	}
}

func TestListSnapshots(t *testing.T) {
	tests := []struct {
		name          string
		csiNfsService *CsiNfsService
		req           *csi.ListSnapshotsRequest
		expectedRes   *csi.ListSnapshotsResponse
		expectedErr   error
	}{
		{
			name:          "Valid snapshot request",
			csiNfsService: &CsiNfsService{},
			req:           &csi.ListSnapshotsRequest{},
			expectedRes:   &csi.ListSnapshotsResponse{},
			expectedErr:   nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resp, err := test.csiNfsService.ListSnapshots(context.Background(), test.req)
			if !reflect.DeepEqual(resp, test.expectedRes) {
				t.Errorf("expected response %+v, got %+v", test.expectedRes, resp)
			}
			assert.Equal(t, test.expectedErr, err)
		})
	}
}

func TestControllerExpandVolume(t *testing.T) {
	tests := []struct {
		name          string
		csiNfsService *CsiNfsService
		req           *csi.ControllerExpandVolumeRequest
		expectedRes   *csi.ControllerExpandVolumeResponse
		expectedErr   error
	}{
		{
			name:          "Valid volume request",
			csiNfsService: &CsiNfsService{},
			req:           &csi.ControllerExpandVolumeRequest{},
			expectedRes:   &csi.ControllerExpandVolumeResponse{},
			expectedErr:   nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resp, err := test.csiNfsService.ControllerExpandVolume(context.Background(), test.req)
			if !reflect.DeepEqual(resp, test.expectedRes) {
				t.Errorf("expected response %+v, got %+v", test.expectedRes, resp)
			}
			assert.Equal(t, test.expectedErr, err)
		})
	}
}

func TestControllerGetVolume(t *testing.T) {
	tests := []struct {
		name          string
		csiNfsService *CsiNfsService
		req           *csi.ControllerGetVolumeRequest
		expectedRes   *csi.ControllerGetVolumeResponse
		expectedErr   error
	}{
		{
			name:          "Valid volume request",
			csiNfsService: &CsiNfsService{},
			req:           &csi.ControllerGetVolumeRequest{},
			expectedRes:   nil,
			expectedErr:   status.Error(400, "Not implemented"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resp, err := test.csiNfsService.ControllerGetVolume(context.Background(), test.req)
			if !reflect.DeepEqual(resp, test.expectedRes) {
				t.Errorf("expected response %+v, got %+v", test.expectedRes, resp)
			}
			assert.Equal(t, test.expectedErr, err)
		})
	}
}

func TestAddNodeToNfsService(t *testing.T) {
	ctx := context.Background()
	t.Run("Error updating service", func(t *testing.T) {
		fakeK8sClient := fake.NewClientset()
		cs := &CsiNfsService{
			k8sclient: &k8s.Client{
				Clientset: fakeK8sClient,
			},
		}

		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "test-namespace",
				Name:      "test-service",
				Labels:    map[string]string{},
			},
		}

		req := &csi.ControllerPublishVolumeRequest{
			NodeId: "test-node",
		}

		updatedService, err := cs.addNodeToNfsService(ctx, service, req)

		assert.Error(t, err)
		assert.Nil(t, updatedService)
	})
}
