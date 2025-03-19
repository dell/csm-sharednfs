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
	"bufio"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/dell/csm-hbnfs/nfs/mocks"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

var bufioMutex sync.Mutex

func TestCheckExport(t *testing.T) {
	exportsDir = "/tmp/noderoot/etc/"
	exportsFile = "exports"
	pathToExports = exportsDir + exportsFile
	defaultGetBufioScanner := GetBufioScanner
	tests := []struct {
		name      string
		setup     func(dir string, fileName string) (*os.File, error)
		teardown  func(dir string, file *os.File)
		directory string
		fileName  string
		file      *os.File
		want      bool
		wantErr   bool
	}{
		{
			name:      "directory exists in /noderoot/etc/exports",
			directory: exportsDir,
			fileName:  exportsFile,
			setup: func(dir string, fileName string) (*os.File, error) {
				err := os.MkdirAll(dir, 0o755)
				if err != nil {
					t.Errorf("failed to create temp directory: %v", err)
					return nil, errors.New("failed to create temp directory")
				}

				// Create a mock /noderoot/etc/exports file
				file, err := os.Create(dir + fileName)
				if err != nil {
					t.Errorf("failed to create temp file: %v", err)
					return nil, errors.New("failed to create temp directory")
				}

				// Write test data to the file
				_, err = file.WriteString(fmt.Sprintf("%s\n", dir+fileName))
				if err != nil {
					t.Fatal(err)
				}

				file.Close()

				return file, nil
			},
			teardown: func(dir string, file *os.File) {
				if file != nil {
					file.Close()
				}
				os.Remove(file.Name())
				os.RemoveAll(dir)
				GetBufioScanner = defaultGetBufioScanner
			},
			want:    true,
			wantErr: false,
		},
		{
			name:      "error opening /noderoot/etc/exports",
			directory: "/error/directory",
			fileName:  exportsFile,
			setup: func(_ string, _ string) (*os.File, error) {
				return nil, nil
			},
			teardown: func(_ string, _ *os.File) {
				GetBufioScanner = defaultGetBufioScanner
			},
			want:    false,
			wantErr: true,
		},
		{
			name:      "bufio scanner error reading /noderoot/etc/exports",
			directory: exportsDir,
			fileName:  exportsFile,
			setup: func(dir string, fileName string) (*os.File, error) {
				err := os.MkdirAll(dir, 0o755)
				if err != nil {
					t.Errorf("failed to create temp directory: %v", err)
					return nil, errors.New("failed to create temp directory")
				}

				// Create a mock /noderoot/etc/exports file
				file, err := os.Create(pathToExports)
				if err != nil {
					t.Errorf("failed to create temp file: %v", err)
					return nil, errors.New("failed to create temp directory")
				}

				// Write test data to the file
				_, err = file.WriteString(fmt.Sprintf("%s\n", dir+fileName))
				if err != nil {
					t.Fatal(err)
				}

				file.Close()
				GetBufioScanner = func(file *os.File) *bufio.Scanner {
					bufioMutex.Lock()
					defer bufioMutex.Unlock()
					scanner := bufio.NewScanner(file)
					// close the file before returning to induce scanner error
					file.Close()
					return scanner
				}

				return file, nil
			},
			teardown: func(dir string, file *os.File) {
				if file != nil {
					file.Close()
				}
				os.Remove(file.Name())
				os.RemoveAll(dir)
				GetBufioScanner = defaultGetBufioScanner
			},
			want:    false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			tt.file, err = tt.setup(tt.directory, tt.fileName)
			defer tt.teardown(tt.directory, tt.file)

			// Call the checkExport function
			result, err := CheckExport(tt.directory)

			// Check the result and error
			if (err != nil) != tt.wantErr {
				t.Errorf("checkExport() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestGetExport(t *testing.T) {
	exportsDir = "/tmp/noderoot/etc/"
	exportsFile = "exports"
	pathToExports = exportsDir + exportsFile
	defaultGetBufioScanner := GetBufioScanner
	tests := []struct {
		name      string
		setup     func(dir string, fileName string) (*os.File, error)
		teardown  func(dir string, file *os.File)
		directory string
		fileName  string
		file      *os.File
		want      bool
		wantErr   bool
	}{
		{
			name:      "directory exists in /noderoot/etc/exports",
			directory: exportsDir,
			fileName:  exportsFile,
			setup: func(dir string, fileName string) (*os.File, error) {
				err := os.MkdirAll(dir, 0o755)
				if err != nil {
					t.Errorf("failed to create temp directory: %v", err)
					return nil, errors.New("failed to create temp directory")
				}

				// Create a mock /noderoot/etc/exports file
				file, err := os.Create(dir + fileName)
				if err != nil {
					t.Errorf("failed to create temp file: %v", err)
					return nil, errors.New("failed to create temp directory")
				}

				// Write test data to the file
				_, err = file.WriteString(fmt.Sprintf("%s\n", dir+fileName))
				if err != nil {
					t.Fatal(err)
				}

				file.Close()

				return file, nil
			},
			teardown: func(dir string, file *os.File) {
				if file != nil {
					file.Close()
				}
				os.Remove(file.Name())
				os.RemoveAll(dir)
				GetBufioScanner = defaultGetBufioScanner
			},
			want:    true,
			wantErr: false,
		},
		{
			name:      "error opening /noderoot/etc/exports",
			directory: "/error/directory",
			fileName:  "exports",
			setup: func(_ string, _ string) (*os.File, error) {
				return nil, nil
			},
			teardown: func(_ string, _ *os.File) {
				GetBufioScanner = defaultGetBufioScanner
			},
			want:    false,
			wantErr: true,
		},
		{
			name:      "scanner error reading /noderoot/etc/exports",
			directory: exportsDir,
			fileName:  exportsFile,
			setup: func(dir string, fileName string) (*os.File, error) {
				err := os.MkdirAll(dir, 0o755)
				if err != nil {
					t.Errorf("failed to create temp directory: %v", err)
					return nil, errors.New("failed to create temp directory")
				}

				// Create a mock /noderoot/etc/exports file
				file, err := os.Create(dir + fileName)
				if err != nil {
					t.Errorf("failed to create temp file: %v", err)
					return nil, errors.New("failed to create temp directory")
				}

				// Close the file handle to simulate an I/O error
				file.Close()

				GetBufioScanner = func(file *os.File) *bufio.Scanner {
					bufioMutex.Lock()
					defer bufioMutex.Unlock()
					scanner := bufio.NewScanner(file)
					// close the file before returning to induce scanner error
					file.Close()
					return scanner
				}

				return file, nil
			},
			teardown: func(dir string, file *os.File) {
				if file != nil {
					file.Close()
				}
				os.Remove(file.Name())
				os.RemoveAll(dir)
				GetBufioScanner = defaultGetBufioScanner
			},
			want:    false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			tt.file, err = tt.setup(tt.directory, tt.fileName)
			defer tt.teardown(tt.directory, tt.file)

			// Call the GetExport function
			result, err := GetExport(tt.directory)

			// Check the result and error
			if (err != nil) != tt.wantErr {
				t.Errorf("GetExport() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			got := result != ""
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetNFSExports(t *testing.T) {
	exportsDir = "/tmp/noderoot/etc/"
	exportsFile = "exports"
	pathToExports = exportsDir + exportsFile
	defaultGetBufioScanner := GetBufioScanner
	tests := []struct {
		name      string
		setup     func(dir string, fileName string) (*os.File, error)
		teardown  func(dir string, file *os.File)
		directory string
		fileName  string
		file      *os.File
		want      bool
		wantErr   bool
	}{
		{
			name:      "directory exists in /noderoot/etc/exports",
			directory: exportsDir,
			fileName:  exportsFile,
			setup: func(dir string, fileName string) (*os.File, error) {
				err := os.MkdirAll(dir, 0o755)
				if err != nil {
					t.Errorf("failed to create temp directory: %v", err)
					return nil, errors.New("failed to create temp directory")
				}

				// Create a mock /noderoot/etc/exports file
				file, err := os.Create(dir + fileName)
				if err != nil {
					t.Errorf("failed to create temp file: %v", err)
					return nil, errors.New("failed to create temp directory")
				}

				// Write test data to the file
				_, err = file.WriteString(fmt.Sprintf("%s\n", dir+fileName))
				if err != nil {
					t.Fatal(err)
				}

				file.Close()

				return file, nil
			},
			teardown: func(dir string, file *os.File) {
				if file != nil {
					file.Close()
				}
				os.Remove(file.Name())
				os.RemoveAll(dir)
				GetBufioScanner = defaultGetBufioScanner
			},
			want:    true,
			wantErr: false,
		},
		{
			name:      "error opening /noderoot/etc/exports",
			directory: "/error/directory",
			fileName:  "exports",
			setup: func(_ string, _ string) (*os.File, error) {
				return nil, nil
			},
			teardown: func(_ string, _ *os.File) {
				GetBufioScanner = defaultGetBufioScanner
			},
			want:    false,
			wantErr: true,
		},
		{
			name:      "scanner error reading /noderoot/etc/exports",
			directory: exportsDir,
			fileName:  exportsFile,
			setup: func(dir string, fileName string) (*os.File, error) {
				err := os.MkdirAll(dir, 0o755)
				if err != nil {
					t.Errorf("failed to create temp directory: %v", err)
					return nil, errors.New("failed to create temp directory")
				}

				// Create a mock /noderoot/etc/exports file
				file, err := os.Create(dir + fileName)
				if err != nil {
					t.Errorf("failed to create temp file: %v", err)
					return nil, errors.New("failed to create temp directory")
				}

				// Write test data to the file
				_, err = file.WriteString(fmt.Sprintf("%s\n", dir+fileName))
				if err != nil {
					t.Fatal(err)
				}

				file.Close()
				GetBufioScanner = func(file *os.File) *bufio.Scanner {
					bufioMutex.Lock()
					defer bufioMutex.Unlock()
					scanner := bufio.NewScanner(file)
					// close the file before returning to induce scanner error
					file.Close()
					return scanner
				}

				return file, nil
			},
			teardown: func(dir string, file *os.File) {
				if file != nil {
					file.Close()
				}
				os.Remove(file.Name())
				os.RemoveAll(dir)
				GetBufioScanner = defaultGetBufioScanner
			},
			want:    false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			tt.file, err = tt.setup(tt.directory, tt.fileName)
			defer tt.teardown(tt.directory, tt.file)

			// Call the GetExports function
			result, err := GetExports(tt.directory)

			// Check the result and error
			if (err != nil) != tt.wantErr {
				t.Errorf("GetExports() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			got := len(result) > 0
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAddExport(t *testing.T) {
	defaultGetBufioScanner := GetBufioScanner
	exportsDir = "/tmp/noderoot/etc/"
	exportsFile = "exports"
	pathToExports = exportsDir + exportsFile
	tests := []struct {
		name      string
		setup     func(dir string, fileName string) (*os.File, error)
		teardown  func(dir string, file *os.File)
		directory string
		fileName  string
		file      *os.File
		want      bool
		wantErr   bool
	}{
		{
			name:      "export does not exist in /noderoot/etc/exports",
			directory: exportsDir,
			fileName:  exportsFile,
			setup: func(dir string, fileName string) (*os.File, error) {
				err := os.MkdirAll(dir, 0o755)
				if err != nil {
					t.Errorf("failed to create temp directory: %v", err)
					return nil, errors.New("failed to create temp directory")
				}

				// Create a mock /noderoot/etc/exports file
				file, err := os.Create(dir + fileName)
				if err != nil {
					t.Errorf("failed to create temp file: %v", err)
					return nil, errors.New("failed to create temp directory")
				}
				return file, nil
			},
			teardown: func(dir string, file *os.File) {
				if file != nil {
					file.Close()
				}
				os.Remove(file.Name())
				os.RemoveAll(dir)
				GetBufioScanner = defaultGetBufioScanner
			},
			want:    true,
			wantErr: false,
		},
		{
			name:      "export exist in /noderoot/etc/exports",
			directory: exportsDir,
			fileName:  exportsFile,
			setup: func(dir string, fileName string) (*os.File, error) {
				err := os.MkdirAll(dir, 0o755)
				if err != nil {
					t.Errorf("failed to create temp directory: %v", err)
					return nil, errors.New("failed to create temp directory")
				}

				// Create a mock /noderoot/etc/exports file
				file, err := os.Create(dir + fileName)
				if err != nil {
					t.Errorf("failed to create temp file: %v", err)
					return nil, errors.New("failed to create temp directory")
				}

				// Write test data to the file
				_, err = file.WriteString(fmt.Sprintf("%s\n", dir+fileName))
				if err != nil {
					t.Fatal(err)
				}

				file.Close()
				return file, nil
			},
			teardown: func(dir string, file *os.File) {
				if file != nil {
					file.Close()
				}
				os.Remove(file.Name())
				os.RemoveAll(dir)
				GetBufioScanner = defaultGetBufioScanner
			},
			want:    true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			tt.file, err = tt.setup(tt.directory, tt.fileName)
			defer tt.teardown(tt.directory, tt.file)

			// Call the GetExports function
			result, err := AddExport(tt.directory, "a.b.c.d/24(rw)")

			// Check the result and error
			if (err != nil) != tt.wantErr {
				t.Errorf("AddExport() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			got := result > 0
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsNfsMountdActive(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name       string
		mockReturn []byte
		mockError  error
		expected   bool
	}{
		{
			name:       "nfs-mountd is active",
			mockReturn: []byte{},
			mockError:  nil,
			expected:   true,
		},
		{
			name:       "nfs-mountd is not active",
			mockReturn: []byte{},
			mockError:  errors.New("nfs-mountd not active"),
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			me := mocks.NewMockExecutor(ctrl)
			GetLocalExecutor = func() Executor {
				return me
			}
			me.EXPECT().ExecuteCommand("chroot", "/noderoot", "container-systemctl", "is-active", "--quiet", "nfs-mountd").Return(tt.mockReturn, tt.mockError)

			value := isNfsMountdActive()
			assert.Equal(t, tt.expected, value)
		})
	}
}

func TestResyncNFSMountd(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name          string
		generation    int64
		resyncg       int64
		mockReturn    []byte
		mockError     error
		expectedError error
	}{
		{
			name:          "generation already synced",
			generation:    1,
			mockReturn:    nil,
			mockError:     nil,
			expectedError: nil,
		},
		{
			name:          "resync successful",
			generation:    2,
			mockReturn:    []byte{},
			mockError:     nil,
			expectedError: nil,
		},
		{
			name:          "resync failed",
			generation:    3,
			mockReturn:    []byte{},
			mockError:     errors.New("resync failed"),
			expectedError: errors.New("resync failed"),
		},
		{
			name:          "resync generation greater than generation failed",
			generation:    3,
			resyncg:       4,
			mockReturn:    []byte{},
			mockError:     nil,
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			me := mocks.NewMockExecutor(ctrl)
			GetLocalExecutor = func() Executor {
				return me
			}
			retrySleep = 10 * time.Millisecond
			me.EXPECT().ExecuteCommand(chroot, nodeRoot, exportfs, "-r", "-a").Return(tt.mockReturn, tt.mockError).AnyTimes()

			if tt.resyncg != 0 {
				syncedGeneration = tt.resyncg
			}
			err := ResyncNFSMountd(tt.generation)
			assert.Equal(t, tt.expectedError, err)
		})
	}
}

func TestDeleteExport(t *testing.T) {
	defaultGetBufioScanner := GetBufioScanner
	exportsDir = "/tmp/noderoot/etc/"
	exportsFile = "exports"
	pathToExports = exportsDir + exportsFile
	tests := []struct {
		name      string
		setup     func(dir string, fileName string) (*os.File, error)
		teardown  func(dir string, file *os.File)
		directory string
		fileName  string
		file      *os.File
		want      bool
		wantErr   bool
	}{
		{
			name:      "export exists in /noderoot/etc/exports",
			directory: exportsDir,
			fileName:  exportsFile,
			setup: func(dir string, fileName string) (*os.File, error) {
				err := os.MkdirAll(dir, 0o755)
				if err != nil {
					t.Errorf("failed to create temp directory: %v", err)
					return nil, errors.New("failed to create temp directory")
				}

				// Create a mock /noderoot/etc/exports file
				file, err := os.Create(dir + fileName)
				if err != nil {
					t.Errorf("failed to create temp file: %v", err)
					return nil, errors.New("failed to create temp directory")
				}

				// Write test data to the file
				_, err = file.WriteString(fmt.Sprintf("%s\n", dir+fileName))
				if err != nil {
					t.Fatal(err)
				}
				_, err = file.WriteString(fmt.Sprintf("%s\n", "dummy-entry"))
				if err != nil {
					t.Fatal(err)
				}

				file.Close()

				return file, nil
			},
			teardown: func(dir string, file *os.File) {
				if file != nil {
					file.Close()
				}
				os.Remove(file.Name())
				os.RemoveAll(dir)
				GetBufioScanner = defaultGetBufioScanner
			},
			want:    true,
			wantErr: false,
		},
		{
			name:      "error opening /noderoot/etc/exports",
			directory: exportsDir,
			fileName:  exportsFile,
			setup: func(_ string, _ string) (*os.File, error) {
				return nil, nil
			},
			teardown: func(_ string, _ *os.File) {
				GetBufioScanner = defaultGetBufioScanner
			},
			want:    true,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			tt.file, err = tt.setup(tt.directory, tt.fileName)
			defer tt.teardown(tt.directory, tt.file)

			// Call the GetExports function
			result, err := DeleteExport(tt.directory)

			// Check the result and error
			if (err != nil) != tt.wantErr {
				t.Errorf("DeleteExport() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			got := result > 0
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRestartNFSMountd(t *testing.T) {
	tests := []struct {
		name                 string
		wait                 time.Duration
		mockReturn           []byte
		mockRestartNFSError  error
		mockIsNFSActiveError error
		expectedError        error
	}{
		{
			name:                 "successful restart",
			mockReturn:           []byte{},
			wait:                 4 * time.Second,
			mockRestartNFSError:  nil,
			mockIsNFSActiveError: nil,
			expectedError:        nil,
		},
		{
			name:                 "restart failed within time",
			mockReturn:           []byte{},
			wait:                 1 * time.Second,
			mockRestartNFSError:  nil,
			mockIsNFSActiveError: errors.New("nfs-mountd is not active"),
			expectedError:        errors.New("timeout reached: nfs-mountd did not restart within"),
		},
		{
			name:                 "execution error",
			mockReturn:           []byte{},
			wait:                 4 * time.Second,
			mockRestartNFSError:  errors.New("failed to restart nfs-mountd"),
			mockIsNFSActiveError: nil,
			expectedError:        errors.New("failed to restart nfs-mountd"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			me := mocks.NewMockExecutor(gomock.NewController(t))
			GetLocalExecutor = func() Executor {
				return me
			}
			waitTime = tt.wait
			me.EXPECT().ExecuteCommand("chroot", "/noderoot", "container-systemctl", "restart", "nfs-mountd").Return(tt.mockReturn, tt.mockRestartNFSError)
			me.EXPECT().ExecuteCommand("chroot", "/noderoot", "container-systemctl", "is-active", "--quiet", "nfs-mountd").Return(tt.mockReturn, tt.mockIsNFSActiveError).AnyTimes()

			GetLocalExecutor = func() Executor {
				return me
			}

			err := restartNFSMountd()
			if err != nil {
				assert.ErrorContains(t, err, tt.expectedError.Error())
			}
		})
	}
}
