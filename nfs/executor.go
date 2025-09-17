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
	"os/exec"
)

//go:generate mockgen -destination=mocks/executor.go -package=mocks github.com/dell/csm-sharednfs/nfs Executor
type Executor interface {
	ExecuteCommand(name string, args ...string) ([]byte, error)
	ExecuteCommandContext(context context.Context, name string, args ...string) ([]byte, error)
	GetCombinedOutput(cmd *exec.Cmd) ([]byte, error)
}

type LocalExecutor struct{}

func (l *LocalExecutor) ExecuteCommand(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

func (l *LocalExecutor) ExecuteCommandContext(context context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(context, name, args...).Output()
}

func (l *LocalExecutor) Run(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

func (l *LocalExecutor) GetCombinedOutput(cmd *exec.Cmd) ([]byte, error) {
	return cmd.CombinedOutput()
}
