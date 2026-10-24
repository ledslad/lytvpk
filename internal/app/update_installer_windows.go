//go:build windows

package app

import (
	"fmt"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func configureUpdateInstaller(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

func waitForUpdateParent(pid int) error {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err == windows.ERROR_INVALID_PARAMETER {
		return nil // The parent has already exited.
	}
	if err != nil {
		return fmt.Errorf("等待原程序退出失败：%w", err)
	}
	defer windows.CloseHandle(handle)
	state, err := windows.WaitForSingleObject(handle, 12*60*60*1000)
	if err != nil || state != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("原程序未在等待期限内退出，未执行更新：%v", err)
	}
	return nil
}
