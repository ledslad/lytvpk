package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type ServerMapSyncTrashPreview struct {
	ID             string   `json:"id"`
	Directory      string   `json:"directory"`
	Maps           int      `json:"maps"`
	Files          int      `json:"files"`
	Bytes          int64    `json:"bytes"`
	Batches        int      `json:"batches"`
	Skipped        int      `json:"skipped"`
	SkippedReasons []string `json:"skipped_reasons"`
}

type ServerMapSyncTrashResult struct {
	Files  int      `json:"files"`
	Bytes  int64    `json:"bytes"`
	Errors []string `json:"errors"`
}

type syncTrashBatch struct {
	Name     string
	Manifest string
	Files    []ServerSyncLocal
}

// A prefix alone does not establish ownership. Only flat, regular files listed
// in a valid restore record are eligible; unexpected contents preserve a batch.
func readSyncTrashBatch(dir string) (syncTrashBatch, error) {
	batch := syncTrashBatch{Name: filepath.Base(dir), Files: []ServerSyncLocal{}}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return batch, errors.New("回收目录不是普通目录")
	}
	record := filepath.Join(dir, "restore.json")
	info, err = os.Lstat(record)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1024*1024 {
		return batch, errors.New("无效的地图恢复记录")
	}
	f, err := os.Open(record)
	if err != nil {
		return batch, err
	}
	data, err := io.ReadAll(io.LimitReader(f, 1024*1024+1))
	f.Close()
	if err != nil || len(data) > 1024*1024 {
		return batch, errors.New("地图恢复记录读取失败")
	}
	var names []string
	if json.Unmarshal(data, &names) != nil || len(names) == 0 {
		return batch, errors.New("地图恢复记录为空或无效")
	}
	allowed, maps := map[string]bool{}, map[string]bool{}
	for _, name := range names {
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\:") || filepath.Base(name) != name || allowed[name] {
			return batch, errors.New("地图恢复记录包含无效文件名")
		}
		allowed[name] = true
		if strings.EqualFold(filepath.Ext(name), ".vpk") {
			maps[strings.TrimSuffix(name, filepath.Ext(name))] = true
		}
	}
	if len(maps) == 0 {
		return batch, errors.New("恢复记录中没有地图")
	}
	for _, name := range names {
		ext := strings.ToLower(filepath.Ext(name))
		if !maps[strings.TrimSuffix(name, filepath.Ext(name))] || (ext != ".vpk" && ext != ".meta" && ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" && ext != ".gif") {
			return batch, errors.New("恢复记录包含无关文件")
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return batch, err
	}
	for _, entry := range entries {
		info, err := os.Lstat(filepath.Join(dir, entry.Name()))
		if err != nil || !info.Mode().IsRegular() || (entry.Name() != "restore.json" && !allowed[entry.Name()]) {
			return batch, errors.New("回收目录含未记录文件、子目录或链接，已保留")
		}
		batch.Files = append(batch.Files, ServerSyncLocal{Name: entry.Name(), Size: info.Size(), Modified: info.ModTime().UnixNano()})
	}
	sum := sha256.Sum256(data)
	batch.Manifest = hex.EncodeToString(sum[:])
	return batch, nil
}

func scanSyncTrash(root string) (ServerMapSyncTrashPreview, []syncTrashBatch, error) {
	preview := ServerMapSyncTrashPreview{}
	if root == "" {
		return preview, nil, errors.New("请先选择本机 addons 目录")
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return preview, nil, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return preview, nil, err
	}
	preview.Directory = root
	disabled := filepath.Join(root, "disabled")
	batches := []syncTrashBatch{}
	info, err := os.Lstat(disabled)
	if err != nil && !os.IsNotExist(err) {
		return preview, nil, err
	}
	if err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return preview, nil, errors.New("disabled 不是普通目录，未清理任何文件")
		}
		entries, err := os.ReadDir(disabled)
		if err != nil {
			return preview, nil, err
		}
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), "server-sync-") {
				continue
			}
			batch, err := readSyncTrashBatch(filepath.Join(disabled, entry.Name()))
			if err != nil {
				preview.Skipped++
				preview.SkippedReasons = append(preview.SkippedReasons, entry.Name()+": "+err.Error())
				continue
			}
			batches = append(batches, batch)
			preview.Batches++
			for _, file := range batch.Files {
				if file.Name == "restore.json" {
					continue
				}
				preview.Files++
				preview.Bytes += file.Size
				if strings.EqualFold(filepath.Ext(file.Name), ".vpk") {
					preview.Maps++
				}
			}
		}
	}
	data, err := json.Marshal(struct {
		Root    string
		Batches []syncTrashBatch
	}{root, batches})
	if err != nil {
		return preview, nil, err
	}
	sum := sha256.Sum256(data)
	preview.ID = hex.EncodeToString(sum[:])
	return preview, batches, nil
}

func (a *App) PreviewServerMapSyncTrash() (ServerMapSyncTrashPreview, error) {
	a.mapSyncMu.Lock()
	defer a.mapSyncMu.Unlock()
	syncFinalizeMu.Lock()
	defer syncFinalizeMu.Unlock()
	preview, _, err := scanSyncTrash(a.GetRootDirectory())
	return preview, err
}

// Delete individual recorded files, never recursively remove disabled or a
// batch. Keep the restore record if any file fails so remaining maps recover.
func (a *App) ClearServerMapSyncTrash(previewID string) (ServerMapSyncTrashResult, error) {
	a.mapSyncMu.Lock()
	defer a.mapSyncMu.Unlock()
	syncFinalizeMu.Lock()
	defer syncFinalizeMu.Unlock()
	result := ServerMapSyncTrashResult{Errors: []string{}}
	if a.HasActiveDownloads() {
		return result, errors.New("下载进行中，请等待完成后再清空地图同步回收站")
	}
	preview, batches, err := scanSyncTrash(a.GetRootDirectory())
	if err != nil {
		return result, err
	}
	if previewID == "" || preview.ID != previewID {
		return result, errors.New("本机目录或回收站内容已改变，请重新点击清空回收站并确认")
	}
	a.mapSyncPlan = nil
	for _, batch := range batches {
		dir := filepath.Join(preview.Directory, "disabled", batch.Name)
		failed := false
		for _, file := range batch.Files {
			if file.Name == "restore.json" {
				continue
			}
			if err := os.Remove(filepath.Join(dir, file.Name)); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s/%s: %v", batch.Name, file.Name, err))
				failed = true
				break
			}
			result.Files++
			result.Bytes += file.Size
		}
		if failed {
			continue
		}
		if err := os.Remove(filepath.Join(dir, "restore.json")); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", batch.Name, err))
			continue
		}
		if err := os.Remove(dir); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", batch.Name, err))
		}
	}
	return result, nil
}
