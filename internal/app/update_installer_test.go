package app

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func updateTestFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateRestartIsExplicitAndFailureIsReportedOnce(t *testing.T) {
	stage := t.TempDir()
	previous := pendingInstallerDirectory
	pendingInstallerDirectory = stage
	t.Cleanup(func() { pendingInstallerDirectory = previous })
	if _, err := os.Stat(filepath.Join(stage, "restart")); !os.IsNotExist(err) {
		t.Fatal("restart was requested implicitly")
	}
	if pending, err := requestUpdateRestart(); !pending || err != nil {
		t.Fatal(pending, err)
	}
	if _, err := os.Stat(filepath.Join(stage, "restart")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root, err := updateStateDirectory()
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(updateInstallationResult{Error: "write blocked", Backup: filepath.Join(stage, "previous.exe")})
	updateTestFile(t, filepath.Join(root, "last-result.json"), data)
	if message := consumeUpdateFailure(); !strings.Contains(message, "write blocked") || !strings.Contains(message, "previous.exe") {
		t.Fatal("failure context was lost", message)
	}
	if message := consumeUpdateFailure(); message != "" {
		t.Fatal("stale failure prevents retrying update", message)
	}
}

func updateTestHash(t *testing.T, path string) string {
	t.Helper()
	hash, err := updateFileHash(path)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func TestPreparedUpdatePreservesTargetAndCreatesBackupAwayFromInstallDirectory(t *testing.T) {
	root, stage := t.TempDir(), t.TempDir()
	target := filepath.Join(root, "manager.exe")
	updateTestFile(t, target, []byte("old executable bytes"))
	updateTestFile(t, filepath.Join(stage, "update.exe"), []byte("new"))
	plan := updateInstallation{Target: target, OldHash: updateTestHash(t, target), NewHash: updateTestHash(t, filepath.Join(stage, "update.exe"))}
	before, _ := os.Stat(target)
	if err := applyPreparedUpdate(plan, stage); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(target)
	if !os.SameFile(before, after) {
		t.Fatal("target was recreated instead of preserving file identity and ACL")
	}
	data, _ := os.ReadFile(target)
	backup, _ := os.ReadFile(filepath.Join(stage, "previous.exe"))
	entries, _ := os.ReadDir(root)
	if string(data) != "new" || string(backup) != "old executable bytes" || len(entries) != 1 {
		t.Fatal("unexpected replacement, backup or adjacent temporary files")
	}
}

func TestPreparedUpdateRejectsChangedInputsAndExistingBackup(t *testing.T) {
	for _, mode := range []string{"changed-source", "changed-target", "existing-backup", "missing-target"} {
		t.Run(mode, func(t *testing.T) {
			root, stage := t.TempDir(), t.TempDir()
			target, source := filepath.Join(root, "manager.exe"), filepath.Join(stage, "update.exe")
			updateTestFile(t, target, []byte("original"))
			updateTestFile(t, source, []byte("new"))
			plan := updateInstallation{Target: target, OldHash: updateTestHash(t, target), NewHash: updateTestHash(t, source)}
			switch mode {
			case "changed-source":
				updateTestFile(t, source, []byte("changed"))
			case "changed-target":
				updateTestFile(t, target, []byte("user changed"))
			case "existing-backup":
				updateTestFile(t, filepath.Join(stage, "previous.exe"), []byte("keep backup"))
			case "missing-target":
				if err := os.Remove(target); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := os.ReadFile(target)
			if err := applyPreparedUpdate(plan, stage); err == nil {
				t.Fatal("unsafe update accepted")
			}
			after, _ := os.ReadFile(target)
			if !bytes.Equal(before, after) {
				t.Fatal("original file changed on failed validation")
			}
		})
	}
}

func TestPrepareUpdateValidatesArchiveWithoutTouchingInstalledFile(t *testing.T) {
	// Minimal AMD64 COFF header accepted by debug/pe; never executed.
	validPE := make([]byte, 128)
	binary.LittleEndian.PutUint16(validPE, 0x8664)
	for _, mode := range []string{"valid", "invalid-exe", "two-exes", "no-exe"} {
		t.Run(mode, func(t *testing.T) {
			root, stage := t.TempDir(), t.TempDir()
			target := filepath.Join(root, "manager.exe")
			updateTestFile(t, target, []byte("installed"))
			var archive bytes.Buffer
			writer := zip.NewWriter(&archive)
			name := "nested/manager.exe"
			data := validPE
			if mode == "invalid-exe" {
				data = []byte("not an executable")
			}
			if mode == "no-exe" {
				name = "README.md"
			}
			entry, _ := writer.Create(name)
			entry.Write(data)
			if mode == "two-exes" {
				entry, _ = writer.Create("other.exe")
				entry.Write(validPE)
			}
			writer.Close()
			zipPath := filepath.Join(root, "release.zip")
			updateTestFile(t, zipPath, archive.Bytes())
			plan, err := prepareUpdate(zipPath, target, stage)
			if (err == nil) != (mode == "valid") {
				t.Fatalf("mode=%s err=%v", mode, err)
			}
			if err == nil && (plan.OldHash == "" || plan.NewHash == "" || plan.ParentPID != os.Getpid()) {
				t.Fatal("incomplete installation plan")
			}
			installed, _ := os.ReadFile(target)
			if string(installed) != "installed" {
				t.Fatal("preparation modified running executable")
			}
		})
	}
}
