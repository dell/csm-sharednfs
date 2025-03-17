package nfs

import (
	"context"
	"errors"
	"fmt"
	"github.com/dell/csm-hbnfs/nfs/mocks"
	"github.com/dell/csm-hbnfs/nfs/proto"
	"go.uber.org/mock/gomock"
	"os"
	"testing"
)

func TestExportMultipleNfsVolume(t *testing.T) {
	testCases := []struct {
		name         string
		request      *proto.ExportMultipleNfsVolumesRequest
		expectedResp *proto.ExportMultipleNfsVolumesResponse
		expectedErr  error
	}{
		{
			name: "Empty Volume Mount Path",
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
			expectedErr: nil,
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
			mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))
			mockExecutor.EXPECT().ExecuteCommand(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return([]byte{}, nil)

			nfsService = &CsiNfsService{
				vcsi: &CsiNfsService{
					executor: mockExecutor,
				},
			}
			
			service := mocks.NewMockService(gomock.NewController(t))
			service.EXPECT().MountVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("/noderoot/etc/exports", nil)
			nfsService.vcsi = service

			nfs := &nfsServer{
				executor: mockExecutor,
			}
			_, err = nfs.ExportMultipleNfsVolumes(context.Background(), tc.request)
			_ = file.Close()
			_ = os.RemoveAll("/noderoot/etc/")

			if !errors.Is(err, tc.expectedErr) {
				t.Errorf("Expected error: %v, but got: %v", tc.expectedErr, err)
			}
		})
	}
}

func TestUnExportMultipleNfsVolume(t *testing.T) {
	testCases := []struct {
		name        string
		request     *proto.UnexportMultipleNfsVolumesRequest
		expected    *proto.UnexportMultipleNfsVolumesResponse
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
			expectedErr: nil,
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

			service := mocks.NewMockService(gomock.NewController(t))
			service.EXPECT().UnmountVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1).Return(nil)
			nfsService.vcsi = service

			nfs := &nfsServer{}
			_, err = nfs.UnexportMultipleNfsVolumes(context.Background(), tc.request)
			_ = file.Close()
			_ = os.RemoveAll("/noderoot/etc/")

			if !errors.Is(err, tc.expectedErr) {
				t.Errorf("Expected error: %v, but got: %v", tc.expectedErr, err)
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
			createExportFile: func() (file *os.File) {
				return nil
			},
			deleteExportFile: func(_ *os.File) {},
			expectedErr:      fmt.Errorf("timeout reached: nfs-mountd did not restart within 30s"),
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
			expectedErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			file := tc.createExportFile()
			NfsExportDirectory = "export"
			_, _ = tc.nfs.Ping(context.Background(), tc.request)
			tc.deleteExportFile(file)
		})
	}
}
