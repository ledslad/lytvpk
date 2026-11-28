package app

import (
	"errors"
	"slices"
	"strings"
)

func syncFileProtected(files []string, name string) bool {
	return slices.ContainsFunc(files, func(protected string) bool {
		return strings.EqualFold(protected, name)
	})
}

func (a *App) GetServerMapSyncProtectedFiles() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return append([]string{}, a.mapSyncProtectedFiles...)
}

func (a *App) SetServerMapSyncFileProtected(filename string, protected bool) ([]string, error) {
	filename = strings.TrimSpace(filename)
	if !validServerMapFilename(filename) {
		return nil, errors.New("请输入完整的 .vpk 文件名，例如 example.vpk")
	}

	a.mapSyncMu.Lock()
	defer a.mapSyncMu.Unlock()
	syncFinalizeMu.Lock()
	defer syncFinalizeMu.Unlock()
	a.configWriteMu.Lock()
	defer a.configWriteMu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()

	config := a.snapshotConfigLocked()
	if protected {
		if !syncFileProtected(config.MapSyncProtectedFiles, filename) {
			config.MapSyncProtectedFiles = append(config.MapSyncProtectedFiles, filename)
			slices.Sort(config.MapSyncProtectedFiles)
		}
	} else {
		config.MapSyncProtectedFiles = slices.DeleteFunc(config.MapSyncProtectedFiles, func(name string) bool {
			return strings.EqualFold(name, filename)
		})
	}
	if err := a.writeConfigFileLocked(config); err != nil {
		return nil, err
	}
	a.mapSyncProtectedFiles = config.MapSyncProtectedFiles
	a.mapSyncPlan = nil
	return append([]string{}, a.mapSyncProtectedFiles...), nil
}
