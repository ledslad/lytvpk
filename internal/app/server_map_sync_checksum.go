package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"vpk-manager/internal/platform/protocol"
)

func attachServerMapChecksums(client *http.Client, manifest *serverSyncManifest) error {
	for i := range manifest.Maps {
		manifest.Maps[i].SHA256 = ""
	}
	if len(manifest.Maps) == 0 {
		return nil
	}
	var index struct {
		Files []struct {
			Name   string `json:"name"`
			Size   int64  `json:"size"`
			SHA256 string `json:"sha256"`
		} `json:"files"`
	}
	if err := syncJSON(client, communityMapWebsite+"/api/v1/map-checksums", &index); err != nil {
		return fmt.Errorf("无法获取成都下载站的地图校验清单：%w", err)
	}
	if index.Files == nil {
		return errors.New("成都下载站返回的地图校验清单不完整")
	}
	byName := make(map[string]int, len(index.Files))
	for i, file := range index.Files {
		key := strings.ToLower(file.Name)
		if _, exists := byName[key]; exists {
			byName[key] = -1
		} else {
			byName[key] = i
		}
	}
	for i := range manifest.Maps {
		remote := &manifest.Maps[i]
		if remote.DownloadURL == "" {
			continue
		}
		manifestIndex, ok := byName[strings.ToLower(remote.ID+".vpk")]
		if ok && manifestIndex >= 0 {
			file := index.Files[manifestIndex]
			decoded, err := hex.DecodeString(file.SHA256)
			if err == nil && len(decoded) == sha256.Size && file.Size == remote.Size {
				remote.SHA256 = hex.EncodeToString(decoded)
				remote.DownloadURL += "?sha256=" + remote.SHA256
				continue
			}
		}
		remote.DownloadURL = ""
		remote.SourceProblem = "成都下载站暂无该版本的有效校验值"
	}
	return nil
}

func (plan *serverMapSyncPlan) addVerifiedMap(remote ServerSyncMap, local []ServerSyncLocal) {
	if remote.SHA256 == "" || remote.DownloadURL == "" {
		plan.Unresolved = append(plan.Unresolved, remote)
		return
	}
	var existing *ServerSyncLocal
	for i := range local {
		if strings.EqualFold(local[i].Name, remote.ID+".vpk") {
			existing = &local[i]
			break
		}
	}
	if existing != nil {
		trustedID := existing.WorkshopID == "" || (protocol.IsValidWorkshopID(remote.WorkshopID) && (existing.WorkshopID == remote.ID || existing.WorkshopID == remote.WorkshopID))
		if !trustedID {
			remote.SourceProblem = "本机同名文件的工坊身份不一致，请手动核对"
			plan.Unresolved = append(plan.Unresolved, remote)
			return
		}
		if existing.Size == remote.Size {
			sum, err := syncFileSHA256(filepath.Join(plan.Directory, existing.Name), existing)
			if err != nil {
				remote.SourceProblem = "无法校验本机地图：" + err.Error()
				plan.Unresolved = append(plan.Unresolved, remote)
				return
			}
			if sum == remote.SHA256 {
				plan.Present++
				return
			}
			plan.localChecksums[existing.Name] = sum
		}
		if plan.AllowlistEnabled {
			remote.SourceProblem = "同步白名单保留本机同名文件，请手动核对"
			plan.Unresolved = append(plan.Unresolved, remote)
			return
		}
		plan.Replacements = append(plan.Replacements, *existing)
	}
	plan.Missing = append(plan.Missing, remote)
	plan.Bytes += remote.Size
}

func syncFileSHA256(path string, expected *ServerSyncLocal) (string, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !before.Mode().IsRegular() || (expected != nil && (before.Size() != expected.Size || before.ModTime().UnixNano() != expected.Modified)) {
		return "", errors.New("文件已改变或不是普通文件")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !sameSyncFileVersion(before, opened) {
		return "", errors.New("文件在校验前已改变")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	after, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !sameSyncFileVersion(opened, after) {
		return "", errors.New("文件在校验期间已改变")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func sameSyncFileVersion(a, b os.FileInfo) bool {
	return b.Mode().IsRegular() && os.SameFile(a, b) && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}
