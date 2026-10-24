package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func syncFinalizeFixture(t *testing.T, root string) (*DownloadTask, string) {
	t.Helper()
	temporaryPath := filepath.Join(t.TempDir(), "download.part")
	if err := os.WriteFile(temporaryPath, []byte("complete"), 0600); err != nil {
		t.Fatal(err)
	}
	return &DownloadTask{WorkshopID: "123456", Filename: "123456.vpk", TotalSize: 8, rootDirectory: root}, temporaryPath
}

func TestFinalizeSyncRejectsShortDownload(t *testing.T) {
	root := t.TempDir()
	task, temporaryPath := syncFinalizeFixture(t, root)
	task.TotalSize = 10
	if _, err := finalizeSyncDownload(task, temporaryPath); err == nil {
		t.Fatal("partial download was accepted")
	}
	if _, err := os.Stat(filepath.Join(root, task.Filename)); !os.IsNotExist(err) {
		t.Fatalf("partial destination must not exist: %v", err)
	}
}

func TestFinalizeSyncPreservesOccupiedDestination(t *testing.T) {
	root := t.TempDir()
	task, temporaryPath := syncFinalizeFixture(t, root)
	destination := filepath.Join(root, task.Filename)
	if err := os.WriteFile(destination, []byte("keep existing"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := finalizeSyncDownload(task, temporaryPath); err == nil {
		t.Fatal("occupied destination was accepted")
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "keep existing" {
		t.Fatalf("existing file changed: %q, %v", data, err)
	}
}

func TestFinalizeSyncUsesCapturedDirectory(t *testing.T) {
	originalRoot, changedRoot := t.TempDir(), t.TempDir()
	task, temporaryPath := syncFinalizeFixture(t, originalRoot)
	app := &App{rootDir: changedRoot}
	path, err := finalizeSyncDownload(task, temporaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(originalRoot, task.Filename) {
		t.Fatalf("destination changed: %s", path)
	}
	if _, err := os.Stat(filepath.Join(app.rootDir, task.Filename)); !os.IsNotExist(err) {
		t.Fatalf("download leaked into new directory: %v", err)
	}
}

func TestFinalizeSyncPreservesExistingSidecar(t *testing.T) {
	root := t.TempDir()
	task, temporaryPath := syncFinalizeFixture(t, root)
	metaPath := filepath.Join(root, "123456.meta")
	if err := os.WriteFile(metaPath, []byte("keep metadata"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := finalizeSyncDownload(task, temporaryPath); err == nil {
		t.Fatal("occupied sidecar was accepted")
	}
	data, err := os.ReadFile(metaPath)
	if err != nil || string(data) != "keep metadata" {
		t.Fatalf("sidecar changed: %q, %v", data, err)
	}
}

func TestSaveSyncMetaNeverReplacesExistingSidecar(t *testing.T) {
	root := t.TempDir()
	task, _ := syncFinalizeFixture(t, root)
	task.syncDownload = true
	path := filepath.Join(root, task.Filename)
	details := WorkshopFileDetails{PublishedFileId: task.WorkshopID, Title: "First"}
	if err := saveDownloadMeta(task, path, details); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(GetMetaFilePath(path))
	if err != nil {
		t.Fatal(err)
	}
	details.Title = "Replacement"
	if err := saveDownloadMeta(task, path, details); err != nil {
		t.Fatalf("same-ID metadata should be idempotent: %v", err)
	}
	details.PublishedFileId = "999999"
	if err := saveDownloadMeta(task, path, details); err == nil {
		t.Fatal("different-ID metadata must be rejected")
	}
	current, err := os.ReadFile(GetMetaFilePath(path))
	if err != nil || !bytes.Equal(original, current) {
		t.Fatalf("existing metadata bytes changed: %q, %v", current, err)
	}
}

func TestFinalizeSyncUpdatesConfirmedOlderVersionWithBackup(t *testing.T) {
	root := t.TempDir()
	task, temporaryPath := syncFinalizeFixture(t, root)
	destination := filepath.Join(root, task.Filename)
	if err := os.WriteFile(destination, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SaveWorkshopMeta(destination, WorkshopFileDetails{PublishedFileId: task.WorkshopID, Title: "Original"}); err != nil {
		t.Fatal(err)
	}
	if _, err := finalizeSyncDownload(task, temporaryPath); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "complete" {
		t.Fatalf("new version missing: %q, %v", data, err)
	}
	backups, err := filepath.Glob(filepath.Join(root, "disabled", "server-sync-*", task.Filename))
	if err != nil || len(backups) != 1 {
		t.Fatalf("expected recorded backup: %v, %v", backups, err)
	}
	data, err = os.ReadFile(backups[0])
	if err != nil || string(data) != "old" {
		t.Fatalf("old version lost: %q, %v", data, err)
	}
}

func TestFinalizeSyncRejectsDifferentIdentity(t *testing.T) {
	root := t.TempDir()
	task, temporaryPath := syncFinalizeFixture(t, root)
	destination := filepath.Join(root, task.Filename)
	if err := os.WriteFile(destination, []byte("unrelated"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SaveWorkshopMeta(destination, WorkshopFileDetails{PublishedFileId: "999999"}); err != nil {
		t.Fatal(err)
	}
	if _, err := finalizeSyncDownload(task, temporaryPath); err == nil {
		t.Fatal("unrelated mod was accepted")
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "unrelated" {
		t.Fatalf("unrelated file changed: %q, %v", data, err)
	}
}

func TestRestoreSyncUpdatePreservesConcurrentConflict(t *testing.T) {
	root := t.TempDir()
	task, _ := syncFinalizeFixture(t, root)
	destination := filepath.Join(root, task.Filename)
	if err := os.WriteFile(destination, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	directory, _, err := quarantineSyncMaps(root, []ServerSyncLocal{{Name: task.Filename, Size: info.Size(), Modified: info.ModTime().UnixNano()}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("concurrent"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := restoreSyncUpdate(root, directory); err == nil {
		t.Fatal("rollback ignored a conflict")
	}
	data, err := os.ReadFile(destination)
	if err != nil || string(data) != "concurrent" {
		t.Fatalf("concurrent file replaced: %q, %v", data, err)
	}
	data, err = os.ReadFile(filepath.Join(directory, task.Filename))
	if err != nil || string(data) != "old" {
		t.Fatalf("backup lost: %q, %v", data, err)
	}
}

func TestActiveDownloadIncludesSelectingAndCancelledWorker(t *testing.T) {
	previous := taskManager
	taskManager = &TaskManager{tasks: map[string]*DownloadTask{}}
	t.Cleanup(func() { taskManager = previous })
	app := &App{}
	task := &DownloadTask{ID: "test", Status: "selecting_ip"}
	taskManager.tasks[task.ID] = task
	if !app.HasActiveDownloads() {
		t.Fatal("IP selection must block directory changes")
	}
	task.Status, task.workerActive = "cancelled", true
	app.RetryDownloadTask(task.ID)
	if task.Status != "cancelled" || !app.HasActiveDownloads() {
		t.Fatal("cancelled worker was restarted or no longer blocks directory changes")
	}
	task.workerActive = false
	if app.HasActiveDownloads() {
		t.Fatal("finished cancellation still blocks directory changes")
	}
}
