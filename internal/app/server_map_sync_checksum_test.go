package app

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestServerMapContentVerificationReplacesSameSizeAndRejectsWrongDownload(t *testing.T) {
	root := t.TempDir()
	filename := "123456789.vpk"
	old := filepath.Join(root, filename)
	writeTestVPK(t, old, map[string][]byte{"materials/test.vmt": []byte("old")})
	correct := filepath.Join(root, "correct.part")
	writeTestVPK(t, correct, map[string][]byte{"materials/test.vmt": []byte("new")})
	bytes, err := os.ReadFile(correct)
	if err != nil {
		t.Fatal(err)
	}
	remote := ServerSyncMap{ID: "123456789", WorkshopID: "123456789", Size: int64(len(bytes)), SHA256: fmt.Sprintf("%x", sha256.Sum256(bytes)), DownloadURL: communityMapWebsite + "/d/" + filename}
	manifest := serverSyncManifest{Maps: []ServerSyncMap{remote}, SnapshotID: "stable"}
	local, err := scanSyncLocal(root)
	if err != nil || len(local) != 1 || local[0].Size != remote.Size {
		t.Fatalf("expected same-sized fixtures: %v %v", local, err)
	}
	plan := buildSyncPlanWithVerification(defaultMapSyncServer, root, manifest, local, true, true)
	if len(plan.Missing) != 1 || len(plan.Replacements) != 1 || plan.Present != 0 || plan.RequiresConfirmation {
		t.Fatalf("expected server replacement: %+v", plan)
	}
	manifest.Maps[0].SHA256 = ""
	unavailable := buildSyncPlanWithVerification(defaultMapSyncServer, root, manifest, local, true, true)
	if len(unavailable.Unresolved) != 1 || len(unavailable.Replacements) != 0 || len(unavailable.Missing) != 0 {
		t.Fatalf("missing digest should preserve old map: %+v", unavailable)
	}
	backup, _, err := quarantineSyncMaps(root, plan.Replacements)
	if err != nil {
		t.Fatal(err)
	}
	task := &DownloadTask{serverMap: &remote, Filename: filename, TotalSize: remote.Size, rootDirectory: root}
	if _, err := finalizeSyncDownload(task, filepath.Join(backup, filename)); err == nil {
		t.Fatal("same-size VPK with wrong digest was accepted")
	}
	if _, err := finalizeSyncDownload(task, correct); err != nil {
		t.Fatal(err)
	}
	local, err = scanSyncLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Maps[0] = remote
	matched := buildSyncPlanWithVerification(defaultMapSyncServer, root, manifest, local, true, true)
	if matched.Present != 1 || len(matched.Missing) != 0 {
		t.Fatalf("new file not matched: %+v", matched)
	}
}
