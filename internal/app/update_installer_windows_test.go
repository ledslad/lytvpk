//go:build windows

package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestUpdateWorksWhenDirectoryDisallowsNewFiles(t *testing.T) {
	root, stage := t.TempDir(), t.TempDir()
	target := filepath.Join(root, "manager.exe")
	updateTestFile(t, target, []byte("original"))
	updateTestFile(t, filepath.Join(stage, "update.exe"), []byte("updated"))
	plan := updateInstallation{Target: target, OldHash: updateTestHash(t, target), NewHash: updateTestHash(t, filepath.Join(stage, "update.exe"))}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sid := user.User.Sid.String()
	setACL := func(path, rights string) {
		t.Helper()
		sd, err := windows.SecurityDescriptorFromString(fmt.Sprintf("D:P(A;;%s;;;%s)", rights, sid))
		if err != nil {
			t.Fatal(err)
		}
		dacl, _, err := sd.DACL()
		if err != nil {
			t.Fatal(err)
		}
		if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
			t.Fatal(err)
		}
	}
	// Only this temporary directory is restricted. The owner can restore its
	// DACL, while the executable keeps an explicit writable file ACL.
	setACL(target, "FA")
	t.Cleanup(func() { setACL(root, "FA") })
	setACL(root, "GRGX")
	if f, err := os.Create(filepath.Join(root, "manager.exe.new")); err == nil {
		f.Close()
		t.Fatal("fixture did not reproduce the denied directory creation")
	}
	if err := applyPreparedUpdate(plan, stage); err != nil {
		t.Fatal("could not update existing writable executable", err)
	}
	if hash := updateTestHash(t, target); hash != plan.NewHash {
		t.Fatal("replacement mismatch")
	}
	if hash := updateTestHash(t, filepath.Join(stage, "previous.exe")); hash != plan.OldHash {
		t.Fatal("backup mismatch")
	}
}

func TestUpdateKeepsLockedExecutableUnchanged(t *testing.T) {
	root, stage := t.TempDir(), t.TempDir()
	target := filepath.Join(root, "manager.exe")
	updateTestFile(t, target, []byte("running executable"))
	updateTestFile(t, filepath.Join(stage, "update.exe"), []byte("updated"))
	plan := updateInstallation{Target: target, OldHash: updateTestHash(t, target), NewHash: updateTestHash(t, filepath.Join(stage, "update.exe"))}
	path, err := windows.UTF16PtrFromString(target)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	if err := applyPreparedUpdate(plan, stage); err == nil {
		t.Fatal("locked executable was reported as replaced")
	}
	if updateTestHash(t, target) != plan.OldHash || updateTestHash(t, filepath.Join(stage, "previous.exe")) != plan.OldHash {
		t.Fatal("locked original or backup changed")
	}
}

func TestUpdateParentFixture(t *testing.T) {
	if os.Getenv("LYTVPK_TEST_UPDATE_PARENT") != "1" {
		return
	}
	var data [1]byte
	_, _ = os.Stdin.Read(data[:])
}

func TestUpdateWaitsForOriginalProcessToExit(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestUpdateParentFixture$")
	cmd.Env = append(os.Environ(), "LYTVPK_TEST_UPDATE_PARENT=1")
	configureUpdateInstaller(cmd)
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { input.Close(); cmd.Wait() })
	done := make(chan error, 1)
	go func() { done <- waitForUpdateParent(cmd.Process.Pid) }()
	select {
	case err := <-done:
		t.Fatalf("did not wait for parent: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	input.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("did not detect parent exit")
	}
}
