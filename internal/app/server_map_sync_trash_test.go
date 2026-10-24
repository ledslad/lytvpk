package app

import (
	"os"
	"path/filepath"
	"testing"
)

func makeSyncTrashFixture(t *testing.T, root string) string {
	t.Helper()
	for _, name := range []string{"old.vpk", "old.meta", "old.png"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	info, err := os.Stat(filepath.Join(root, "old.vpk"))
	if err != nil {
		t.Fatal(err)
	}
	dir, _, err := quarantineSyncMaps(root, []ServerSyncLocal{{Name: "old.vpk", Size: info.Size(), Modified: info.ModTime().UnixNano()}})
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestSyncTrashDeletesOnlyRecordedSyncFiles(t *testing.T) {
	root := t.TempDir()
	dir := makeSyncTrashFixture(t, root)
	protected := []string{filepath.Join(root, "current.vpk"), filepath.Join(root, "disabled", "other.vpk")}
	for _, path := range protected {
		if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	a := &App{rootDir: root}
	preview, err := a.PreviewServerMapSyncTrash()
	if err != nil || preview.Maps != 1 || preview.Files != 3 || preview.Bytes != 22 || preview.Batches != 1 {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
	result, err := a.ClearServerMapSyncTrash(preview.ID)
	if err != nil || result.Files != 3 || result.Bytes != 22 || len(result.Errors) != 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("batch not removed: %v", err)
	}
	for _, path := range protected {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "keep" {
			t.Fatalf("unrelated file changed: %v", err)
		}
	}
}

func TestSyncTrashRejectsChangesAndDirectorySwitch(t *testing.T) {
	root := t.TempDir()
	dir := makeSyncTrashFixture(t, root)
	a := &App{rootDir: root}
	preview, err := a.PreviewServerMapSyncTrash()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "old.vpk"), []byte("changed map"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ClearServerMapSyncTrash(preview.ID); err == nil {
		t.Fatal("changed backup was deleted")
	}
	preview, err = a.PreviewServerMapSyncTrash()
	if err != nil {
		t.Fatal(err)
	}
	a.rootDir = t.TempDir()
	if _, err := a.ClearServerMapSyncTrash(preview.ID); err == nil {
		t.Fatal("directory switch did not invalidate confirmation")
	}
	if _, err := os.Stat(filepath.Join(dir, "old.meta")); err != nil {
		t.Fatal("original backup was removed", err)
	}
}

func TestSyncTrashPreservesUnrecognizedBatches(t *testing.T) {
	for _, mode := range []string{"extra-file", "subdirectory", "bad-record", "traversal", "unrelated", "link"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			dir := makeSyncTrashFixture(t, root)
			var err error
			switch mode {
			case "extra-file":
				err = os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("keep"), 0600)
			case "subdirectory":
				err = os.Mkdir(filepath.Join(dir, "nested"), 0700)
			case "bad-record":
				err = os.WriteFile(filepath.Join(dir, "restore.json"), []byte("invalid"), 0600)
			case "traversal":
				err = os.WriteFile(filepath.Join(dir, "restore.json"), []byte(`["../other.vpk"]`), 0600)
			case "unrelated":
				err = os.WriteFile(filepath.Join(dir, "restore.json"), []byte(`["old.vpk","other.txt"]`), 0600)
			case "link":
				if err := os.Remove(filepath.Join(dir, "old.png")); err != nil {
					t.Fatal(err)
				}
				err = os.Symlink(filepath.Join(root, "outside.png"), filepath.Join(dir, "old.png"))
				if err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			a := &App{rootDir: root}
			preview, err := a.PreviewServerMapSyncTrash()
			if err != nil || preview.Skipped != 1 || preview.Files != 0 {
				t.Fatalf("preview=%+v err=%v", preview, err)
			}
			if result, err := a.ClearServerMapSyncTrash(preview.ID); err != nil || result.Files != 0 {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if _, err := os.Stat(filepath.Join(dir, "old.vpk")); err != nil {
				t.Fatal("map was removed from unrecognized batch", err)
			}
		})
	}
}

func TestSyncTrashRemovesEmptyRestoredBatchAndInvalidatesSyncPlan(t *testing.T) {
	root := t.TempDir()
	dir := makeSyncTrashFixture(t, root)
	a := &App{rootDir: root, mapSyncPlan: &serverMapSyncPlan{}}
	if count, err := a.RestoreLastServerMapSync(); err != nil || count != 1 {
		t.Fatal(count, err)
	}
	preview, err := a.PreviewServerMapSyncTrash()
	if err != nil || preview.Files != 0 || preview.Batches != 1 {
		t.Fatal(preview, err)
	}
	if _, err := a.ClearServerMapSyncTrash(preview.ID); err != nil {
		t.Fatal(err)
	}
	if a.mapSyncPlan != nil {
		t.Fatal("old sync preview still valid")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("empty batch not removed", err)
	}
	if _, err := os.Stat(filepath.Join(root, "old.vpk")); err != nil {
		t.Fatal("restored map removed", err)
	}
}

func TestSyncTrashDoesNotDeleteWhileDownloadWorkerIsActive(t *testing.T) {
	root := t.TempDir()
	dir := makeSyncTrashFixture(t, root)
	a := &App{rootDir: root}
	preview, err := a.PreviewServerMapSyncTrash()
	if err != nil {
		t.Fatal(err)
	}
	taskManager.mu.Lock()
	taskManager.tasks["trash-test"] = &DownloadTask{Status: "cancelled", workerActive: true}
	taskManager.mu.Unlock()
	t.Cleanup(func() {
		taskManager.mu.Lock()
		delete(taskManager.tasks, "trash-test")
		taskManager.mu.Unlock()
	})
	if _, err := a.ClearServerMapSyncTrash(preview.ID); err == nil {
		t.Fatal("active download worker did not block deletion")
	}
	if _, err := os.Stat(filepath.Join(dir, "old.vpk")); err != nil {
		t.Fatal("backup deleted during download", err)
	}
}
