//go:build !windows

package app

import (
	"errors"
	"os/exec"
)

func configureUpdateInstaller(cmd *exec.Cmd) {}

func waitForUpdateParent(pid int) error {
	return errors.New("自动安装更新目前仅支持 Windows")
}
