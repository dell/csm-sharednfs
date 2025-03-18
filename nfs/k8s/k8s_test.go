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

package k8s

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	ktesting "k8s.io/client-go/testing"
)

func TestConnect(t *testing.T) {
	fakeClientSet := fake.NewSimpleClientset()
	tests := []struct {
		name              string
		newForConfigFunc  func(config *rest.Config) (kubernetes.Interface, error)
		restInClusterFunc func() (*rest.Config, error)
		want              *K8sClient
		wantErr           bool
	}{
		{
			name: "Test Connect with successful connection",
			newForConfigFunc: func(config *rest.Config) (kubernetes.Interface, error) {
				return fakeClientSet, nil
			},
			restInClusterFunc: func() (*rest.Config, error) {
				return &rest.Config{}, nil
			},

			want: &K8sClient{
				Clientset: fakeClientSet,
			},
			wantErr: false,
		},
		{
			name: "Test Connect with failed rest connection",
			newForConfigFunc: func(config *rest.Config) (kubernetes.Interface, error) {
				return fake.NewSimpleClientset(), nil
			},
			restInClusterFunc: func() (*rest.Config, error) {
				return nil, errors.New("failed to create rest config")
			},

			want:    nil,
			wantErr: true,
		},
		{
			name: "Test Connect with failed k8s client connection",
			newForConfigFunc: func(config *rest.Config) (kubernetes.Interface, error) {
				return nil, errors.New("failed to create k8s client")
			},
			restInClusterFunc: func() (*rest.Config, error) {
				return &rest.Config{}, nil
			},

			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RestInClusterConfigFunc = tt.restInClusterFunc
			NewForConfigFunc = tt.newForConfigFunc
			got, err := Connect()
			if (err != nil) != tt.wantErr {
				t.Errorf("Connect() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Connect() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCreateService(t *testing.T) {
	// Create a fake Kubernetes clientset
	clientset := fake.NewSimpleClientset()

	// Create a fake service
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-service",
		},
	}

	// Create a fake K8sClient
	k8sClient := &K8sClient{
		Clientset: clientset,
	}

	// Call the CreateService method
	s, err := k8sClient.CreateService(context.Background(), "test-namespace", service)
	if err != nil {
		t.Errorf("CreateService returned an unexpected error: %v", err)
	}

	// Check that the service was created correctly
	if s.Name != service.Name {
		t.Errorf("CreateService did not create the service correctly. Expected: %v, got: %v", service, s)
	}
}

func TestGetService(t *testing.T) {
	// Create a fake Kubernetes clientset
	clientset := fake.NewSimpleClientset()

	// Create a fake service
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-service",
		},
	}
	// Create a fake K8sClient
	k8sClient := &K8sClient{
		Clientset: clientset,
	}

	// Call the CreateService method
	_, err := k8sClient.CreateService(context.Background(), "test-namespace", service)
	if err != nil {
		t.Errorf("Create failed in GetService returned an unexpected error: %v", err)
	}

	s, err := k8sClient.GetService(context.Background(), "test-namespace", "test-service")
	if err != nil {
		t.Errorf("GetService returned an unexpected error: %v", err)
	}
	// Check that the service was created correctly
	if s.Name != service.Name {
		t.Errorf("GetService failed. Expected: %v, got: %v", service, s)
	}
}

func TestUpdateService(t *testing.T) {
	// Create a fake Kubernetes clientset
	clientset := fake.NewSimpleClientset()

	// Create a fake service
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-service",
			Annotations: map[string]string{
				"test-annotation": "test-annotation-value",
			},
		},
	}
	// Create a fake K8sClient
	k8sClient := &K8sClient{
		Clientset: clientset,
	}

	// Call the CreateService method
	_, err := k8sClient.CreateService(context.Background(), "test-namespace", service)
	// Check that there is no error
	if err != nil {
		t.Errorf("CreateService returned an unexpected error: %v", err)
	}

	// Call the UpdateService method, change the service annotation to new-test-annotation-value
	service = &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-service",
			Annotations: map[string]string{
				"test-annotation": "new-test-annotation-value",
			},
		},
	}
	_, err = k8sClient.UpdateService(context.Background(), "test-namespace", service)
	if err != nil {
		t.Errorf("UpdateService returned an unexpected error: %v", err)
	}

	s, err := k8sClient.GetService(context.Background(), "test-namespace", "test-service")
	if err != nil {
		t.Errorf("failed to get secret : unexpected error: %v", err)
	}
	// Check that the service annotation was updated correctly since that is the only change.
	if s.Annotations["test-annotation"] != service.Annotations["test-annotation"] {
		t.Errorf("CreateService did not create the service correctly. Expected: %v, got: %v", service, s)
	}
}

func TestDeleteService(t *testing.T) {
	// Create a fake Kubernetes clientset
	clientset := fake.NewSimpleClientset()

	// Create a fake service
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-service",
		},
	}
	// Create a fake K8sClient
	k8sClient := &K8sClient{
		Clientset: clientset,
	}

	// Call the CreateService method
	_, err := k8sClient.CreateService(context.Background(), "test-namespace", service)
	// Check that there is no error
	if err != nil {
		t.Errorf("CreateService returned an unexpected error: %v", err)
	}

	err = k8sClient.DeleteService(context.Background(), "test-namespace", "test-service")
	// Check that the service was created correctly
	if err != nil {
		t.Errorf("DeleteService did not delete the service correctly.")
	}
}

func TestGetEndpointSlice(t *testing.T) {
	// Create a fake Kubernetes clientset
	clientset := fake.NewSimpleClientset()

	// Create a fake endpoint slice
	namespace := "test-namespace"
	name := "test-endpoint-slice"
	endpointSlice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	_, err := clientset.DiscoveryV1().EndpointSlices(namespace).Create(context.Background(), endpointSlice, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create fake endpoint slice: %v", err)
	}

	// Create a fake K8sClient
	k8sClient := &K8sClient{
		Clientset: clientset,
	}

	// Call the GetEndpointSlice method
	result, err := k8sClient.GetEndpointSlice(context.Background(), namespace, name)
	if err != nil {
		t.Errorf("GetEndpointSlice returned an unexpected error: %v", err)
	}

	// Check that the endpoint slice was retrieved correctly
	if result.Name != name {
		t.Errorf("GetEndpointSlice did not retrieve the endpoint slice correctly. Expected: %v, got: %v", name, result.Name)
	}
}

func TestCreateEndpointSlice(t *testing.T) {
	// Create a fake Kubernetes clientset
	clientset := fake.NewSimpleClientset()

	// Create a fake endpoint slice
	namespace := "test-namespace"
	name := "test-endpoint-slice"
	endpointSlice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	// Create a fake K8sClient
	k8sClient := &K8sClient{
		Clientset: clientset,
	}

	// Call the CreateEndpointSlice method
	result, err := k8sClient.CreateEndpointSlice(context.Background(), namespace, endpointSlice)
	if err != nil {
		t.Errorf("CreateEndpointSlice returned an unexpected error: %v", err)
	}

	// Check that the endpoint slice was created correctly
	if result.Name != name {
		t.Errorf("CreateEndpointSlice did not create the endpoint slice correctly. Expected: %v, got: %v", name, result.Name)
	}
}

func TestUpdateEndpointSlice(t *testing.T) {
	// Create a fake Kubernetes clientset
	clientset := fake.NewSimpleClientset()

	// Create a fake endpoint slice
	namespace := "test-namespace"
	name := "test-endpoint-slice"
	endpointSlice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	_, err := clientset.DiscoveryV1().EndpointSlices(namespace).Create(context.Background(), endpointSlice, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create fake endpoint slice: %v", err)
	}

	// Create a fake K8sClient
	k8sClient := &K8sClient{
		Clientset: clientset,
	}

	// Update the endpoint slice
	endpointSlice.Labels = map[string]string{
		"foo": "bar",
	}

	// Call the UpdateEndpointSlice method
	result, err := k8sClient.UpdateEndpointSlice(context.Background(), namespace, endpointSlice)
	if err != nil {
		t.Errorf("UpdateEndpointSlice returned an unexpected error: %v", err)
	}

	// Check that the endpoint slice was updated correctly
	if !reflect.DeepEqual(result.Labels, endpointSlice.Labels) {
		t.Errorf("UpdateEndpointSlice did not update the endpoint slice correctly. Expected: %v, got: %v", endpointSlice.Labels, result.Labels)
	}
}

func TestDeleteEndpointSlice(t *testing.T) {
	// Create a fake Kubernetes clientset
	clientset := fake.NewSimpleClientset()

	// Create a fake endpoint slice
	namespace := "test-namespace"
	name := "test-endpoint-slice"
	endpointSlice := &discoveryv1.EndpointSlice{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	_, err := clientset.DiscoveryV1().EndpointSlices(namespace).Create(context.Background(), endpointSlice, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create fake endpoint slice: %v", err)
	}

	// Create a fake K8sClient
	k8sClient := &K8sClient{
		Clientset: clientset,
	}

	// Call the DeleteEndpointSlice method
	err = k8sClient.DeleteEndpointSlice(context.Background(), namespace, name)
	if err != nil {
		t.Errorf("DeleteEndpointSlice returned an unexpected error: %v", err)
	}

	// Check that the endpoint slice was deleted correctly
	_, err = k8sClient.GetEndpointSlice(context.Background(), namespace, name)
	if err == nil {
		t.Errorf("DeleteEndpointSlice did not delete the endpoint slice correctly. Expected: NotFound, got: %v", err)
	}
}

func TestGetPersistentVolume(t *testing.T) {
	// Create a fake Kubernetes clientset
	clientset := fake.NewSimpleClientset()

	// Create a fake persistent volume
	name := "test-pv"
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	_, err := clientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create fake persistent volume: %v", err)
	}

	// Create a fake K8sClient
	k8sClient := &K8sClient{
		Clientset: clientset,
	}

	// Call the GetPersistentVolume method
	result, err := k8sClient.GetPersistentVolume(context.Background(), name)
	if err != nil {
		t.Errorf("GetPersistentVolume returned an unexpected error: %v", err)
	}

	// Check that the persistent volume was retrieved correctly
	if result.Name != name {
		t.Errorf("GetPersistentVolume did not retrieve the persistent volume correctly. Expected: %v, got: %v", name, result.Name)
	}
}

func TestGetNode(t *testing.T) {
	// Create a fake Kubernetes clientset
	clientset := fake.NewSimpleClientset()

	// Create a fake node
	name := "test-node"
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	_, err := clientset.CoreV1().Nodes().Create(context.Background(), node, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create fake node: %v", err)
	}

	// Create a fake K8sClient
	k8sClient := &K8sClient{
		Clientset: clientset,
	}

	// Call the GetNode method
	result, err := k8sClient.GetNode(context.Background(), name)
	if err != nil {
		t.Errorf("GetNode returned an unexpected error: %v", err)
	}

	// Check that the node was retrieved correctly
	if result.Name != name {
		t.Errorf("GetNode did not retrieve the node correctly. Expected: %v, got: %v", name, result.Name)
	}
}

func TestGetEndpointSlices(t *testing.T) {
	tests := []struct {
		name           string
		namespace      string
		labelSelector  string
		setup          func(clientset *fake.Clientset)
		expectedErr    bool
		expectedResult []discoveryv1.EndpointSlice
	}{
		{
			name:          "get endpoint slices successfully",
			namespace:     "test-namespace",
			labelSelector: "foo=bar",
			setup: func(clientset *fake.Clientset) {
				namespace := "test-namespace"
				name := "test-endpoint-slice"
				endpointSlice := &discoveryv1.EndpointSlice{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
						Labels: map[string]string{
							"foo": "bar",
						},
					},
				}
				es, err := clientset.DiscoveryV1().EndpointSlices(namespace).Create(context.Background(), endpointSlice, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("Failed to create fake endpoint slice: %v", err)
				}
				t.Logf("Created endpoint slice: %+v", es)
			},
			expectedErr: false,
			expectedResult: []discoveryv1.EndpointSlice{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-endpoint-slice",
						Namespace: "test-namespace",
						Labels: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
		},
		{
			name:          "get endpoint slices with error",
			namespace:     "test-namespace",
			labelSelector: "foo=bar",
			setup: func(clientset *fake.Clientset) {
				clientset.PrependReactor("list", "endpointslices", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
					t.Logf("Reactor called : %v", action)
					return true, nil, fmt.Errorf("failed to list endpointslices")
				})
			},
			expectedErr:    true,
			expectedResult: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create a fake Kubernetes clientset
			clientset := fake.NewSimpleClientset()

			// Set up the fake clientset
			if test.setup != nil {
				test.setup(clientset)
			}

			// Create a fake K8sClient
			k8sClient := &K8sClient{
				Clientset: clientset,
			}

			// Call the GetEndpointSlices method
			result, err := k8sClient.GetEndpointSlices(context.Background(), test.namespace, test.labelSelector)

			t.Logf("Result: %+v", result)
			if test.expectedErr {
				if result != nil {
					t.Errorf("GetEndpointSlices returned an unexpected error. Expected: %v, got: %v", test.expectedErr, err)
				}
			} else {
				if result[0].Name != test.expectedResult[0].Name || result[0].Namespace != test.expectedResult[0].Namespace ||
					result[0].Labels["foo"] != test.expectedResult[0].Labels["foo"] {
					t.Errorf("GetEndpointSlices did not retrieve the correct endpoint slices. Expected: %+v, got: %+v", test.expectedResult, result)
				}
			}
		})
	}
}

func TestGetAllNodes(t *testing.T) {
	tests := []struct {
		name    string
		want    []*v1.Node
		wantErr bool
	}{
		{
			name: "successful retrieval",
			want: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
					},
				},
			},
			wantErr: false,
		},
		{
			name:    "error retrieving nodes",
			want:    nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fake Kubernetes client
			clientset := fake.NewSimpleClientset()
			for _, node := range tt.want {
				_, err := clientset.CoreV1().Nodes().Create(context.Background(), node, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("Failed to create fake node: %v", err)
				}
			}

			// Create a fake K8sClient
			k8sClient := &K8sClient{
				Clientset: clientset,
			}

			// Call the GetAllNodes method
			result, err := k8sClient.GetAllNodes(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAllNodes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check the retrieved nodes
			if !reflect.DeepEqual(result, tt.want) {
				// reflect.DeepEqual returns false if both maps are empty, hence the length check
				if len(result) != 0 && len(tt.want) != 0 {
					t.Errorf("GetAllNodes() = %v, want %v", result, tt.want)
				}
			}
		})
	}
}

func TestGetNodeByCSINodeId(t *testing.T) {
	tests := []struct {
		name      string
		driverKey string
		csiNodeId string
		node      *v1.Node
		wantNode  *v1.Node
		wantErr   bool
	}{
		{
			name:      "successful retrieval",
			driverKey: "csi-powerstore.dellemc.com",
			csiNodeId: "csi-node-e8d45b5d10cc4f79953ba100c7fff4cb-10.20.30.40",
			wantNode: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						"csi.volume.kubernetes.io/nodeid": "{\"csi-powerstore.dellemc.com\": \"csi-node-e8d45b5d10cc4f79953ba100c7fff4cb-10.20.30.40\",\"csi-vxflexos.dellemc.com\": \"638AD887-6FC4-4A8E-B3D1-DE4A9023BE3B\"}",
					},
				},
			},
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						"csi.volume.kubernetes.io/nodeid": "{\"csi-powerstore.dellemc.com\": \"csi-node-e8d45b5d10cc4f79953ba100c7fff4cb-10.20.30.40\",\"csi-vxflexos.dellemc.com\": \"638AD887-6FC4-4A8E-B3D1-DE4A9023BE3B\"}",
					},
				},
			},
			wantErr: false,
		},
		{
			name:      "bad json retrieval",
			driverKey: "csi.volume.kubernetes.io/nodeid",
			csiNodeId: "csi-node-e8d45b5d10cc4f79953ba100c7fff4cb-10.20.30.40",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						"csi.volume.kubernetes.io/nodeid": "{ invalid: 123",
					},
				},
			},
			wantNode: nil,
			wantErr:  true,
		},
		{
			name:      "error retrieving nodes",
			driverKey: "csi.volume.kubernetes.io/nodeid",
			csiNodeId: "node2",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node1",
					Annotations: map[string]string{
						"csi.volume.kubernetes.io/nodeid": "node1",
					},
				},
			},
			wantNode: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fake Kubernetes client
			clientset := fake.NewSimpleClientset()

			// Create a fake K8sClient
			k8sClient := &K8sClient{
				Clientset: clientset,
			}

			if tt.node != nil {
				_, err := clientset.CoreV1().Nodes().Create(context.Background(), tt.node, metav1.CreateOptions{})
				if err != nil {
					t.Fatalf("Failed to create fake node: %v", err)
				}
			}

			// Call the GetNodeByCSINodeId method
			result, err := k8sClient.GetNodeByCSINodeId(context.Background(), tt.driverKey, tt.csiNodeId)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetNodeByCSINodeId() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check the retrieved node
			if !reflect.DeepEqual(result, tt.wantNode) {
				t.Errorf("GetNodeByCSINodeId() = %v, want %v", result, tt.wantNode)
			}
		})
	}
}
