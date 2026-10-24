package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMapSyncPlanProtectsInstalledDependenciesAndOnlyCleansExplicitMaps(t *testing.T) {
	local := []ServerSyncLocal{
		{Name: "old.vpk", WorkshopID: "123456789", Size: 10},
		{Name: "dependency.vpk", WorkshopID: "234567890", Size: 10},
		{Name: "failed.vpk", WorkshopID: "345678901", Size: 10},
		{Name: "unlisted.vpk", Size: 10},
		{Name: "weapon.vpk", WorkshopID: "456789012", Size: 10},
	}
	manifest := serverSyncManifest{SnapshotID: "snapshot", Maps: []ServerSyncMap{
		{ID: "dependency", WorkshopID: "234567890", Size: 20},
		{ID: "new", WorkshopID: "567890123", Size: 30},
		{ID: "manual", Size: 40},
	}, Items: []serverSyncDeleted{
		{ID: "old", WorkshopID: "123456789", Status: "removed"},
		{ID: "dependency", WorkshopID: "234567890", Status: "removed"},
		{ID: "failed", WorkshopID: "345678901", Status: "removed", HasErrors: true},
		{ID: "weapon", WorkshopID: "456789012", Status: "removed"},
	}}
	p := buildSyncPlan(defaultMapSyncServer, t.TempDir(), manifest, local, func(path string) bool { return !strings.HasSuffix(path, "weapon.vpk") })
	if len(p.Cleanup) != 1 || p.Cleanup[0].Name != "old.vpk" {
		t.Fatalf("清理范围错误: %+v", p.Cleanup)
	}
	if len(p.Missing) != 2 || len(p.Unresolved) != 1 {
		t.Fatalf("缺图识别错误: %+v", p)
	}
	manifest.Maps = append(manifest.Maps, ServerSyncMap{ID: "reinstalled", WorkshopID: "123456789", Size: 999})
	p2 := buildSyncPlan(defaultMapSyncServer, p.Directory, manifest, local, func(string) bool { return true })
	for _, file := range p2.Cleanup {
		if file.Name == "old.vpk" {
			t.Fatal("已重新安装的图被清理")
		}
	}
	if p2.ID == p.ID {
		t.Fatal("变化的计划没有失效")
	}
}

func TestMapSyncRejectsMismatchedSnapshotAndIncompletePages(t *testing.T) {
	for _, mode := range []string{"mismatch", "short", "rate", "ok"} {
		t.Run(mode, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == publicMapsPath {
					json.NewEncoder(w).Encode(serverSyncManifest{SnapshotID: "same", Complete: true, GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano), Maps: []ServerSyncMap{}})
					return
				}
				if mode == "rate" {
					w.Header().Set("Retry-After", "60")
					w.WriteHeader(429)
					return
				}
				d := serverSyncManifest{SnapshotID: "same", Complete: true, Items: []serverSyncDeleted{}, Total: 0}
				if mode == "mismatch" {
					d.SnapshotID = "other"
				}
				if mode == "short" {
					d.Total = 2
				}
				json.NewEncoder(w).Encode(d)
			}))
			defer s.Close()
			_, err := fetchSyncManifest(s.Client(), s.URL)
			if (err == nil) != (mode == "ok") {
				t.Fatalf("mode=%s err=%v", mode, err)
			}
		})
	}
}

func TestMapSyncQuarantineAndRestoreDoesNotOverwrite(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"old.vpk", "old.meta", "old.png"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	info, _ := os.Stat(filepath.Join(root, "old.vpk"))
	files := []ServerSyncLocal{{Name: "old.vpk", Size: info.Size(), Modified: info.ModTime().UnixNano()}}
	dir, count, err := quarantineSyncMaps(root, files)
	if err != nil || count != 1 {
		t.Fatalf("隔离失败 %d %v", count, err)
	}
	if _, err := os.Stat(filepath.Join(root, "old.vpk")); !os.IsNotExist(err) {
		t.Fatal("原文件未移走")
	}
	a := &App{rootDir: root}
	os.WriteFile(filepath.Join(root, "old.vpk"), []byte("new-file"), 0600)
	if _, err := a.RestoreLastServerMapSync(); err == nil {
		t.Fatal("覆盖冲突未拒绝")
	}
	data, _ := os.ReadFile(filepath.Join(root, "old.vpk"))
	if string(data) != "new-file" {
		t.Fatal("已有文件被覆盖")
	}
	os.Remove(filepath.Join(root, "old.vpk"))
	if count, err := a.RestoreLastServerMapSync(); err != nil || count != 1 {
		t.Fatalf("恢复失败 %d %v", count, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "old.meta")); !os.IsNotExist(err) {
		t.Fatal("meta未恢复")
	}
}

func TestMapSyncRejectsChangedFilesAndUnsafeSource(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "map.vpk")
	os.WriteFile(p, []byte("data"), 0600)
	_, _, err := quarantineSyncMaps(root, []ServerSyncLocal{{Name: "map.vpk", Size: 999, Modified: 0}})
	if err == nil {
		t.Fatal("改变的文件仍被隔离")
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal("文件丢失")
	}
	// Build a synthetic credential-bearing URL without embedding a secret URL.
	credentialURL := url.URL{Scheme: "https", Host: "example.com", User: url.UserPassword("test-user", "test-password")}
	for _, value := range []string{"http://cd.fastbot.top", credentialURL.String(), "https://cd.fastbot.top/api/maps"} {
		if _, err := normalizeMapSyncServer(value); err == nil {
			t.Fatalf("不安全来源被接受: %s", value)
		}
	}
	if source, err := normalizeMapSyncServer(""); err != nil || source != defaultMapSyncServer {
		t.Fatal(source, err)
	}
}

func TestMapSyncSourcePersistsAndMigratesExistingConfig(t *testing.T) {
	a := newConfigTestApp(t)
	os.WriteFile(a.configPath, []byte(`{"migrationVersion":2,"defaultDirectory":"keep"}`), 0600)
	a.loadConfig()
	c := a.GetAppConfig()
	if c.MapSyncServer != defaultMapSyncServer || c.DefaultDirectory != "keep" || c.MigrationVersion != configMigrationVersion {
		t.Fatalf("迁移错误: %+v", c)
	}
	c.MapSyncServer = "https://gd.fastbot.top/"
	if err := a.SaveAppConfig(c); err != nil {
		t.Fatal(err)
	}
	a.loadConfig()
	if a.GetAppConfig().MapSyncServer != "gd.fastbot.top" {
		t.Fatal("来源未持久化")
	}
}
