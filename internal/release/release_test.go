package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractChangelogSectionPrefersVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CHANGELOG.md")
	content := strings.Join([]string{
		"# Changelog",
		"",
		"## Unreleased",
		"",
		"- pending change",
		"",
		"## v1.2.3",
		"",
		"- shipped change",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	body, section, err := extractChangelogSection(path, "v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if section != "v1.2.3" {
		t.Fatalf("expected version section, got %q", section)
	}
	if body != "- shipped change" {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestExtractChangelogSectionFallsBackToUnreleased(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CHANGELOG.md")
	content := strings.Join([]string{
		"# Changelog",
		"",
		"## Unreleased",
		"",
		"### Added",
		"",
		"- automated release notes",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	body, section, err := extractChangelogSection(path, "v9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	if section != "Unreleased" {
		t.Fatalf("expected Unreleased section, got %q", section)
	}
	if !strings.Contains(body, "automated release notes") {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestRenderReleaseNotesIncludesAssetsSummary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CHANGELOG.md")
	content := strings.Join([]string{
		"# Changelog",
		"",
		"## Unreleased",
		"",
		"### Added",
		"",
		"- deterministic archives",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	notes, err := renderReleaseNotes(path, "v1.0.0", []Artifact{
		{
			Name: "tg-clipd_v1.0.0_linux_amd64.tar.gz",
			Target: Target{
				Binary: "tg-clipd",
				GOOS:   "linux",
				GOARCH: "amd64",
			},
		},
		{
			Name: "tg-clipboard_v1.0.0_linux_amd64.tar.gz",
			Target: Target{
				Binary: "tg-clipboard",
				GOOS:   "linux",
				GOARCH: "amd64",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"# tg-clipboard v1.0.0",
		"- deterministic archives",
		"- `tg-clipboard`: `linux/amd64`",
		"tg-clipboard_v1.0.0_checksums.txt",
		"tg-clipboard_v1.0.0_artifacts.json",
	} {
		if !strings.Contains(notes, want) {
			t.Fatalf("expected %q in notes:\n%s", want, notes)
		}
	}
}

func TestCopyServiceDefinitionsByPlatform(t *testing.T) {
	repo := t.TempDir()
	for platform, file := range map[string]string{
		"launchd": "agent.plist",
		"systemd": "agent.service",
	} {
		dir := filepath.Join(repo, "packaging", platform)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, file), []byte(platform), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		goos string
		file string
	}{
		{"darwin", "agent.plist"},
		{"linux", "agent.service"},
	}
	for _, test := range tests {
		staging := filepath.Join(t.TempDir(), test.goos)
		if err := copyServiceDefinitions(repo, staging, Target{GOOS: test.goos}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(staging, "service", test.file)); err != nil {
			t.Fatalf("%s service file missing: %v", test.goos, err)
		}
	}
}

func TestArchiveEntriesIncludesNestedServiceFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "tg-clipd"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "service"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "service", "tg-clipd.service"), []byte("unit"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := archiveEntries(root)
	if err != nil {
		t.Fatal(err)
	}
	// Entries are sorted; "service/…" sorts before "tg-clipd".
	want := []string{filepath.Join("service", "tg-clipd.service"), "tg-clipd"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected entries: got %v want %v", got, want)
	}
}
