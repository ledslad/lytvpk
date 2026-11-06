package app

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"l4d2-manager-next/pkg/valve/vpk"
	"vpk-manager/internal/platform/protocol"
)

const communityMapWebsite = "https://l4d2.fastbot.top"
const communityMapCatalogURL = communityMapWebsite + "/api/v1/catalog"
const serverMapTaskPrefix = "server-map-"

type serverMapCatalog struct {
	Files []serverMapCatalogEntry `json:"files"`
}

type serverMapCatalogEntry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Size        int64  `json:"size"`
	DownloadURL string `json:"download_url"`
}

// A Windows basename must be preserved exactly, without path or device aliases.
func validServerMapFilename(name string) bool {
	if len(name) > 240 || len(name) <= 4 || !strings.EqualFold(filepath.Ext(name), ".vpk") ||
		strings.ContainsAny(name, "<>:\"/\\|?*\x00") || strings.TrimSpace(name) != name || strings.HasSuffix(name, ".") {
		return false
	}
	for _, r := range name {
		if r < 32 {
			return false
		}
	}
	base := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" ||
		(len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '0' && base[3] <= '9') {
		return false
	}
	return true
}

func attachServerMapDownloads(client *http.Client, server string, manifest *serverSyncManifest) {
	for i := range manifest.Maps {
		manifest.Maps[i].DownloadURL = ""
		manifest.Maps[i].SourceProblem = ""
	}
	// This site's files belong to CD. Matching names on another server do not
	// establish that the bytes come from that other server.
	if server != defaultMapSyncServer {
		return
	}
	needed := false
	for _, remote := range manifest.Maps {
		needed = needed || !protocol.IsValidWorkshopID(remote.WorkshopID)
	}
	if !needed {
		return
	}
	var catalog serverMapCatalog
	if err := syncJSON(client, communityMapCatalogURL, &catalog); err != nil || catalog.Files == nil {
		for i := range manifest.Maps {
			if !protocol.IsValidWorkshopID(manifest.Maps[i].WorkshopID) {
				manifest.Maps[i].SourceProblem = "网站下载清单暂不可用，请到 " + communityMapWebsite + "/ 手动下载"
			}
		}
		return
	}
	mergeServerMapDownloads(manifest, catalog)
}

func mergeServerMapDownloads(manifest *serverSyncManifest, catalog serverMapCatalog) {
	byName := map[string][]serverMapCatalogEntry{}
	for _, entry := range catalog.Files {
		key := strings.ToLower(entry.Name)
		byName[key] = append(byName[key], entry)
	}
	counts := map[string]int{}
	for _, remote := range manifest.Maps {
		counts[strings.ToLower(remote.ID)]++
	}
	for i := range manifest.Maps {
		remote := &manifest.Maps[i]
		if protocol.IsValidWorkshopID(remote.WorkshopID) {
			continue
		}
		remote.DownloadURL = ""
		remote.SourceProblem = "网站未提供唯一匹配的地图文件"
		filename := remote.ID + ".vpk"
		entries := byName[strings.ToLower(filename)]
		if !validServerMapFilename(filename) || counts[strings.ToLower(remote.ID)] != 1 || len(entries) != 1 {
			continue
		}
		entry := entries[0]
		if entry.Category != "maps" || entry.ID != entry.Name || !validServerMapFilename(entry.Name) {
			continue
		}
		if remote.Size <= 0 || entry.Size != remote.Size {
			remote.SourceProblem = "网站文件大小与当前服务器地图不一致，暂不自动下载"
			continue
		}
		u, err := url.Parse(entry.DownloadURL)
		if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" && u.EscapedPath() != "/d/"+url.PathEscape(entry.Name) {
			continue
		}
		if u.IsAbs() && (u.Scheme != "https" || u.Host != "l4d2.fastbot.top") || !u.IsAbs() && u.Host != "" || u.Path != "/d/"+entry.Name {
			continue
		}
		remote.DownloadURL = communityMapWebsite + "/d/" + url.PathEscape(entry.Name)
		remote.SourceProblem = ""
	}
}

func serverMapTaskID(remote ServerSyncMap) string {
	return fmt.Sprintf("%s%x", serverMapTaskPrefix, sha256.Sum256([]byte(remote.DownloadURL+"\n"+remote.ID)))
}

func (a *App) hasQueuedServerMap(remote ServerSyncMap) bool {
	taskManager.mu.RLock()
	defer taskManager.mu.RUnlock()
	for _, task := range taskManager.tasks {
		if task.WorkshopID == serverMapTaskID(remote) && (task.workerActive || task.Status == "pending" || task.Status == "downloading" || task.Status == "selecting_ip") {
			return true
		}
	}
	return false
}

func (a *App) startServerMapDownload(remote ServerSyncMap, rootDirectory string) string {
	details := WorkshopFileDetails{
		PublishedFileId: serverMapTaskID(remote), Filename: remote.ID + ".vpk",
		Title: remote.Title, FileSize: strconv.FormatInt(remote.Size, 10), FileUrl: remote.DownloadURL,
	}
	return a.createDownloadTask(details, false, true, &remote, rootDirectory)
}

func finalizeServerMapDownload(task *DownloadTask, temporaryPath string) (string, error) {
	remote := task.serverMap
	filename := remote.ID + ".vpk"
	if !validServerMapFilename(filename) || task.Filename != filename || remote.Size <= 0 || task.TotalSize != remote.Size {
		return "", fmt.Errorf("网站地图下载身份或文件名无效")
	}
	// Parse the actual VPK directory; a login/error page with a .vpk suffix is
	// never installed. Resource-only map volumes do not need mission metadata.
	opener := vpk.Single(temporaryPath)
	_, err := opener.ReadArchive()
	opener.Close()
	if err != nil {
		return "", fmt.Errorf("下载内容不是可读取的 VPK：%w", err)
	}
	target := filepath.Join(task.rootDirectory, filename)
	if _, err := os.Lstat(GetMetaFilePath(target)); !os.IsNotExist(err) {
		return "", fmt.Errorf("目标存在身份文件，保留现有文件，请重新预览")
	}
	if err := os.Link(temporaryPath, target); err != nil {
		return "", fmt.Errorf("目标已存在或无法安全安装，保留现有文件：%w", err)
	}
	_ = os.Remove(temporaryPath)
	return target, nil
}
