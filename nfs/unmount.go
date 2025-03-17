package nfs

import "syscall"

//go:generate mockgen -destination=mocks/unmount.go -package=mocks github.com/dell/csm-hbnfs/nfs Unmounter
type Unmounter interface {
	Unmount(target string, flag int) error
}

type SyscallUnmount struct{}

func (s *SyscallUnmount) Unmount(target string, flag int) error {
	return syscall.Unmount(target, flag)
}
