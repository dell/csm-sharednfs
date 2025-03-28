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
	"os"
	"strings"
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
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh-keyscan", "-t", "rsa,ecdsa,ed25519", "localhost").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-server").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-mountd").Times(1).Return([]byte{}, nil)
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
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh-keyscan", "-t", "rsa,ecdsa,ed25519", "localhost").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-server").Times(1).Return([]byte("Active: active"), nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-mountd").Times(1).Return([]byte{}, nil)

				return &CsiNfsService{
					executor: mockExecutor,
				}
			}(),
			expectedErr: nil,
		},
		{
			name: "Error with chroot ssh enable nfs-server service",
			nfsServer: func() *CsiNfsService {
				mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh-keyscan", "-t", "rsa,ecdsa,ed25519", "localhost").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-server").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-mountd").Times(1).Return([]byte{}, nil)
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
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh-keyscan", "-t", "rsa,ecdsa,ed25519", "localhost").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-server").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-mountd").Times(1).Return([]byte{}, nil)
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
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh-keyscan", "-t", "rsa,ecdsa,ed25519", "localhost").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-server").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-mountd").Times(1).Return([]byte{}, nil)
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
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh-keyscan", "-t", "rsa,ecdsa,ed25519", "localhost").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-server").Times(1).Return([]byte{}, nil)
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh", "localhost", "systemctl", "status", "nfs-mountd").Times(1).Return([]byte{}, nil)
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
			knownHostsFile := `# localhost:22 SSH-2.0-OpenSSH_8.4
			localhost ssh-rsa OLD_KEY
			localhost ecdsa-sha2-nistp256 OLD_ECDSA_KEY
			localhost ssh-ed25519 OLD_ED25519_KEY
			`
			var err error
			knownHostsPath, err = setupKnownHosts(strings.ReplaceAll(knownHostsFile, "\t", ""))
			if err != nil {
				t.Fatalf("failed to set up known_hosts file: %v", err)
			}
			defer os.Remove(knownHostsPath)
			err = test.nfsServer.initializeNfsServer()
			assert.Equal(t, test.expectedErr, err)
		})
	}
}

func setupKnownHosts(content string) (string, error) {
	tmpFile, err := os.CreateTemp("", "known_hosts")
	if err != nil {
		return "", err
	}
	if _, err := tmpFile.WriteString(content); err != nil {
		return "", err
	}
	return tmpFile.Name(), nil
}

func TestUpdateKnownHosts(t *testing.T) {
	tests := []struct {
		name          string
		initialHosts  string
		expectedHosts string
		service       *CsiNfsService
	}{
		{
			name: "Update existing ssh-rsa key",
			service: func() *CsiNfsService {
				mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))
				keyscanOutput := `localhost ssh-rsa NEW_RSA_KEY
				localhost ecdsa-sha2-nistp256 NEW_ECDSA_KEY
				localhost ssh-ed25519 NEW_ED25519_KEY`

				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh-keyscan", "-t", "rsa,ecdsa,ed25519", "localhost").Times(1).Return([]byte(keyscanOutput), nil)

				return &CsiNfsService{
					executor: mockExecutor,
				}
			}(),
			initialHosts: `# localhost:22 SSH-2.0-OpenSSH_8.4
			localhost ssh-rsa OLD_KEY
			localhost ecdsa-sha2-nistp256 OLD_ECDSA_KEY
			localhost ssh-ed25519 OLD_ED25519_KEY
			`,

			expectedHosts: strings.ReplaceAll(string(`# localhost:22 SSH-2.0-OpenSSH_8.4
			localhost ssh-rsa NEW_RSA_KEY
			localhost ecdsa-sha2-nistp256 NEW_ECDSA_KEY
			localhost ssh-ed25519 NEW_ED25519_KEY
			`), "\t", ""),
		},
		{
			name: "Add new keys",
			service: func() *CsiNfsService {
				mockExecutor := mocks.NewMockExecutor(gomock.NewController(t))
				keyscanOutput := `localhost ssh-rsa NEW_RSA_KEY
				localhost ecdsa-sha2-nistp256 NEW_ECDSA_KEY
				localhost ssh-ed25519 NEW_ED25519_KEY
				`
				mockExecutor.EXPECT().ExecuteCommand("chroot", "/noderoot", "ssh-keyscan", "-t", "rsa,ecdsa,ed25519", "localhost").Times(1).Return([]byte(keyscanOutput), nil)

				return &CsiNfsService{
					executor: mockExecutor,
				}
			}(),
			initialHosts: `# localhost:22 SSH-2.0-OpenSSH_8.4`,
			expectedHosts: strings.ReplaceAll(string(`# localhost:22 SSH-2.0-OpenSSH_8.4
			localhost ssh-rsa NEW_RSA_KEY
			localhost ecdsa-sha2-nistp256 NEW_ECDSA_KEY
			localhost ssh-ed25519 NEW_ED25519_KEY
			`), "\t", ""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up the known_hosts file
			var err error
			knownHostsPath, err = setupKnownHosts(strings.ReplaceAll(tt.initialHosts, "\t", ""))
			if err != nil {
				t.Fatalf("failed to set up known_hosts file: %v", err)
			}
			defer os.Remove(knownHostsPath)

			// Run the updateKnownHosts function
			err = tt.service.updateKnownHosts()
			if err != nil {
				t.Fatalf("updateKnownHosts() error : %v ", err)
			}

			// Read the updated known_hosts file
			updatedHosts, err := os.ReadFile(knownHostsPath)
			if err != nil {
				t.Fatalf("failed to read updated known_hosts file: %v", err)
			}

			// Compare the updated known_hosts file with the expected content
			if err != nil && string(updatedHosts) != tt.expectedHosts {
				t.Errorf("updateKnownHosts() failed :\n got = %v\n, want %v", string(updatedHosts), string(tt.expectedHosts))
			}
		})
	}
}
