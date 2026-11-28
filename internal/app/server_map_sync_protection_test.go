package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestLocalSyncProtectionCoversCleanupDownloadsAndAllReplacements(t *testing.T) {
	root := t.TempDir()
	local := []ServerSyncLocal{{Name: "private.vpk", Size: 1}, {Name: "old.vpk", Size: 1}}
	manifest := serverSyncManifest{
		Maps: []ServerSyncMap{
			{ID: "private", Size: 5, DownloadURL: "https://example.test/private.vpk", SHA256: "checksum"},
			{ID: "workshop-alias", WorkshopID: "1234567890", Size: 7},
			{ID: "fresh", Size: 3, DownloadURL: "https://example.test/fresh.vpk", SHA256: "checksum"},
		},
		Items: []serverSyncDeleted{{ID: "private", Status: "removed"}, {ID: "old", Status: "removed"}},
	}
	protected := []string{"PRIVATE.VPK", "OLD.VPK", "1234567890.VPK"}
	for _, verify := range []bool{false, true} {
		plan := buildSyncPlanWithProtection(defaultMapSyncServer, root, manifest, local, verify, verify, protected)
		if len(plan.Cleanup) != 0 || len(plan.Replacements) != 0 || len(plan.Missing) != 1 || plan.Missing[0].ID != "fresh" || len(plan.Unresolved) != 2 {
			t.Fatalf("verify=%v unsafe plan: %+v", verify, plan.ServerMapSyncPreview)
		}
	}
	manifest.AllowlistEnabled = true
	for _, verify := range []bool{false, true} {
		plan := buildSyncPlanWithVerification(defaultMapSyncServer, root, manifest, local, verify, verify)
		if len(plan.Cleanup) != 0 || len(plan.Replacements) != 0 || len(plan.Missing) != 1 || plan.Missing[0].ID != "fresh" || !plan.AllowlistEnabled {
			t.Fatalf("verify=%v unsafe server allowlist plan: %+v", verify, plan.ServerMapSyncPreview)
		}
	}
}

func TestLocalSyncProtectionPersistsAndStopsQueuedDownload(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config.json")
	a := &App{configPath: configPath, mapSyncPlan: &serverMapSyncPlan{}}
	files, err := a.SetServerMapSyncFileProtected("Personal.vpk", true)
	if err != nil || !reflect.DeepEqual(files, []string{"Personal.vpk"}) || a.mapSyncPlan != nil {
		t.Fatalf("set protection: %v %v", files, err)
	}
	if files, err = a.SetServerMapSyncFileProtected("personal.VPK", true); err != nil || len(files) != 1 {
		t.Fatalf("case-insensitive duplicate: %v %v", files, err)
	}
	if _, err := a.SetServerMapSyncFileProtected("../unexpected.vpk", true); err == nil {
		t.Fatal("accepted a path")
	}
	restarted := &App{configPath: configPath}
	restarted.loadConfig()
	if got := restarted.GetServerMapSyncProtectedFiles(); !reflect.DeepEqual(got, []string{"Personal.vpk"}) {
		t.Fatalf("reloaded protection = %v", got)
	}
	temp, dest := filepath.Join(root, "queued.vpk"), filepath.Join(root, "Personal.vpk")
	for path, data := range map[string]string{temp: "queued download", dest: "player's mod"} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	task := &DownloadTask{Filename: "Personal.vpk", rootDirectory: root, TotalSize: int64(len("queued download")), syncDownload: true}
	if err := restarted.commitDownloadFile(context.Background(), task, temp, dest); err == nil {
		t.Fatal("protected download installed")
	}
	if data, err := os.ReadFile(dest); err != nil || string(data) != "player's mod" {
		t.Fatalf("player's original file changed: %q %v", data, err)
	}
	if files, err := restarted.SetServerMapSyncFileProtected("personal.VPK", false); err != nil || len(files) != 0 {
		t.Fatalf("remove protection: %v %v", files, err)
	}
}

func TestServerAllowlistManifestSkipsDeletionRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != publicMapsPath || r.URL.RawQuery != mapSyncAllowlistQuery {
			t.Errorf("unexpected URL: %s", r.URL)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(serverSyncManifest{
			SnapshotID: "reviewed", GeneratedAt: time.Now().UTC().Format(time.RFC3339), Complete: true,
			Maps: []ServerSyncMap{}, AllowlistEnabled: true,
		})
	}))
	defer server.Close()
	manifest, err := fetchSyncManifest(server.Client(), server.URL)
	if err != nil || !manifest.AllowlistEnabled || len(manifest.Items) != 0 {
		t.Fatalf("protected manifest = %+v, err = %v", manifest, err)
	}
}
