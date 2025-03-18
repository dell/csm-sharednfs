package nfs

import (
	"context"
	"errors"
	"fmt"
	"github.com/dell/csm-hbnfs/nfs/mocks"
	"github.com/dell/csm-hbnfs/nfs/proto"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"net"
	"os"
	"reflect"
	"testing"
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
	testCases := []struct {
		name         string
		request      *proto.ExportMultipleNfsVolumesRequest
		expectedResp *proto.ExportMultipleNfsVolumesResponse
		service      *mocks.MockService
		executor     *mocks.MockExecutor
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
				service.EXPECT().MountVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("/noderoot/etc/exports", nil)
				return service
			}(),

			executor: func() *mocks.MockExecutor {
				mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))
				mockExecutor.EXPECT().ExecuteCommand(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return([]byte{}, nil)
				return mockExecutor
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
				service.EXPECT().MountVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("/noderoot/etc/exports", fmt.Errorf("failed to mount volume"))
				return service
			}(),
			executor: func() *mocks.MockExecutor {
				mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))
				return mockExecutor
			}(),
			expectedErr: fmt.Errorf("failed to mount volume"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			err := os.MkdirAll("/noderoot/etc", os.ModePerm)
			if err != nil {
				t.Fatal(err)
			}
			file, err := os.Create("/noderoot/etc/exports")
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
			_, err = nfs.ExportMultipleNfsVolumes(context.Background(), tc.request)
			_ = file.Close()
			_ = os.RemoveAll("/noderoot/etc/")

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
	testCases := []struct {
		name        string
		request     *proto.UnexportMultipleNfsVolumesRequest
		expected    *proto.UnexportMultipleNfsVolumesResponse
		service     *mocks.MockService
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
				service.EXPECT().UnmountVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)
				return service
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
				service.EXPECT().UnmountVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(3).Return(fmt.Errorf("failed to unmount"))
				return service
			}(),
			expectedErr: fmt.Errorf("failed to unmount"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := os.MkdirAll("/noderoot/etc/", os.ModePerm)
			if err != nil {
				t.Fatal(err)
			}
			file, err := os.Create("/noderoot/etc/exports")
			if err != nil {
				t.Fatal(err)
			}
			_, err = file.WriteString("nfs exports")
			if err != nil {
				t.Fatal(err)
			}

			mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))
			nfsService = &CsiNfsService{
				vcsi: &CsiNfsService{
					executor: mockExecutor,
				},
			}

			nfsService.vcsi = tc.service
			nfs := &nfsServer{}
			_, err = nfs.UnexportMultipleNfsVolumes(context.Background(), tc.request)
			_ = file.Close()
			_ = os.RemoveAll("/noderoot/etc/")

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
	err := os.MkdirAll("/noderoot/etc/", os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Create("/noderoot/etc/exports")
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteString("export 127.0.0.1(rw)\n")
	if err != nil {
		t.Fatal(err)
	}

	getExportsRequest := &proto.GetExportsRequest{}
	nfs := nfsServer{}
	_, err = nfs.GetExports(context.Background(), getExportsRequest)
	_ = file.Close()
	_ = os.RemoveAll("/noderoot/etc/")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNFSPing(t *testing.T) {
	testCases := []struct {
		name             string
		request          *proto.PingRequest
		expected         *proto.PingResponse
		nfs              *nfsServer
		executor         *mocks.MockExecutor
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
			createExportFile: func() (file *os.File) {
				return nil
			},
			deleteExportFile: func(_ *os.File) {},
			expectedErr:      nil,
		},
		{
			name: "True DumpAllExports",
			request: &proto.PingRequest{
				NodeIpAddress:  "127.0.0.1",
				DumpAllExports: true,
			},
			expected: &proto.PingResponse{
				Ready:  true,
				Status: "",
			},
			nfs: func() *nfsServer {
				mockUnmounter := mocks.NewMockUnmounter(gomock.NewController(t))
				mockUnmounter.EXPECT().Unmount(gomock.Any(), gomock.Any()).Return(nil)
				return &nfsServer{
					unmounter: mockUnmounter,
				}
			}(),
			executor: func() *mocks.MockExecutor {
				mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))
				mockExecutor.EXPECT().ExecuteCommand(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return([]byte{}, nil)
				return mockExecutor
			}(),
			createExportFile: func() *os.File {
				err := os.MkdirAll("/noderoot/etc/", os.ModePerm)
				if err != nil {
					t.Fatal(err)
				}
				err = os.MkdirAll("/noderoot/export 127.0.0.1(rw)", os.ModePerm)
				if err != nil {
					t.Fatal(err)
				}
				file, err := os.Create("/noderoot/etc/exports")
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
				_ = os.RemoveAll("/noderoot/etc/")
				_ = os.RemoveAll("/noderoot/export 127.0.0.1(rw)")
			},
			expectedErr: fmt.Errorf("timeout reached: nfs-mountd did not restart within 30s\n"),
		},
		{
			name: "No exports File",
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
			createExportFile: func() (file *os.File) {
				return nil
			},
			deleteExportFile: func(_ *os.File) {},
			expectedErr:      fmt.Errorf("open /noderoot/etc/exports: no such file or directory"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			file := tc.createExportFile()
			NfsExportDirectory = "export"
			tc.nfs.executor = tc.executor
			resp, err := tc.nfs.Ping(context.Background(), tc.request)

			if tc.expectedErr != nil {
				if tc.expectedErr.Error() != err.Error() {
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
			listenFunc := func(network, address string) (net.Listener, error) {
				if tt.listenErr != nil {
					return nil, tt.listenErr
				}
				return &mockListener{}, nil
			}
			serveFunc := func(s *grpc.Server, lis net.Listener) error {
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
