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
	"fmt"
	"net"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	k8s "github.com/dell/csm-hbnfs/nfs/k8s"
	"github.com/dell/csm-hbnfs/nfs/mocks"
	"github.com/dell/csm-hbnfs/nfs/proto"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// MockListener is a mock implementation of net.Listener
type mockListener struct{}

func (m *mockListener) Accept() (net.Conn, error) {
	return nil, nil
}

func (m *mockListener) Close() error {
	return nil
}

func (m *mockListener) Addr() net.Addr {
	return nil
}

func TestExportMultipleNfsVolume(t *testing.T) {
	exportsDir = "/tmp/noderoot/etc/"
	exportsFile = "exports"
	pathToExports = exportsDir + exportsFile

	testCases := []struct {
		name         string
		request      *proto.ExportMultipleNfsVolumesRequest
		expectedResp *proto.ExportMultipleNfsVolumesResponse
		service      *mocks.MockService
		executor     *mocks.MockExecutor
		osMock       *mocks.MockOSInterface
		expectedErr  error
	}{
		{
			name: "Successful ExportMultipleNfsVolumes",
			request: &proto.ExportMultipleNfsVolumesRequest{
				VolumeIds: []string{
					"test-volume",
				},
				ExportNfsContext: map[string]string{"test-key": "test-value"},
			},

			expectedResp: &proto.ExportMultipleNfsVolumesResponse{
				SuccessfulIds: []string{
					"test-volume",
				},
				ExportNfsContext: map[string]string{"test-key": "test-value"},
			},
			service: func() *mocks.MockService {
				service := mocks.NewMockService(gomock.NewController(t))
				service.EXPECT().MountVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(pathToExports, nil)
				return service
			}(),

			executor: func() *mocks.MockExecutor {
				mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))
				mockExecutor.EXPECT().ExecuteCommand(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand(chroot, nodeRoot, exportfs, "-r", "-a").Return(nil, nil).AnyTimes()
				return mockExecutor
			}(),

			osMock: func() *mocks.MockOSInterface {
				mockOs := mocks.NewMockOSInterface(gomock.NewController(t))
				mockOs.EXPECT().Chown(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)
				mockOs.EXPECT().Chmod(gomock.Any(), gomock.Any()).Times(1).Return(nil)

				mockOs.EXPECT().Open(gomock.Any()).Times(1).DoAndReturn(func(name string) (*os.File, error) {
					return os.Open(name)
				})
				mockOs.EXPECT().OpenFile(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).DoAndReturn(func(name string, flag int, perm os.FileMode) (*os.File, error) {
					return os.OpenFile(name, flag, perm)
				})

				return mockOs
			}(),
			expectedErr: nil,
		},
		{
			name: "UnSuccessful ExportMultipleNfsVolumes",
			request: &proto.ExportMultipleNfsVolumesRequest{
				VolumeIds: []string{
					"test-volume",
				},
				ExportNfsContext: map[string]string{"test-key": "test-value"},
			},

			expectedResp: &proto.ExportMultipleNfsVolumesResponse{
				UnsuccessfulIds: []string{
					"test-volume",
				},
				ExportNfsContext: map[string]string{"test-key": "test-value"},
			},
			service: func() *mocks.MockService {
				service := mocks.NewMockService(gomock.NewController(t))
				service.EXPECT().MountVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(pathToExports, fmt.Errorf("failed to mount volume"))
				return service
			}(),
			executor: func() *mocks.MockExecutor {
				mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))
				return mockExecutor
			}(),
			osMock: func() *mocks.MockOSInterface {
				mockOs := mocks.NewMockOSInterface(gomock.NewController(t))
				return mockOs
			}(),
			expectedErr: fmt.Errorf("failed to mount volume"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := os.MkdirAll(exportsDir, os.ModePerm)
			if err != nil {
				t.Fatal(err)
			}
			file, err := os.Create(pathToExports)
			if err != nil {
				t.Fatal(err)
			}

			GetLocalExecutor = func() Executor {
				return tc.executor
			}

			nfsService = &CsiNfsService{
				vcsi: &CsiNfsService{
					executor: tc.executor,
				},
			}
			nfsService.vcsi = tc.service
			nfs := &nfsServer{
				executor: tc.executor,
			}

			opSys = tc.osMock

			_, err = nfs.ExportMultipleNfsVolumes(context.Background(), tc.request)
			_ = file.Close()
			_ = os.RemoveAll(exportsDir)

			if tc.expectedErr != nil {
				if tc.expectedErr.Error() != err.Error() {
					t.Errorf("Expected error: %v, but got: %v", tc.expectedErr, err)
				}
			} else {
				if !errors.Is(err, tc.expectedErr) {
					t.Errorf("Expected error: %v, but got: %v", tc.expectedErr, err)
				}
			}
		})
	}
}

func TestUnExportMultipleNfsVolume(t *testing.T) {
	exportsDir = "/tmp/noderoot/etc/"
	exportsFile = "exports"
	pathToExports = exportsDir + exportsFile

	testCases := []struct {
		name        string
		request     *proto.UnexportMultipleNfsVolumesRequest
		expected    *proto.UnexportMultipleNfsVolumesResponse
		service     *mocks.MockService
		executor    *mocks.MockExecutor
		osMock      *mocks.MockOSInterface
		expectedErr error
	}{
		{
			name: "Successful UnExportMultipleNfsVolumes",
			request: &proto.UnexportMultipleNfsVolumesRequest{
				VolumeIds: []string{"test-volume"},
				ExportNfsContext: map[string]string{
					"ServiceName": "test-service",
					"test-key":    "test-value",
				},
			},
			expected: &proto.UnexportMultipleNfsVolumesResponse{
				SuccessfulIds: []string{"test-volume"},
				ExportNfsContext: map[string]string{
					"ServiceName": "test-service",
					"test-key":    "test-value",
				},
			},
			service: func() *mocks.MockService {
				service := mocks.NewMockService(gomock.NewController(t))
				service.EXPECT().UnmountVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return service
			}(),
			executor: func() *mocks.MockExecutor {
				mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))
				mockExecutor.EXPECT().ExecuteCommand(chroot, nodeRoot, exportfs, "-r", "-a").Return(nil, nil).AnyTimes()
				return mockExecutor
			}(),
			osMock: func() *mocks.MockOSInterface {
				mockOs := mocks.NewMockOSInterface(gomock.NewController(t))
				mockOs.EXPECT().Open(gomock.Any()).Times(1).DoAndReturn(func(name string) (*os.File, error) {
					return os.Open(name)
				})
				mockOs.EXPECT().OpenFile(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).DoAndReturn(func(name string, flag int, perm os.FileMode) (*os.File, error) {
					return os.OpenFile(name, flag, perm)
				})
				return mockOs
			}(),
			expectedErr: nil,
		},
		{
			name: "Unsuccessful UnExportMultipleNfsVolumes",
			request: &proto.UnexportMultipleNfsVolumesRequest{
				VolumeIds: []string{"test-volume"},
				ExportNfsContext: map[string]string{
					"ServiceName": "test-service",
					"test-key":    "test-value",
				},
			},
			expected: &proto.UnexportMultipleNfsVolumesResponse{
				UnsuccessfulIds: []string{"test-volume"},
				ExportNfsContext: map[string]string{
					"ServiceName": "test-service",
					"test-key":    "test-value",
				},
			},
			service: func() *mocks.MockService {
				service := mocks.NewMockService(gomock.NewController(t))
				service.EXPECT().UnmountVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("failed to unmount")).AnyTimes()
				return service
			}(),
			executor: func() *mocks.MockExecutor {
				mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))
				mockExecutor.EXPECT().ExecuteCommand(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return([]byte{}, fmt.Errorf("failed to resync"))
				return mockExecutor
			}(),
			osMock: func() *mocks.MockOSInterface {
				mockOs := mocks.NewMockOSInterface(gomock.NewController(t))
				mockOs.EXPECT().Open(gomock.Any()).Times(1).DoAndReturn(func(name string) (*os.File, error) {
					return os.Open(name)
				})
				mockOs.EXPECT().OpenFile(gomock.Any(), gomock.Any(), gomock.Any()).Times(1).DoAndReturn(func(name string, flag int, perm os.FileMode) (*os.File, error) {
					return os.OpenFile(name, flag, perm)
				})
				return mockOs
			}(),
			expectedErr: fmt.Errorf("failed to resync"),
		},
	}

	retrySleep = 50 * time.Millisecond

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := os.MkdirAll(exportsDir, os.ModePerm)
			if err != nil {
				t.Fatal(err)
			}
			file, err := os.Create(pathToExports)
			if err != nil {
				t.Fatal(err)
			}
			_, err = file.WriteString("nfs exports")
			if err != nil {
				t.Fatal(err)
			}

			nfsService = &CsiNfsService{
				vcsi: &CsiNfsService{
					executor: tc.executor,
				},
			}

			nfsService.vcsi = tc.service
			nfs := &nfsServer{
				executor: tc.executor,
			}

			GetLocalExecutor = func() Executor {
				return tc.executor
			}

			opSys = tc.osMock

			_, err = nfs.UnexportMultipleNfsVolumes(context.Background(), tc.request)
			_ = file.Close()
			_ = os.RemoveAll(exportsDir)

			if tc.expectedErr != nil {
				if tc.expectedErr.Error() != err.Error() {
					t.Errorf("Expected error: %v, but got: %v", tc.expectedErr, err)
				}
			} else {
				if !errors.Is(err, tc.expectedErr) {
					t.Errorf("Expected error: %v, but got: %v", tc.expectedErr, err)
				}
			}
		})
	}
}

func TestNFSGetExports(t *testing.T) {
	exportsDir = "/tmp/noderoot/etc/"
	exportsFile = "exports"
	pathToExports = exportsDir + exportsFile

	err := os.MkdirAll(exportsDir, os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(pathToExports)
	if err != nil {
		t.Fatal(err)
	}

	mockOs := mocks.NewMockOSInterface(gomock.NewController(t))
	mockOs.EXPECT().Open(gomock.Any()).Times(1).DoAndReturn(func(name string) (*os.File, error) {
		return os.Open(name)
	})

	opSys = mockOs

	_, err = file.WriteString("export 127.0.0.1(rw)\n")
	if err != nil {
		t.Fatal(err)
	}

	getExportsRequest := &proto.GetExportsRequest{}
	nfs := nfsServer{}
	_, err = nfs.GetExports(context.Background(), getExportsRequest)
	_ = file.Close()
	_ = os.RemoveAll(exportsDir)
	if err != nil {
		t.Fatal(err)
	}
}

func TestNFSPing(t *testing.T) {
	exportsDir = "/tmp/noderoot/etc/"
	exportsFile = "exports"
	pathToExports = exportsDir + exportsFile

	// set attempts and timeouts to a minimum to avoid
	// tests timing out and restore their defaults after
	// this test is complete.
	defaultMaxGetSvcAttempts := maxGetSvcAttempts
	defaultMaxUnmountAttempts := maxUnmountAttempts
	defaultTimeout := timeout
	defaultRetrySleep := retrySleep
	defaultWaitTime := waitTime

	defer func() {
		maxGetSvcAttempts = defaultMaxGetSvcAttempts
		maxUnmountAttempts = defaultMaxUnmountAttempts
		timeout = defaultTimeout
		retrySleep = defaultRetrySleep
		waitTime = defaultWaitTime
	}()

	retrySleep = 0
	waitTime = 0
	maxGetSvcAttempts = 1
	maxUnmountAttempts = 1
	timeout = 0 * time.Second

	testCases := []struct {
		name             string
		request          *proto.PingRequest
		expected         *proto.PingResponse
		nfs              *nfsServer
		executor         *mocks.MockExecutor
		osMock           *mocks.MockOSInterface
		nfsService       *CsiNfsService
		createExportFile func() (file *os.File)
		deleteExportFile func(file *os.File)
		expectedErr      error
	}{
		{
			name: "False DumpAllExports",
			request: &proto.PingRequest{
				NodeIpAddress:  "127.0.0.1",
				DumpAllExports: false,
			},
			expected: &proto.PingResponse{
				Ready:  true,
				Status: "",
			},
			nfs: func() *nfsServer {
				return &nfsServer{}
			}(),
			executor: func() *mocks.MockExecutor {
				mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))
				return mockExecutor
			}(),
			osMock: func() *mocks.MockOSInterface {
				mockOs := mocks.NewMockOSInterface(gomock.NewController(t))
				return mockOs
			}(),
			nfsService: func() *CsiNfsService {
				return &CsiNfsService{}
			}(),
			createExportFile: func() (file *os.File) {
				return nil
			},
			deleteExportFile: func(_ *os.File) {},
			expectedErr:      nil,
		},
		{
			name: "error getting exports",
			request: &proto.PingRequest{
				NodeIpAddress:  "127.0.0.1",
				DumpAllExports: true,
			},
			nfs: func() *nfsServer {
				return &nfsServer{}
			}(),
			osMock: func() *mocks.MockOSInterface {
				mockOs := mocks.NewMockOSInterface(gomock.NewController(t))
				mockOs.EXPECT().Open(gomock.Any()).Times(1).Return(nil, fmt.Errorf("file not found"))
				return mockOs
			}(),
			nfsService: func() *CsiNfsService {
				return &CsiNfsService{}
			}(),
			expected: &proto.PingResponse{
				Ready:  true,
				Status: "",
			},
			createExportFile: func() (file *os.File) {
				return &os.File{}
			},
			deleteExportFile: func(_ *os.File) {},
			expectedErr:      errors.New("file not found"),
		},
		{
			name: "fail to GetServiceContent",
			request: &proto.PingRequest{
				NodeIpAddress:  "127.0.0.1",
				DumpAllExports: true,
			},
			expected: &proto.PingResponse{
				Ready:  true,
				Status: "",
			},
			nfs: func() *nfsServer {
				return &nfsServer{}
			}(),
			executor: func() *mocks.MockExecutor {
				mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))
				return mockExecutor
			}(),
			osMock: func() *mocks.MockOSInterface {
				mockOs := mocks.NewMockOSInterface(gomock.NewController(t))
				// GetExports
				mockOs.EXPECT().Open(gomock.Any()).DoAndReturn(func(name string) (*os.File, error) {
					return os.Open(name)
				}).MaxTimes(1)
				return mockOs
			}(),
			nfsService: func() *CsiNfsService {
				csiNFSService := &CsiNfsService{
					k8sclient: &k8s.Client{
						Clientset: fake.NewClientset(),
					},
				}

				return csiNFSService
			}(),
			createExportFile: func() *os.File {
				err := os.MkdirAll(exportsDir, os.ModePerm)
				if err != nil {
					t.Fatal(err)
				}
				err = os.MkdirAll("/tmp/noderoot/export 127.0.0.1(rw)", os.ModePerm)
				if err != nil {
					t.Fatal(err)
				}
				file, err := os.Create(pathToExports)
				if err != nil {
					t.Fatal(err)
				}
				_, err = file.WriteString("export 127.0.0.1(rw)\n")
				if err != nil {
					t.Fatal(err)
				}
				return file
			},
			deleteExportFile: func(file *os.File) {
				_ = file.Close()
				_ = os.RemoveAll(exportsDir)
				_ = os.RemoveAll("/tmp/noderoot/export 127.0.0.1(rw)")
			},
			expectedErr: nil,
		},

		{
			name: "fail to get driverVolumeID",
			request: &proto.PingRequest{
				NodeIpAddress:  "127.0.0.1",
				DumpAllExports: true,
			},
			expected: &proto.PingResponse{
				Ready:  true,
				Status: "",
			},
			nfs: func() *nfsServer {
				return &nfsServer{}
			}(),
			executor: func() *mocks.MockExecutor {
				mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))
				return mockExecutor
			}(),
			osMock: func() *mocks.MockOSInterface {
				mockOs := mocks.NewMockOSInterface(gomock.NewController(t))
				// GetExports
				mockOs.EXPECT().Open(gomock.Any()).DoAndReturn(func(name string) (*os.File, error) {
					return os.Open(name)
				}).MaxTimes(1)
				return mockOs
			}(),
			nfsService: func() *CsiNfsService {
				csiNFSService := &CsiNfsService{
					k8sclient: &k8s.Client{
						Clientset: fake.NewClientset(),
					},
				}

				// mocks for k8s clientset
				_, err := csiNFSService.k8sclient.CreateService(context.Background(), DriverNamespace, &v1.Service{
					ObjectMeta: metav1.ObjectMeta{Name: ""},
				})
				if err != nil {
					t.Fatalf("failed to create fake nfs service: err: %s", err.Error())
				}

				return csiNFSService
			}(),
			createExportFile: func() *os.File {
				err := os.MkdirAll(exportsDir, os.ModePerm)
				if err != nil {
					t.Fatal(err)
				}
				err = os.MkdirAll("/tmp/noderoot/export 127.0.0.1(rw)", os.ModePerm)
				if err != nil {
					t.Fatal(err)
				}
				file, err := os.Create(pathToExports)
				if err != nil {
					t.Fatal(err)
				}
				_, err = file.WriteString("export 127.0.0.1(rw)\n")
				if err != nil {
					t.Fatal(err)
				}
				return file
			},
			deleteExportFile: func(file *os.File) {
				_ = file.Close()
				_ = os.RemoveAll(exportsDir)
				_ = os.RemoveAll("/tmp/noderoot/export 127.0.0.1(rw)")
			},
			expectedErr: nil,
		},

		{
			name: "fail to get DeleteExports",
			request: &proto.PingRequest{
				NodeIpAddress:  "127.0.0.1",
				DumpAllExports: true,
			},
			expected: &proto.PingResponse{
				Ready:  true,
				Status: "",
			},
			nfs: func() *nfsServer {
				return &nfsServer{}
			}(),
			executor: func() *mocks.MockExecutor {
				mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))
				return mockExecutor
			}(),
			osMock: func() *mocks.MockOSInterface {
				mockOs := mocks.NewMockOSInterface(gomock.NewController(t))
				// GetExports
				mockOs.EXPECT().Open(gomock.Any()).DoAndReturn(func(name string) (*os.File, error) {
					return os.Open(name)
				}).MaxTimes(1)

				// DeleteExports
				mockOs.EXPECT().Open(gomock.Any()).DoAndReturn(func(_ string) (*os.File, error) {
					return nil, os.ErrNotExist
				}).MaxTimes(1)
				return mockOs
			}(),
			nfsService: func() *CsiNfsService {
				csiNFSService := &CsiNfsService{
					k8sclient: &k8s.Client{
						Clientset: fake.NewClientset(),
					},
				}

				// mocks for k8s clientset
				_, err := csiNFSService.k8sclient.CreateService(context.Background(), DriverNamespace, &v1.Service{
					ObjectMeta: metav1.ObjectMeta{Name: "", Annotations: map[string]string{
						"driverVolumeID": "00000000-0000-0000-0000-000000000001/RT-M0001/scsi",
					}},
				})
				if err != nil {
					t.Fatalf("failed to create fake nfs service: err: %s", err.Error())
				}

				return csiNFSService
			}(),
			createExportFile: func() *os.File {
				err := os.MkdirAll(exportsDir, os.ModePerm)
				if err != nil {
					t.Fatal(err)
				}
				err = os.MkdirAll("/tmp/noderoot/export 127.0.0.1(rw)", os.ModePerm)
				if err != nil {
					t.Fatal(err)
				}
				file, err := os.Create(pathToExports)
				if err != nil {
					t.Fatal(err)
				}
				_, err = file.WriteString("export 127.0.0.1(rw)\n")
				if err != nil {
					t.Fatal(err)
				}
				return file
			},
			deleteExportFile: func(file *os.File) {
				_ = file.Close()
				_ = os.RemoveAll(exportsDir)
				_ = os.RemoveAll("/tmp/noderoot/export 127.0.0.1(rw)")
			},
			expectedErr: nil,
		},

		{
			name: "fail to resync nfs-mountd",
			request: &proto.PingRequest{
				NodeIpAddress:  "127.0.0.1",
				DumpAllExports: true,
			},
			expected: &proto.PingResponse{
				Ready:  true,
				Status: "",
			},
			nfs: func() *nfsServer {
				return &nfsServer{}
			}(),
			executor: func() *mocks.MockExecutor {
				mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))

				// ResyncNFSMountd() fails to resync
				mockExecutor.EXPECT().ExecuteCommand(chroot, nodeRoot, exportfs, "-r", "-a").Return(
					[]byte{}, errors.New("failed to resync")).Times(2)
				return mockExecutor
			}(),
			osMock: func() *mocks.MockOSInterface {
				mockOs := mocks.NewMockOSInterface(gomock.NewController(t))
				// GetExports
				mockOs.EXPECT().Open(gomock.Any()).DoAndReturn(func(name string) (*os.File, error) {
					return os.Open(name)
				}).MaxTimes(2)

				// DeleteExport()
				mockOs.EXPECT().OpenFile(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(name string, flag int, perm os.FileMode) (*os.File, error) {
					return os.OpenFile(name, flag, perm)
				}).Times(1)
				return mockOs
			}(),
			nfsService: func() *CsiNfsService {
				csiNFSService := &CsiNfsService{
					k8sclient: &k8s.Client{
						Clientset: fake.NewClientset(),
					},
				}

				// mocks for k8s clientset
				_, err := csiNFSService.k8sclient.CreateService(context.Background(), DriverNamespace, &v1.Service{
					ObjectMeta: metav1.ObjectMeta{Name: "", Annotations: map[string]string{
						"driverVolumeID": "00000000-0000-0000-0000-000000000001/RT-M0001/scsi",
					}},
				})
				if err != nil {
					t.Fatalf("failed to create fake nfs service: err: %s", err.Error())
				}

				return csiNFSService
			}(),
			createExportFile: func() *os.File {
				err := os.MkdirAll(exportsDir, os.ModePerm)
				if err != nil {
					t.Fatal(err)
				}
				err = os.MkdirAll("/tmp/noderoot/export 127.0.0.1(rw)", os.ModePerm)
				if err != nil {
					t.Fatal(err)
				}
				file, err := os.Create(pathToExports)
				if err != nil {
					t.Fatal(err)
				}
				_, err = file.WriteString("export 127.0.0.1(rw)\n")
				if err != nil {
					t.Fatal(err)
				}
				return file
			},
			deleteExportFile: func(file *os.File) {
				_ = file.Close()
				_ = os.RemoveAll(exportsDir)
				_ = os.RemoveAll("/tmp/noderoot/export 127.0.0.1(rw)")
			},
			expectedErr: errors.New("failed to resync"),
		},

		{
			name: "success: dump all exports request",
			request: &proto.PingRequest{
				NodeIpAddress:  "127.0.0.1",
				DumpAllExports: true,
			},
			expected: &proto.PingResponse{
				Ready:  true,
				Status: "",
			},
			nfs: func() *nfsServer {
				return &nfsServer{}
			}(),
			executor: func() *mocks.MockExecutor {
				mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))

				// ResyncNFSMountd() resync
				mockExecutor.EXPECT().ExecuteCommand(chroot, nodeRoot, exportfs, "-r", "-a").Return(
					[]byte{}, nil).Times(1)
				return mockExecutor
			}(),
			osMock: func() *mocks.MockOSInterface {
				mockOs := mocks.NewMockOSInterface(gomock.NewController(t))
				// GetExports
				mockOs.EXPECT().Open(gomock.Any()).DoAndReturn(func(name string) (*os.File, error) {
					return os.Open(name)
				}).MaxTimes(2)

				// DeleteExport()
				mockOs.EXPECT().OpenFile(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(name string, flag int, perm os.FileMode) (*os.File, error) {
					return os.OpenFile(name, flag, perm)
				}).Times(1)
				return mockOs
			}(),
			nfsService: func() *CsiNfsService {
				csiNFSService := &CsiNfsService{
					k8sclient: &k8s.Client{
						Clientset: fake.NewClientset(),
					},
				}

				// mocks for k8s clientset
				_, err := csiNFSService.k8sclient.CreateService(context.Background(), DriverNamespace, &v1.Service{
					ObjectMeta: metav1.ObjectMeta{Name: "", Annotations: map[string]string{
						"driverVolumeID": "00000000-0000-0000-0000-000000000001/RT-M0001/scsi",
					}},
				})
				if err != nil {
					t.Fatalf("failed to create fake nfs service: err: %s", err.Error())
				}

				// mocks for csi-powerstore csi interface
				mockVcsi := mocks.NewMockService(gomock.NewController(t))
				mockVcsi.EXPECT().UnmountVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)

				csiNFSService.vcsi = mockVcsi

				return csiNFSService
			}(),
			createExportFile: func() *os.File {
				err := os.MkdirAll(exportsDir, os.ModePerm)
				if err != nil {
					t.Fatal(err)
				}
				err = os.MkdirAll("/tmp/noderoot/export 127.0.0.1(rw)", os.ModePerm)
				if err != nil {
					t.Fatal(err)
				}
				file, err := os.Create(pathToExports)
				if err != nil {
					t.Fatal(err)
				}
				_, err = file.WriteString("export 127.0.0.1(rw)\n")
				if err != nil {
					t.Fatal(err)
				}
				return file
			},
			deleteExportFile: func(file *os.File) {
				_ = file.Close()
				_ = os.RemoveAll(exportsDir)
				_ = os.RemoveAll("/tmp/noderoot/export 127.0.0.1(rw)")
			},
			expectedErr: nil,
		},

		{
			name: "fail: unable to unmount volume",
			request: &proto.PingRequest{
				NodeIpAddress:  "127.0.0.1",
				DumpAllExports: true,
			},
			expected: &proto.PingResponse{
				Ready:  false,
				Status: "",
			},
			nfs: func() *nfsServer {
				return &nfsServer{}
			}(),
			executor: func() *mocks.MockExecutor {
				mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))

				// ResyncNFSMountd() resync
				mockExecutor.EXPECT().ExecuteCommand(chroot, nodeRoot, exportfs, "-r", "-a").Return(
					[]byte{}, nil).Times(2)
				return mockExecutor
			}(),
			osMock: func() *mocks.MockOSInterface {
				mockOs := mocks.NewMockOSInterface(gomock.NewController(t))
				// GetExports
				mockOs.EXPECT().Open(gomock.Any()).DoAndReturn(func(name string) (*os.File, error) {
					return os.Open(name)
				}).AnyTimes()

				// DeleteExport() + AddExport()
				mockOs.EXPECT().OpenFile(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(name string, flag int, perm os.FileMode) (*os.File, error) {
					return os.OpenFile(name, flag, perm)
				}).Times(2)
				return mockOs
			}(),
			nfsService: func() *CsiNfsService {
				csiNFSService := &CsiNfsService{
					k8sclient: &k8s.Client{
						Clientset: fake.NewClientset(),
					},
				}

				// mocks for k8s clientset
				_, err := csiNFSService.k8sclient.CreateService(context.Background(), DriverNamespace, &v1.Service{
					ObjectMeta: metav1.ObjectMeta{Name: "", Annotations: map[string]string{
						"driverVolumeID": "00000000-0000-0000-0000-000000000001/RT-M0001/scsi",
					}},
				})
				if err != nil {
					t.Fatalf("failed to create fake nfs service: err: %s", err.Error())
				}

				// mocks for csi-powerstore csi interface
				mockVcsi := mocks.NewMockService(gomock.NewController(t))
				mockVcsi.EXPECT().UnmountVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(errors.New("failed to unmount"))

				csiNFSService.vcsi = mockVcsi

				return csiNFSService
			}(),
			createExportFile: func() *os.File {
				err := os.MkdirAll(exportsDir, os.ModePerm)
				if err != nil {
					t.Fatal(err)
				}
				err = os.MkdirAll("/tmp/noderoot/export 127.0.0.1(rw)", os.ModePerm)
				if err != nil {
					t.Fatal(err)
				}
				file, err := os.Create(pathToExports)
				if err != nil {
					t.Fatal(err)
				}
				_, err = file.WriteString("export 127.0.0.1(rw)\n")
				if err != nil {
					t.Fatal(err)
				}
				return file
			},
			deleteExportFile: func(file *os.File) {
				_ = file.Close()
				_ = os.RemoveAll(exportsDir)
				_ = os.RemoveAll("/tmp/noderoot/export 127.0.0.1(rw)")
			},
			expectedErr: errors.New("dumping all exports failed"),
		},
	}

	waitTime = 1 * time.Second
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			nfsService = tc.nfsService
			defer func() { nfsService = nil }()

			file := tc.createExportFile()
			NfsExportDirectory = "export"
			tc.nfs.executor = tc.executor
			GetLocalExecutor = func() Executor {
				return tc.executor
			}
			opSys = tc.osMock

			resp, err := tc.nfs.Ping(context.Background(), tc.request)

			if tc.expectedErr != nil {
				if !strings.Contains(err.Error(), tc.expectedErr.Error()) {
					t.Errorf("Expected error: %v, but got: %v", tc.expectedErr, err)
				}
			} else {
				if !errors.Is(err, tc.expectedErr) {
					t.Errorf("Expected error: %v, but got: %v", tc.expectedErr, err)
				}
			}
			if !reflect.DeepEqual(resp, tc.expected) {
				t.Fatalf("expected %v, got %v", tc.expected, resp)
			}
			tc.deleteExportFile(file)
		})
	}
}

func TestStartNfsServiceServer(t *testing.T) {
	tests := []struct {
		name      string
		listenErr error
		serveErr  error
		wantErr   bool
	}{
		{
			name:      "Successful startNfsServiceServer",
			listenErr: nil,
			serveErr:  nil,
			wantErr:   false,
		},
		{
			name:      "Error with ListenFunc",
			listenErr: fmt.Errorf("error with ListenFunc"),
			serveErr:  nil,
			wantErr:   true,
		},
		{
			name:      "Error with ServeFunc",
			listenErr: nil,
			serveErr:  fmt.Errorf("error with ServeFunc"),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listenFunc := func(_, _ string) (net.Listener, error) {
				if tt.listenErr != nil {
					return nil, tt.listenErr
				}
				return &mockListener{}, nil
			}
			serveFunc := func(_ *grpc.Server, _ net.Listener) error {
				if tt.serveErr != nil {
					return tt.serveErr
				}
				return nil
			}
			err := startNfsServiceServer("127.0.0.1", "9090", listenFunc, serveFunc)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestListen(t *testing.T) {
	tests := []struct {
		name    string
		address string
		port    string
		wantErr bool
	}{
		{
			name:    "Successful listen",
			address: "127.0.0.1",
			port:    "9090",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lis, err := listen(tt.address, tt.port)
			if (err != nil) != tt.wantErr {
				t.Errorf("listen() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && lis == nil {
				t.Errorf("listen() returned nil listener")
			}
		})
	}
}
