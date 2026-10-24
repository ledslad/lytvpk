package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkshopDownloadFilename(t *testing.T) {
	for _, test := range []struct {
		name, id, source, expected string
	}{
		{"named VPK", "123456", "Campaign With A Long Name.vpk", "123456.vpk"},
		{"uppercase VPK", "123456", `folder\Campaign.VPK`, "123456.vpk"},
		{"missing extension", "123456", "campaign", "123456.vpk"},
		{"missing filename", "123456", "", "123456.vpk"},
		{"unknown name", "123456", "unknown.vpk", "123456.vpk"},
		{"collection child", "654321", "collection-main.vpk", "654321.vpk"},
		{"zip archive", "123456", "campaign.ZIP", "123456.zip"},
		{"rar archive", "123456", "campaign.rar", "123456.rar"},
		{"7z archive", "123456", "campaign.7z", "123456.7z"},
		{"other real format", "123456", "download.bin", "123456.bin"},
		{"direct download exception", "direct-123456", `folder\campaign.zip`, "campaign.zip"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if actual := workshopDownloadFilename(test.id, test.source); actual != test.expected {
				t.Fatalf("filename = %q; expected %q", actual, test.expected)
			}
		})
	}
}

func TestReplaceExistingModRetainsWorkshopIDFilename(t *testing.T) {
	for _, location := range []string{"root", "workshop", "disabled"} {
		t.Run(location, func(t *testing.T) {
			app, root, _ := newWorkshopMoveTestApp(t, false)
			directory := root
			if location != "root" {
				directory = filepath.Join(root, location)
			}
			oldPath := filepath.Join(directory, "old-readable-name.vpk")
			addWorkshopMoveTestFile(t, app, oldPath, location)
			cached, _ := app.vpkCache.Load(oldPath)
			cached.(*VPKFileCache).File.WorkshopID = "123456"
			newPath := filepath.Join(root, workshopDownloadFilename("123456", "new-readable-name.VPK"))
			if err := os.WriteFile(newPath, []byte("new version"), 0600); err != nil {
				t.Fatal(err)
			}
			actual := app.replaceExistingMod(newPath, "123456", root)
			expected := filepath.Join(directory, "123456.vpk")
			if actual != expected {
				t.Fatalf("replacement path = %s; expected %s", actual, expected)
			}
			data, err := os.ReadFile(expected)
			if err != nil || string(data) != "new version" {
				t.Fatalf("replacement content missing: %q, %v", data, err)
			}
			if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
				t.Fatalf("old filename retained: %v", err)
			}
		})
	}
}
