package app

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServerMapCatalogRejectsToolsAmbiguousSourcesAndSizeMismatch(t *testing.T) {
	for _, mode := range []string{"ok", "tool", "duplicate", "wrong-path", "external", "size", "id", "duplicate-map"} {
		t.Run(mode, func(t *testing.T) {
			entry := serverMapCatalogEntry{ID: "Broken hand.vpk", Name: "Broken hand.vpk", Category: "maps", Size: 100, DownloadURL: "/d/Broken%20hand.vpk"}
			manifest := serverSyncManifest{Maps: []ServerSyncMap{{ID: "Broken hand", Size: 100}}}
			switch mode {
			case "tool":
				entry.Category = "tools"
			case "wrong-path":
				entry.DownloadURL = "/files/Broken%20hand.vpk"
			case "external":
				entry.DownloadURL = "https://download.invalid/d/Broken%20hand.vpk"
			case "size":
				entry.Size++
			case "id":
				entry.ID = "another.vpk"
			case "duplicate-map":
				manifest.Maps = append(manifest.Maps, manifest.Maps[0])
			}
			catalog := serverMapCatalog{Files: []serverMapCatalogEntry{entry}}
			if mode == "duplicate" {
				catalog.Files = append(catalog.Files, entry)
			}
			mergeServerMapDownloads(&manifest, catalog)
			if (manifest.Maps[0].DownloadURL != "") != (mode == "ok") {
				t.Fatalf("unsafe matching: %+v", manifest.Maps)
			}
		})
	}
}

func TestServerMapSizeComparisonOnlyAppliesToManualMaps(t *testing.T) {
	manual := ServerSyncMap{ID: "manual", Size: 100, DownloadURL: communityMapWebsite + "/d/manual.vpk"}
	manifest := serverSyncManifest{SnapshotID: "snapshot", Maps: []ServerSyncMap{manual, {ID: "workshop", WorkshopID: "123456789", Size: 100}}}
	local := []ServerSyncLocal{{Name: "manual.vpk", Size: 5}, {Name: "renamed.vpk", WorkshopID: "123456789", Size: 5}, {Name: "other.vpk", Size: 5}}
	plan := buildSyncPlan(defaultMapSyncServer, t.TempDir(), manifest, local)
	if plan.Present != 1 || len(plan.Missing) != 1 || len(plan.Replacements) != 1 || plan.Replacements[0].Name != "manual.vpk" || len(plan.Cleanup) != 0 {
		t.Fatalf("incorrect update scope: %+v", plan)
	}
	local[0].Size = 100
	plan = buildSyncPlan(defaultMapSyncServer, plan.Directory, manifest, local)
	if plan.Present != 2 || len(plan.Missing) != 0 || len(plan.Replacements) != 0 {
		t.Fatal(plan)
	}
	local[0].Size = 5
	manifest.Maps[0].DownloadURL = ""
	plan = buildSyncPlan(defaultMapSyncServer, plan.Directory, manifest, local)
	if len(plan.Unresolved) != 1 || len(plan.Replacements) != 0 {
		t.Fatal("unavailable source must preserve old file", plan)
	}
	manifest.Maps[0].DownloadURL = manual.DownloadURL
	local[0].WorkshopID = "999999999"
	plan = buildSyncPlan(defaultMapSyncServer, plan.Directory, manifest, local)
	if len(plan.Unresolved) != 1 || len(plan.Replacements) != 0 || len(plan.Cleanup) != 0 {
		t.Fatal("same-name Workshop file must not be replaced", plan)
	}
}

func TestServerMapDoesNotBorrowCDDownloadsForGD(t *testing.T) {
	manifest := serverSyncManifest{Maps: []ServerSyncMap{{ID: "manual", Size: 10, DownloadURL: "https://download.invalid/manual.vpk"}}}
	attachServerMapDownloads(&http.Client{}, "gd.fastbot.top", &manifest)
	if manifest.Maps[0].DownloadURL != "" {
		t.Fatal("untrusted map URL retained")
	}
}

func TestServerMapDownloadPreservesNameAndRejectsInvalidContentWithoutRestoringOld(t *testing.T) {
	root := t.TempDir()
	name := "Broken hand.vpk"
	oldPath := filepath.Join(root, name)
	if err := os.WriteFile(oldPath, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(oldPath)
	dir, _, err := quarantineSyncMaps(root, []ServerSyncLocal{{Name: name, Size: info.Size(), Modified: info.ModTime().UnixNano()}})
	if err != nil {
		t.Fatal(err)
	}
	temporary := filepath.Join(root, "new.part")
	if err := os.WriteFile(temporary, []byte("<html>login</html>"), 0600); err != nil {
		t.Fatal(err)
	}
	stat, _ := os.Stat(temporary)
	remote := &ServerSyncMap{ID: "Broken hand", Size: stat.Size()}
	task := &DownloadTask{serverMap: remote, Filename: name, TotalSize: stat.Size(), rootDirectory: root}
	if _, err := finalizeSyncDownload(task, temporary); err == nil {
		t.Fatal("HTML was installed")
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatal("old file was automatically restored")
	}
	if data, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(data) != "old" {
		t.Fatal("backup lost", err)
	}
	writeTestVPK(t, temporary, map[string][]byte{"materials/resource.vmt": []byte("resource part without missions")})
	stat, _ = os.Stat(temporary)
	remote.Size, task.TotalSize = stat.Size(), stat.Size()
	path, err := finalizeSyncDownload(task, temporary)
	if err != nil || path != oldPath {
		t.Fatal(path, err)
	}
	if err := saveDownloadMeta(task, path, WorkshopFileDetails{PublishedFileId: serverMapTaskID(*remote)}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(GetMetaFilePath(path)); !os.IsNotExist(err) {
		t.Fatal("synthetic Workshop identity was saved")
	}
	local, err := scanSyncLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	plan := buildSyncPlan(defaultMapSyncServer, root, serverSyncManifest{Maps: []ServerSyncMap{*remote}}, local)
	if plan.Present != 1 || len(plan.Missing) != 0 {
		t.Fatal("download not recognized on next sync", plan)
	}
}

func TestServerMapRejectsUnsafeFilenameAndRequiresConfirmation(t *testing.T) {
	for _, name := range []string{"../map.vpk", "CON.vpk", "a:map.vpk", "x\\map.vpk", "map.vpk ", "map.exe", "NUL.test.vpk"} {
		if validServerMapFilename(name) {
			t.Fatalf("unsafe name accepted: %s", name)
		}
	}
	if !validServerMapFilename("Broken hand.vpk") {
		t.Fatal("valid name refused")
	}
	if _, err := (&App{}).StartServerMapSync("preview", true, false); err == nil || !strings.Contains(err.Error(), "确认") {
		t.Fatal(err)
	}
}
