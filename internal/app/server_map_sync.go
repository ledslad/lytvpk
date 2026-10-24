package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"vpk-manager/internal/parser"
	"vpk-manager/internal/platform/protocol"
)

const defaultMapSyncServer = "cd.fastbot.top"
const publicMapsPath = "/api/public/maps"
const communityStatusURL = "https://l4d2.fastbot.top/api/v1/servers"

type ServerSyncMap struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	WorkshopID  string   `json:"workshop_id"`
	WorkshopIDs []string `json:"workshop_ids"`
	Size        int64    `json:"size_bytes"`
}
type serverSyncDeleted struct {
	ID         string `json:"id"`
	WorkshopID string `json:"workshop_id"`
	Status     string `json:"status"`
	HasErrors  bool   `json:"has_errors"`
}
type serverSyncManifest struct {
	SnapshotID  string              `json:"snapshot_id"`
	GeneratedAt string              `json:"generated_at"`
	Complete    bool                `json:"complete"`
	Maps        []ServerSyncMap     `json:"maps"`
	Items       []serverSyncDeleted `json:"items"`
	Total       int                 `json:"total"`
	NextOffset  *int                `json:"next_offset"`
}
type ServerSyncLocal struct {
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	Modified   int64  `json:"modified"`
	WorkshopID string `json:"workshop_id"`
}
type ServerMapSyncPreview struct {
	ID         string            `json:"id"`
	Server     string            `json:"server"`
	Directory  string            `json:"directory"`
	Missing    []ServerSyncMap   `json:"missing"`
	Unresolved []ServerSyncMap   `json:"unresolved"`
	Cleanup    []ServerSyncLocal `json:"cleanup"`
	Present    int               `json:"present"`
	Bytes      int64             `json:"bytes"`
}
type serverMapSyncPlan struct {
	ServerMapSyncPreview
	Snapshot string
	Created  time.Time
}
type ServerMapSyncResult struct {
	Queued              int      `json:"queued"`
	AlreadyQueued       int      `json:"already_queued"`
	Quarantined         int      `json:"quarantined"`
	QuarantineDirectory string   `json:"quarantine_directory"`
	Errors              []string `json:"errors"`
}

func normalizeMapSyncServer(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultMapSyncServer, nil
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", errors.New("请输入服务器域名或 HTTPS 地址，不要包含路径、密码或参数")
	}
	return strings.ToLower(u.Host), nil
}

func syncJSON(client *http.Client, address string, output any) error {
	r, err := client.Get(address)
	if err != nil {
		return fmt.Errorf("查询服务器失败: %w", err)
	}
	defer r.Body.Close()
	if r.StatusCode == 429 {
		return fmt.Errorf("请求过于频繁，请在 %s 秒后重试", defaultString(r.Header.Get("Retry-After"), "60"))
	}
	if r.StatusCode != 200 {
		return fmt.Errorf("服务器查询失败（HTTP %d），已停止清理", r.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 16*1024*1024+1))
	if err != nil || len(data) > 16*1024*1024 {
		return errors.New("服务器清单读取失败或过大")
	}
	if err := json.Unmarshal(data, output); err != nil {
		return errors.New("服务器返回了无效的地图清单")
	}
	return nil
}

func fetchSyncManifest(client *http.Client, base string) (serverSyncManifest, error) {
	var current serverSyncManifest
	if err := syncJSON(client, base+publicMapsPath, &current); err != nil {
		return current, err
	}
	stamp, err := time.Parse(time.RFC3339Nano, current.GeneratedAt)
	if err != nil || time.Since(stamp) > 5*time.Minute || time.Until(stamp) > time.Minute || !current.Complete || current.SnapshotID == "" || current.Maps == nil {
		return current, errors.New("服务器地图快照过期或不完整，已停止清理")
	}
	current.Items = []serverSyncDeleted{}
	offset, total := 0, -1
	for page := 0; page < 11; page++ {
		var deleted serverSyncManifest
		if err := syncJSON(client, fmt.Sprintf("%s%s/deleted?limit=1000&offset=%d", base, publicMapsPath, offset), &deleted); err != nil {
			return current, err
		}
		if !deleted.Complete || deleted.SnapshotID != current.SnapshotID || deleted.Items == nil || deleted.Total < 0 || deleted.Total > 10000 || (total != -1 && total != deleted.Total) {
			return current, errors.New("删除记录与当前清单快照不一致，请稍后重新预览")
		}
		total = deleted.Total
		current.Items = append(current.Items, deleted.Items...)
		if deleted.NextOffset == nil {
			if len(current.Items) != total {
				return current, errors.New("删除记录分页不完整")
			}
			return current, nil
		}
		if *deleted.NextOffset != offset+len(deleted.Items) || *deleted.NextOffset <= offset || *deleted.NextOffset >= total {
			return current, errors.New("删除记录分页无效")
		}
		offset = *deleted.NextOffset
	}
	return current, errors.New("删除记录超过分页限制")
}

func scanSyncLocal(root string) ([]ServerSyncLocal, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	files := []ServerSyncLocal{}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.EqualFold(filepath.Ext(entry.Name()), ".vpk") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		file := ServerSyncLocal{Name: entry.Name(), Size: info.Size(), Modified: info.ModTime().UnixNano()}
		meta, err := LoadWorkshopMeta(filepath.Join(root, file.Name))
		if err != nil {
			return nil, fmt.Errorf("读取 %s 的元数据失败: %w", file.Name, err)
		}
		if meta != nil {
			file.WorkshopID = meta.WorkshopID
		}
		if file.WorkshopID == "" {
			id := strings.TrimSuffix(file.Name, filepath.Ext(file.Name))
			if protocol.IsValidWorkshopID(id) {
				file.WorkshopID = id
			}
		}
		files = append(files, file)
	}
	return files, nil
}

func syncIdentity(file ServerSyncLocal, id, workshop string) bool {
	return (id != "" && strings.EqualFold(strings.TrimSuffix(file.Name, filepath.Ext(file.Name)), id)) || (workshop != "" && file.WorkshopID == workshop)
}

func buildSyncPlan(server, root string, manifest serverSyncManifest, local []ServerSyncLocal, isMap func(string) bool) serverMapSyncPlan {
	plan := serverMapSyncPlan{ServerMapSyncPreview: ServerMapSyncPreview{Server: server, Directory: root, Missing: []ServerSyncMap{}, Unresolved: []ServerSyncMap{}, Cleanup: []ServerSyncLocal{}}, Snapshot: manifest.SnapshotID, Created: time.Now()}
	for _, remote := range manifest.Maps {
		present := false
		for _, file := range local {
			if syncIdentity(file, remote.ID, remote.WorkshopID) && file.Size == remote.Size {
				present = true
				break
			}
		}
		if present {
			plan.Present++
			continue
		}
		if protocol.IsValidWorkshopID(remote.WorkshopID) {
			plan.Missing = append(plan.Missing, remote)
			plan.Bytes += remote.Size
		} else {
			plan.Unresolved = append(plan.Unresolved, remote)
		}
	}
	for _, file := range local {
		protected := false
		for _, remote := range manifest.Maps {
			if syncIdentity(file, remote.ID, remote.WorkshopID) {
				protected = true
			}
			for _, id := range remote.WorkshopIDs {
				if id != "" && id == file.WorkshopID {
					protected = true
				}
			}
		}
		if protected {
			continue
		}
		for _, record := range manifest.Items {
			if record.Status == "removed" && !record.HasErrors && syncIdentity(file, record.ID, record.WorkshopID) && isMap(filepath.Join(root, file.Name)) {
				plan.Cleanup = append(plan.Cleanup, file)
				break
			}
		}
	}
	// Include local file fingerprints so a changed directory invalidates approval.
	data, _ := json.Marshal(struct {
		Plan     ServerMapSyncPreview
		Snapshot string
		Local    []ServerSyncLocal
	}{plan.ServerMapSyncPreview, plan.Snapshot, local})
	sum := sha256.Sum256(data)
	plan.ID = hex.EncodeToString(sum[:])
	return plan
}

func (a *App) makeSyncPlan() (serverMapSyncPlan, error) {
	a.mu.RLock()
	root, source := a.rootDir, a.mapSyncServer
	a.mu.RUnlock()
	if root == "" {
		return serverMapSyncPlan{}, errors.New("请先选择本机 addons 目录")
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return serverMapSyncPlan{}, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return serverMapSyncPlan{}, err
	}
	server, err := normalizeMapSyncServer(source)
	if err != nil {
		return serverMapSyncPlan{}, err
	}
	manifest, err := fetchSyncManifest(&http.Client{Timeout: 90 * time.Second}, "https://"+server)
	if err != nil {
		return serverMapSyncPlan{}, err
	}
	local, err := scanSyncLocal(root)
	if err != nil {
		return serverMapSyncPlan{}, err
	}
	return buildSyncPlan(server, root, manifest, local, func(path string) bool {
		file, err := parser.ParseVPKFile(path)
		return err == nil && file.PrimaryTag == "地图" && len(file.Chapters) > 0
	}), nil
}

func (a *App) PreviewServerMapSync() (ServerMapSyncPreview, error) {
	a.mapSyncMu.Lock()
	defer a.mapSyncMu.Unlock()
	plan, err := a.makeSyncPlan()
	if err != nil {
		return ServerMapSyncPreview{}, err
	}
	a.mapSyncPlan = &plan
	return plan.ServerMapSyncPreview, nil
}

func (a *App) StartServerMapSync(planID string, cleanup bool) (ServerMapSyncResult, error) {
	a.mapSyncMu.Lock()
	defer a.mapSyncMu.Unlock()
	result := ServerMapSyncResult{Errors: []string{}}
	if cleanup && a.HasActiveDownloads() {
		return result, errors.New("下载进行中，请等待完成后同步并清理，或取消清理选项仅补图")
	}
	if a.mapSyncPlan == nil || a.mapSyncPlan.ID != planID || time.Since(a.mapSyncPlan.Created) > 10*time.Minute {
		return result, errors.New("预览已过期，请重新预览")
	}
	plan, err := a.makeSyncPlan()
	if err != nil {
		return result, err
	}
	if plan.ID != planID {
		return result, errors.New("服务器清单或本机文件已改变，请重新预览后执行")
	}
	// Validate all missing files before any moves; downloading uses the existing queue.
	details := []WorkshopFileDetails{}
	seen := map[string]bool{}
	for _, remote := range plan.Missing {
		if seen[remote.WorkshopID] {
			continue
		}
		seen[remote.WorkshopID] = true
		group, err := a.GetWorkshopDetailsGrouped(remote.WorkshopID)
		if err != nil {
			return result, fmt.Errorf("解析缺图 %s 失败: %w", remote.Title, err)
		}
		found := false
		for _, item := range group.Groups {
			if item.Main.PublishedFileId == remote.WorkshopID && item.Main.Result == 1 && item.Main.FileUrl != "" && strings.EqualFold(filepath.Ext(item.Main.Filename), ".vpk") {
				if parseFileSize(item.Main.FileSize) != remote.Size {
					return result, fmt.Errorf("%s 的工坊版本大小与服务器不同，未提交下载或清理", remote.Title)
				}
				details = append(details, item.Main)
				found = true
				break
			}
		}
		if !found {
			return result, fmt.Errorf("缺图 %s 暂无可用 VPK 下载，未执行清理", remote.Title)
		}
	}
	// Workshop lookups may take minutes. Revalidate immediately before moving
	// anything, including current installations and all local fingerprints.
	latest, err := a.makeSyncPlan()
	if err != nil {
		return result, err
	}
	if latest.ID != planID {
		return result, errors.New("解析期间地图清单或本机文件发生变化，请重新预览")
	}
	if cleanup && a.HasActiveDownloads() {
		return result, errors.New("有正在下载的任务，未执行清理，请等待完成后重试")
	}
	if a.GetRootDirectory() != plan.Directory {
		// Resolve once more: users may switch the active directory during lookup.
		root, err := filepath.EvalSymlinks(a.GetRootDirectory())
		if err != nil || !strings.EqualFold(root, plan.Directory) {
			return result, errors.New("当前目录已切换，请重新预览")
		}
	}
	if cleanup && len(plan.Cleanup) > 0 {
		dir, count, err := quarantineSyncMaps(plan.Directory, plan.Cleanup)
		result.QuarantineDirectory, result.Quarantined = dir, count
		if err != nil {
			return result, err
		}
	}
	for _, item := range details {
		active := false
		for _, task := range a.GetDownloadTasks() {
			if task.WorkshopID == item.PublishedFileId && (task.workerActive || task.Status == "pending" || task.Status == "downloading" || task.Status == "selecting_ip") {
				active = true
				break
			}
		}
		if active {
			result.AlreadyQueued++
			continue
		}
		a.startDownloadTask(item, a.GetWorkshopPreferredIP(), true)
		result.Queued++
	}
	a.mapSyncPlan = nil
	return result, nil
}

// moveSyncFileNoReplace works within one volume and never overwrites a file
// created between validation and commit. Failure leaves recoverable data.
func moveSyncFileNoReplace(source, destination string) error {
	if err := os.Link(source, destination); err != nil {
		return err
	}
	if err := os.Remove(source); err != nil {
		rollbackErr := os.Remove(destination)
		return fmt.Errorf("移走源文件失败: %v；撤销链接: %v", err, rollbackErr)
	}
	return nil
}

func quarantineSyncMaps(root string, files []ServerSyncLocal) (string, int, error) {
	disabled := filepath.Join(root, "disabled")
	if info, err := os.Lstat(disabled); err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()) {
		return "", 0, errors.New("disabled 目录不是普通目录")
	}
	if err := os.MkdirAll(disabled, 0755); err != nil {
		return "", 0, err
	}
	dir, err := os.MkdirTemp(disabled, "server-sync-"+time.Now().Format("20060102-150405")+"-")
	if err != nil {
		return "", 0, err
	}
	names := []string{}
	for _, file := range files {
		if filepath.Base(file.Name) != file.Name {
			return dir, 0, errors.New("无效的本机文件名")
		}
		info, err := os.Lstat(filepath.Join(root, file.Name))
		if err != nil || !info.Mode().IsRegular() || info.Size() != file.Size || info.ModTime().UnixNano() != file.Modified {
			return dir, 0, errors.New("本机地图已改变，未移动文件")
		}
		names = append(names, file.Name)
		base := strings.TrimSuffix(file.Name, filepath.Ext(file.Name))
		for _, ext := range []string{".meta", ".jpg", ".jpeg", ".png", ".webp", ".gif"} {
			name := base + ext
			if info, err := os.Lstat(filepath.Join(root, name)); err == nil && info.Mode().IsRegular() {
				names = append(names, name)
			}
		}
	}
	data, _ := json.MarshalIndent(names, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "restore.json"), data, 0600); err != nil {
		return dir, 0, err
	}
	moved := []string{}
	for _, name := range names {
		if err := moveSyncFileNoReplace(filepath.Join(root, name), filepath.Join(dir, name)); err != nil {
			rollback := []string{}
			for i := len(moved) - 1; i >= 0; i-- {
				if e := moveSyncFileNoReplace(filepath.Join(dir, moved[i]), filepath.Join(root, moved[i])); e != nil {
					rollback = append(rollback, e.Error())
				}
			}
			return dir, 0, fmt.Errorf("隔离失败: %v；回退错误: %v；恢复记录: %s", err, rollback, dir)
		}
		moved = append(moved, name)
	}
	return dir, len(files), nil
}

func (a *App) RestoreLastServerMapSync() (int, error) {
	a.mapSyncMu.Lock()
	defer a.mapSyncMu.Unlock()
	syncFinalizeMu.Lock()
	defer syncFinalizeMu.Unlock()
	if a.HasActiveDownloads() {
		return 0, errors.New("下载进行中，请等待完成或取消并停止任务后再恢复")
	}
	root := a.GetRootDirectory()
	if root == "" {
		return 0, errors.New("请先选择本机 addons 目录")
	}
	disabled, err := os.Lstat(filepath.Join(root, "disabled"))
	if err != nil || !disabled.IsDir() || disabled.Mode()&os.ModeSymlink != 0 {
		return 0, errors.New("没有可用的普通隔离目录")
	}
	dirs, err := filepath.Glob(filepath.Join(root, "disabled", "server-sync-*"))
	if err != nil {
		return 0, err
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, dir := range dirs {
		info, err := os.Lstat(dir)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, "restore.json"))
		if err != nil {
			continue
		}
		var names []string
		if json.Unmarshal(data, &names) != nil {
			continue
		}
		pending := []string{}
		for _, name := range names {
			if filepath.Base(name) != name || name == "." || name == ".." {
				return 0, errors.New("恢复记录包含无效文件名")
			}
			info, err := os.Lstat(filepath.Join(dir, name))
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return 0, err
			}
			if !info.Mode().IsRegular() {
				return 0, errors.New("隔离文件不是普通文件")
			}
			if _, err := os.Lstat(filepath.Join(root, name)); !os.IsNotExist(err) {
				return 0, fmt.Errorf("%s 已存在，不覆盖；请先处理冲突", name)
			}
			pending = append(pending, name)
		}
		if len(pending) == 0 {
			continue
		}
		count := 0
		for _, name := range pending {
			if err := moveSyncFileNoReplace(filepath.Join(dir, name), filepath.Join(root, name)); err != nil {
				return count, err
			}
			if strings.EqualFold(filepath.Ext(name), ".vpk") {
				count++
			}
		}
		return count, nil
	}
	return 0, errors.New("当前目录没有可恢复的同步隔离文件")
}

type CommunityServerStatus struct {
	ID           string `json:"id"`
	Online       bool   `json:"online"`
	Name         string `json:"name"`
	ServerName   string `json:"server_name"`
	Map          string `json:"map"`
	MapTitle     string `json:"map_title"`
	Chapter      int    `json:"chapter"`
	ChapterCount int    `json:"chapter_count"`
	Players      int    `json:"players"`
	MaxPlayers   int    `json:"max_players"`
	Ping         int    `json:"ping_ms"`
}
type CommunityServerStatuses struct {
	Servers []CommunityServerStatus `json:"servers"`
}

func (a *App) GetCommunityServerStatuses() (CommunityServerStatuses, error) {
	var data CommunityServerStatuses
	err := syncJSON(&http.Client{Timeout: 15 * time.Second}, communityStatusURL, &data)
	return data, err
}
