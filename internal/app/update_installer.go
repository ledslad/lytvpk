package app

import (
	"archive/zip"
	"crypto/sha256"
	"debug/pe"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

const updateInstallerFlag = "--lytvpk-update-installer"

var updateInstallerMu sync.Mutex
var pendingInstallerDirectory string

type updateInstallation struct {
	ParentPID int
	Target    string
	OldHash   string
	NewHash   string
}

type updateInstallationResult struct {
	Error  string `json:"error"`
	Backup string `json:"backup"`
}

func updateStateDirectory() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, "LytVPK", "updates")
	return dir, os.MkdirAll(dir, 0700)
}

func updateFileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// This function never creates, renames or removes a file beside the target.
// Opening the existing file preserves its ACL, which may allow modification
// even when the installation directory disallows creating new files.
func overwriteUpdateFile(source, target string) error {
	info, err := os.Lstat(target)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("目标不是现有普通文件：%s", target)
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer out.Close()
	opened, err := out.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return errors.New("目标文件发生变化，未写入")
	}
	if err := out.Truncate(0); err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	return out.Close()
}

func prepareUpdate(zipPath, currentExe, stage string) (updateInstallation, error) {
	plan := updateInstallation{ParentPID: os.Getpid()}
	var err error
	plan.Target, err = filepath.Abs(currentExe)
	if err != nil {
		return plan, err
	}
	info, err := os.Lstat(plan.Target)
	if err != nil || !info.Mode().IsRegular() {
		return plan, errors.New("当前程序不是普通文件，未准备更新")
	}
	plan.OldHash, err = updateFileHash(plan.Target)
	if err != nil {
		return plan, err
	}
	archive, err := zip.OpenReader(zipPath)
	if err != nil {
		return plan, err
	}
	defer archive.Close()
	var executable *zip.File
	for _, file := range archive.File {
		if strings.EqualFold(filepath.Ext(file.Name), ".exe") {
			if executable != nil || file.FileInfo().IsDir() || file.Mode()&os.ModeSymlink != 0 {
				return plan, errors.New("更新包含多个或无效 EXE，未准备更新")
			}
			executable = file
		}
	}
	if executable == nil || executable.UncompressedSize64 == 0 || executable.UncompressedSize64 > 256*1024*1024 {
		return plan, errors.New("更新包缺少有效 EXE 或文件过大")
	}
	r, err := executable.Open()
	if err != nil {
		return plan, err
	}
	defer r.Close()
	source := filepath.Join(stage, "update.exe")
	f, err := os.OpenFile(source, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		return plan, err
	}
	n, copyErr := io.Copy(f, io.LimitReader(r, 256*1024*1024+1))
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil || uint64(n) != executable.UncompressedSize64 {
		return plan, fmt.Errorf("解压更新文件失败：%v；%v", copyErr, closeErr)
	}
	image, err := pe.Open(source)
	if err != nil {
		return plan, fmt.Errorf("更新文件不是有效的 Windows 程序：%w", err)
	}
	machine := image.Machine
	image.Close()
	if machine != pe.IMAGE_FILE_MACHINE_AMD64 {
		return plan, errors.New("更新文件不适用于 Windows amd64")
	}
	plan.NewHash, err = updateFileHash(source)
	return plan, err
}

func copyUpdateBackup(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	if copyErr == nil {
		copyErr = out.Sync()
	}
	closeErr := out.Close()
	return errors.Join(copyErr, closeErr)
}

func applyPreparedUpdate(plan updateInstallation, stage string) error {
	source, backup := filepath.Join(stage, "update.exe"), filepath.Join(stage, "previous.exe")
	if hash, err := updateFileHash(source); err != nil || hash != plan.NewHash {
		return errors.New("更新文件校验失败，未修改当前程序")
	}
	if hash, err := updateFileHash(plan.Target); err != nil || hash != plan.OldHash {
		return errors.New("当前程序已改变，未覆盖；请重新检查更新")
	}
	if err := copyUpdateBackup(plan.Target, backup); err != nil {
		return fmt.Errorf("备份失败，未替换程序：%w", err)
	}
	if hash, err := updateFileHash(backup); err != nil || hash != plan.OldHash {
		return errors.New("备份校验失败，未替换程序")
	}
	err := overwriteUpdateFile(source, plan.Target)
	if err == nil {
		if hash, checkErr := updateFileHash(plan.Target); checkErr == nil && hash == plan.NewHash {
			return nil
		}
		err = errors.New("安装后文件校验失败")
	}
	if hash, checkErr := updateFileHash(plan.Target); checkErr == nil && hash == plan.OldHash {
		return fmt.Errorf("安装失败，原程序已保留：%w", err)
	}
	if rollbackErr := overwriteUpdateFile(backup, plan.Target); rollbackErr != nil {
		return fmt.Errorf("安装失败：%v；回退失败：%v；备份：%s", err, rollbackErr, backup)
	}
	if hash, checkErr := updateFileHash(plan.Target); checkErr != nil || hash != plan.OldHash {
		return fmt.Errorf("回退校验失败，请用备份恢复：%s", backup)
	}
	return fmt.Errorf("安装失败，已回退：%w", err)
}

func installUpdate(zipPath, currentExe string) error {
	updateInstallerMu.Lock()
	defer updateInstallerMu.Unlock()
	if pendingInstallerDirectory != "" {
		return errors.New("更新已就绪，请先退出程序完成安装")
	}
	root, err := updateStateDirectory()
	if err != nil {
		return err
	}
	stage, err := os.MkdirTemp(root, "update-")
	if err != nil {
		return err
	}
	started := false
	defer func() {
		if !started {
			// stage is a freshly created private directory under the user profile.
			_ = os.RemoveAll(stage)
		}
	}()
	plan, err := prepareUpdate(zipPath, currentExe, stage)
	if err != nil {
		return err
	}
	helper := filepath.Join(stage, "installer.exe")
	if err := copyUpdateBackup(currentExe, helper); err != nil {
		return err
	}
	data, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	manifest := filepath.Join(stage, "installation.json")
	if err := os.WriteFile(manifest, data, 0600); err != nil {
		return err
	}
	cmd := exec.Command(helper, updateInstallerFlag, manifest)
	configureUpdateInstaller(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动更新助手失败：%w", err)
	}
	_ = cmd.Process.Release()
	started = true
	pendingInstallerDirectory = stage
	return nil
}

func requestUpdateRestart() (bool, error) {
	updateInstallerMu.Lock()
	defer updateInstallerMu.Unlock()
	if pendingInstallerDirectory == "" {
		return false, nil
	}
	return true, os.WriteFile(filepath.Join(pendingInstallerDirectory, "restart"), []byte("restart"), 0600)
}

// RunUpdateInstaller handles only the private helper invocation, before Wails
// or singleton initialization. It never terminates the running application.
func RunUpdateInstaller(args []string) bool {
	if len(args) < 2 || args[1] != updateInstallerFlag {
		return false
	}
	if len(args) != 3 {
		return true
	}
	self, err := os.Executable()
	if err != nil {
		return true
	}
	stage := filepath.Dir(self)
	if filepath.Clean(args[2]) != filepath.Join(stage, "installation.json") {
		return true
	}
	var plan updateInstallation
	data, err := os.ReadFile(args[2])
	if err != nil || len(data) > 64*1024 || json.Unmarshal(data, &plan) != nil || plan.ParentPID <= 0 || !filepath.IsAbs(plan.Target) || strings.EqualFold(plan.Target, self) {
		return true
	}
	err = waitForUpdateParent(plan.ParentPID)
	parentExited := err == nil
	if err == nil {
		err = applyPreparedUpdate(plan, stage)
	}
	result := updateInstallationResult{Backup: filepath.Join(stage, "previous.exe")}
	if err != nil {
		result.Error = err.Error()
	}
	if root, rootErr := updateStateDirectory(); rootErr == nil {
		data, _ := json.Marshal(result)
		_ = os.WriteFile(filepath.Join(root, "last-result.json"), data, 0600)
	}
	if _, restartErr := os.Stat(filepath.Join(stage, "restart")); restartErr == nil && parentExited {
		// Restart only when the user confirmed the update dialog. If installation
		// failed, launch the preserved/restored program so it can report the error.
		cmd := exec.Command(plan.Target)
		if cmd.Start() == nil {
			_ = cmd.Process.Release()
		}
	}
	return true
}

func consumeUpdateFailure() string {
	root, err := updateStateDirectory()
	if err != nil {
		return ""
	}
	path := filepath.Join(root, "last-result.json")
	data, err := os.ReadFile(path)
	var result updateInstallationResult
	if err != nil || json.Unmarshal(data, &result) != nil || result.Error == "" {
		return ""
	}
	_ = os.Remove(path)
	return fmt.Sprintf("上次更新安装失败：%s。备份位置：%s", result.Error, result.Backup)
}
