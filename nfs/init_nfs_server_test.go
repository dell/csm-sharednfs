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
	"errors"
	"testing"

	"github.com/dell/csm-hbnfs/nfs/mocks"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestInitializeNfsServer(t *testing.T) {

	tests := []struct {
		name        string
		nfsServer   *CsiNfsService
		expectedErr error
	}{
		{
			name: "Valid NFS server - all commands successful",
			nfsServer: func() *CsiNfsService {
				mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-server").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-mountd").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("cp", "/nfs.conf", "/noderoot/tmp/nfs.conf").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "cp", "/tmp/nfs.conf", "/etc/nfs.conf").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "enable", "nfs-server").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "start", "nfs-server").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-server").Times(1).Return([]byte("Active: active"), nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-mountd").Times(1).Return([]byte("Active: active"), nil)

				return &CsiNfsService{
					executor: mockExecutor,
				}
			}(),
			expectedErr: nil,
		},
		{
			name: "Server is active, no need to initialize",
			nfsServer: func() *CsiNfsService {
				mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-server").Times(1).Return([]byte("Active: active"), nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-mountd").Times(1).Return([]byte{}, nil)

				return &CsiNfsService{
					executor: mockExecutor,
				}
			}(),
			expectedErr: nil,
		},
		{
			name: "Error copying nfs.conf file",
			nfsServer: func() *CsiNfsService {
				mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-server").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-mountd").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("cp", "/nfs.conf", "/noderoot/tmp/nfs.conf").Times(1).Return([]byte{}, errors.New("error copying nfs.conf"))

				return &CsiNfsService{
					executor: mockExecutor,
				}
			}(),
			expectedErr: errors.New("error copying nfs.conf"),
		},
		{
			name: "Error with chroot copying nfs.conf file",
			nfsServer: func() *CsiNfsService {
				mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-server").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-mountd").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("cp", "/nfs.conf", "/noderoot/tmp/nfs.conf").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "cp", "/tmp/nfs.conf", "/etc/nfs.conf").Times(1).Return([]byte{}, errors.New("error copying nfs.conf"))

				return &CsiNfsService{
					executor: mockExecutor,
				}

			}(),
			expectedErr: errors.New("error copying nfs.conf"),
		},
		{
			name: "Error with chroot copying nfs.conf file",
			nfsServer: func() *CsiNfsService {
				mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-server").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-mountd").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("cp", "/nfs.conf", "/noderoot/tmp/nfs.conf").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "cp", "/tmp/nfs.conf", "/etc/nfs.conf").Times(1).Return([]byte{}, errors.New("error copying nfs.conf"))

				return &CsiNfsService{
					executor: mockExecutor,
				}

			}(),
			expectedErr: errors.New("error copying nfs.conf"),
		},
		{
			name: "Error with chroot copying nfs.conf file",
			nfsServer: func() *CsiNfsService {
				mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-server").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-mountd").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("cp", "/nfs.conf", "/noderoot/tmp/nfs.conf").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "cp", "/tmp/nfs.conf", "/etc/nfs.conf").Times(1).Return([]byte{}, errors.New("error copying nfs.conf"))

				return &CsiNfsService{
					executor: mockExecutor,
				}

			}(),
			expectedErr: errors.New("error copying nfs.conf"),
		},
		{
			name: "Error with chroot ssh enable nfs-server service",
			nfsServer: func() *CsiNfsService {
				mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-server").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-mountd").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("cp", "/nfs.conf", "/noderoot/tmp/nfs.conf").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "cp", "/tmp/nfs.conf", "/etc/nfs.conf").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "enable", "nfs-server").Times(1).Return([]byte{}, errors.New("error chroot ssh enable"))
				return &CsiNfsService{
					executor: mockExecutor,
				}

			}(),
			expectedErr: errors.New("error chroot ssh enable"),
		},
		{
			name: "Error with chroot ssh start nfs-server service",
			nfsServer: func() *CsiNfsService {
				mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-server").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-mountd").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("cp", "/nfs.conf", "/noderoot/tmp/nfs.conf").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "cp", "/tmp/nfs.conf", "/etc/nfs.conf").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "enable", "nfs-server").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "start", "nfs-server").Times(1).Return([]byte{}, errors.New("error chroot ssh start"))

				return &CsiNfsService{
					executor: mockExecutor,
				}

			}(),
			expectedErr: errors.New("error chroot ssh start"),
		},
		{
			name: "Error with chroot ssh status nfs-server service",
			nfsServer: func() *CsiNfsService {
				mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-server").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-mountd").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("cp", "/nfs.conf", "/noderoot/tmp/nfs.conf").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "cp", "/tmp/nfs.conf", "/etc/nfs.conf").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "enable", "nfs-server").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "start", "nfs-server").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-server").Times(1).Return([]byte{}, errors.New("error chroot ssh status"))

				return &CsiNfsService{
					executor: mockExecutor,
				}

			}(),
			expectedErr: errors.New("error chroot ssh status"),
		},
		{
			name: "Error with chroot ssh status nfs-mountd service",
			nfsServer: func() *CsiNfsService {
				mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-server").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-mountd").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("cp", "/nfs.conf", "/noderoot/tmp/nfs.conf").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "cp", "/tmp/nfs.conf", "/etc/nfs.conf").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "enable", "nfs-server").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "start", "nfs-server").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-server").Times(1).Return([]byte("Active: active"), nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-mountd").Times(1).Return([]byte{}, errors.New("error ssh status nfs-mountd"))

				return &CsiNfsService{
					executor: mockExecutor,
				}

			}(),
			expectedErr: errors.New("error ssh status nfs-mountd"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.nfsServer.initializeNfsServer()
			assert.Equal(t, test.expectedErr, err)
		})
	}
}
