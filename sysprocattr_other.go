//go:build !linux

package agentrunner

import "syscall"

func taskSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid: true,
	}
}
