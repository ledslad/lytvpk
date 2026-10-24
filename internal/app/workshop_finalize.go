package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var syncFinalizeMu sync.Mutex

// finalizeSyncDownload commits into the directory captured when the task was
// created. Linking the completed file fails atomically if the target exists.
// No cached workshop or disabled file is removed by a synchronization task.
func finalizeSyncDownload(task *DownloadTask, temporaryPath string) (string, error) {
	syncFinalizeMu.Lock()
	defer syncFinalizeMu.Unlock()
	if task.rootDirectory == "" || task.TotalSize <= 0 {
		return "", fmt.Errorf("同步下载缺少目标目录或预期文件大小")
	}
	info, err := os.Stat(temporaryPath)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() != task.TotalSize {
		return "", fmt.Errorf("同步文件大小不符：预期 %d，实际 %d", task.TotalSize, info.Size())
	}
	filename := filepath.Base(task.Filename)
	if filename != task.WorkshopID+".vpk" {
		return "", fmt.Errorf("同步文件名必须为工坊 ID.vpk")
	}
	targetPath := filepath.Join(task.rootDirectory, filename)
	quarantineDirectory := ""
	if existing, err := os.Lstat(targetPath); err == nil {
		metaInfo, metaErr := os.Lstat(GetMetaFilePath(targetPath))
		if !existing.Mode().IsRegular() || metaErr != nil || !metaInfo.Mode().IsRegular() {
			return "", fmt.Errorf("目标已存在且无法确认工坊身份，保留原文件：%s", targetPath)
		}
		meta, err := LoadWorkshopMeta(targetPath)
		if err != nil || meta == nil || meta.WorkshopID != task.WorkshopID {
			return "", fmt.Errorf("目标工坊身份不符，保留原文件：%s", targetPath)
		}
		if existing.Size() == task.TotalSize {
			_ = os.Remove(temporaryPath)
			return targetPath, nil
		}
		quarantineDirectory, _, err = quarantineSyncMaps(task.rootDirectory, []ServerSyncLocal{{
			Name: filename, Size: existing.Size(), Modified: existing.ModTime().UnixNano(), WorkshopID: task.WorkshopID,
		}})
		if err != nil {
			return "", err
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	rollback := func(cause error) (string, error) {
		if quarantineDirectory == "" {
			return "", cause
		}
		if err := restoreSyncUpdate(task.rootDirectory, quarantineDirectory); err != nil {
			return "", fmt.Errorf("%w；旧文件保留在 %s，回退：%v", cause, quarantineDirectory, err)
		}
		return "", fmt.Errorf("%w；旧版本已还原，隔离记录：%s", cause, quarantineDirectory)
	}
	base := strings.TrimSuffix(targetPath, filepath.Ext(targetPath))
	for _, ext := range []string{".meta", ".jpg", ".jpeg", ".png", ".webp", ".gif"} {
		if _, err := os.Lstat(base + ext); err == nil {
			return rollback(fmt.Errorf("目标关联文件已存在，保留原文件：%s", base+ext))
		} else if !os.IsNotExist(err) {
			return rollback(err)
		}
	}
	if err := os.Link(temporaryPath, targetPath); err != nil {
		return rollback(fmt.Errorf("目标文件已占用或无法安全写入，保留原文件：%w", err))
	}
	// The destination now owns the completed bytes. A leftover temporary link is
	// harmless and may be cleaned later; never roll back a successful commit.
	_ = os.Remove(temporaryPath)
	return targetPath, nil
}

// A rollback never overwrites files created after quarantine. Any conflicting
// old file stays in the recorded quarantine directory for manual recovery.
func restoreSyncUpdate(root, directory string) error {
	data, err := os.ReadFile(filepath.Join(directory, "restore.json"))
	if err != nil {
		return err
	}
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return err
	}
	var failures []string
	for _, name := range names {
		if filepath.Base(name) != name || name == "." || name == ".." {
			return fmt.Errorf("无效的恢复文件名")
		}
		source, target := filepath.Join(directory, name), filepath.Join(root, name)
		if err := os.Link(source, target); err != nil {
			failures = append(failures, name+": "+err.Error())
			continue
		}
		if err := os.Remove(source); err != nil {
			failures = append(failures, name+": "+err.Error())
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%s", strings.Join(failures, "; "))
	}
	return nil
}

func commitDownloadFile(ctx context.Context, task *DownloadTask, temporaryPath, targetPath string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if task.syncDownload {
		_, err := finalizeSyncDownload(task, temporaryPath)
		return err
	}
	return os.Rename(temporaryPath, targetPath)
}

// Synchronization metadata is staged separately and linked without replacing an
// existing sidecar. Ordinary downloads retain the upstream overwrite behavior.
func saveDownloadMeta(task *DownloadTask, targetPath string, details WorkshopFileDetails) error {
	if !task.syncDownload {
		return SaveWorkshopMeta(targetPath, details)
	}
	if info, err := os.Lstat(GetMetaFilePath(targetPath)); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("目标身份文件不是普通文件，保留原文件")
		}
		meta, err := LoadWorkshopMeta(targetPath)
		if err == nil && meta != nil && meta.WorkshopID == details.PublishedFileId {
			return nil
		}
		return fmt.Errorf("目标身份文件已存在且身份不符，保留原文件")
	} else if !os.IsNotExist(err) {
		return err
	}
	stage, err := os.CreateTemp(filepath.Dir(targetPath), ".sync-meta-*.vpk")
	if err != nil {
		return err
	}
	stagePath := stage.Name()
	if err := stage.Close(); err != nil {
		_ = os.Remove(stagePath)
		return err
	}
	defer os.Remove(stagePath)
	metaPath := GetMetaFilePath(stagePath)
	defer os.Remove(metaPath)
	if err := SaveWorkshopMeta(stagePath, details); err != nil {
		return err
	}
	return os.Link(metaPath, GetMetaFilePath(targetPath))
}
