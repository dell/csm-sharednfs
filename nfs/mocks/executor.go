// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/dell/csm-sharednfs/nfs (interfaces: Executor)
//
// Generated by this command:
//
//	mockgen -destination=mocks/executor.go -package=mocks github.com/dell/csm-sharednfs/nfs Executor
//

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	exec "os/exec"
	reflect "reflect"

	gomock "go.uber.org/mock/gomock"
)

// MockExecutor is a mock of Executor interface.
type MockExecutor struct {
	ctrl     *gomock.Controller
	recorder *MockExecutorMockRecorder
	isgomock struct{}
}

// MockExecutorMockRecorder is the mock recorder for MockExecutor.
type MockExecutorMockRecorder struct {
	mock *MockExecutor
}

// NewMockExecutor creates a new mock instance.
func NewMockExecutor(ctrl *gomock.Controller) *MockExecutor {
	mock := &MockExecutor{ctrl: ctrl}
	mock.recorder = &MockExecutorMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockExecutor) EXPECT() *MockExecutorMockRecorder {
	return m.recorder
}

// ExecuteCommand mocks base method.
func (m *MockExecutor) ExecuteCommand(name string, args ...string) ([]byte, error) {
	m.ctrl.T.Helper()
	varargs := []any{name}
	for _, a := range args {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "ExecuteCommand", varargs...)
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ExecuteCommand indicates an expected call of ExecuteCommand.
func (mr *MockExecutorMockRecorder) ExecuteCommand(name any, args ...any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]any{name}, args...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ExecuteCommand", reflect.TypeOf((*MockExecutor)(nil).ExecuteCommand), varargs...)
}

// ExecuteCommandContext mocks base method.
func (m *MockExecutor) ExecuteCommandContext(context context.Context, name string, args ...string) ([]byte, error) {
	m.ctrl.T.Helper()
	varargs := []any{context, name}
	for _, a := range args {
		varargs = append(varargs, a)
	}
	ret := m.ctrl.Call(m, "ExecuteCommandContext", varargs...)
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ExecuteCommandContext indicates an expected call of ExecuteCommandContext.
func (mr *MockExecutorMockRecorder) ExecuteCommandContext(context, name any, args ...any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	varargs := append([]any{context, name}, args...)
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ExecuteCommandContext", reflect.TypeOf((*MockExecutor)(nil).ExecuteCommandContext), varargs...)
}

// GetCombinedOutput mocks base method.
func (m *MockExecutor) GetCombinedOutput(cmd *exec.Cmd) ([]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetCombinedOutput", cmd)
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetCombinedOutput indicates an expected call of GetCombinedOutput.
func (mr *MockExecutorMockRecorder) GetCombinedOutput(cmd any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetCombinedOutput", reflect.TypeOf((*MockExecutor)(nil).GetCombinedOutput), cmd)
}
