package release

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestBuildAndVerifyPackageManagersFromLocalArtifacts(t *testing.T) {
	t.Parallel()

	distDir := writePackageManagerFixture(t, "v1.2.3")
	outDir := filepath.Join(distDir, "package-managers")
	baseURL := "https://github.com/arjanflac/tailboard/releases/download/v1.2.3"

	result, err := BuildPackageManagers(context.Background(), PackageManagerOptions{
		RepoRoot:         ".",
		DistDir:          distDir,
		OutputDir:        outDir,
		Version:          "v1.2.3",
		ReleaseAssetBase: baseURL,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(result.HomebrewFiles) != 3 {
		t.Fatalf("expected 3 homebrew formulas, got %d", len(result.HomebrewFiles))
	}
	if len(result.ScoopFiles) != 3 {
		t.Fatalf("expected 3 scoop manifests, got %d", len(result.ScoopFiles))
	}
	if len(result.WingetFiles) != 9 {
		t.Fatalf("expected 9 winget manifests, got %d", len(result.WingetFiles))
	}

	clipdFormula, err := os.ReadFile(filepath.Join(outDir, "homebrew", "tg-clipd.rb"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`url "https://github.com/arjanflac/tailboard/releases/download/v1.2.3/tg-clipboard_v1.2.3_artifacts.json"`,
		`tg-clipd_v1.2.3_darwin_arm64.tar.gz`,
		`tg-clipd_v1.2.3_linux_arm64.tar.gz`,
		`tg-clipd_v1.2.3_linux_amd64.tar.gz`,
		`resource("archive").stage do`,
	} {
		if !strings.Contains(string(clipdFormula), want) {
			t.Fatalf("expected %q in tg-clipd formula:\n%s", want, clipdFormula)
		}
	}

	var scoop scoopManifest
	scoopBytes, err := os.ReadFile(filepath.Join(outDir, "scoop", "tg-clip.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(scoopBytes, &scoop); err != nil {
		t.Fatal(err)
	}
	if scoop.Bin != "tg-clip.exe" {
		t.Fatalf("unexpected scoop bin: %q", scoop.Bin)
	}
	if got := scoop.Architecture["64bit"].URL; got != baseURL+"/tg-clip_v1.2.3_windows_amd64.zip" {
		t.Fatalf("unexpected scoop URL: %s", got)
	}

	var wingetInstaller wingetInstallerManifest
	wingetInstallerPath := filepath.Join(outDir, "winget", "manifests", "a", "ArjanFlac", "Tailboard", "1.2.3", "ArjanFlac.Tailboard.installer.yaml")
	if err := readWingetYAML(wingetInstallerPath, &wingetInstaller); err != nil {
		t.Fatal(err)
	}
	if wingetInstaller.InstallerType != "zip" {
		t.Fatalf("unexpected winget installer type: %q", wingetInstaller.InstallerType)
	}
	if got := wingetInstaller.Installers[0].NestedInstallerFiles[0].RelativeFilePath; got != `tg-clip_v1.2.3_windows_amd64\tg-clip.exe` {
		t.Fatalf("unexpected winget nested installer path: %q", got)
	}

	if _, err := VerifyPackageManagers(context.Background(), PackageManagerOptions{
		RepoRoot:         ".",
		DistDir:          distDir,
		OutputDir:        outDir,
		Version:          "v1.2.3",
		ReleaseAssetBase: baseURL,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestBuildPackageManagersFromPublishedManifestURLs(t *testing.T) {
	t.Parallel()

	distDir := writePackageManagerFixture(t, "v2.0.0")
	manifestPath := filepath.Join(distDir, "tg-clipboard_v2.0.0_artifacts.json")
	checksumsPath := filepath.Join(distDir, "tg-clipboard_v2.0.0_checksums.txt")
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	checksumBytes, err := os.ReadFile(checksumsPath)
	if err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/download/v2.0.0/tg-clipboard_v2.0.0_artifacts.json":
			_, _ = w.Write(manifestBytes)
		case "/releases/download/v2.0.0/tg-clipboard_v2.0.0_checksums.txt":
			_, _ = w.Write(checksumBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	outDir := filepath.Join(distDir, "published-package-managers")
	result, err := BuildPackageManagers(context.Background(), PackageManagerOptions{
		RepoRoot:     ".",
		DistDir:      distDir,
		OutputDir:    outDir,
		Version:      "v2.0.0",
		ManifestRef:  server.URL + "/releases/download/v2.0.0/tg-clipboard_v2.0.0_artifacts.json",
		ChecksumsRef: server.URL + "/releases/download/v2.0.0/tg-clipboard_v2.0.0_checksums.txt",
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.ReleaseAssetBase != server.URL+"/releases/download/v2.0.0" {
		t.Fatalf("unexpected derived asset base URL: %s", result.ReleaseAssetBase)
	}

	var versionManifest wingetVersionManifest
	versionPath := filepath.Join(outDir, "winget", "manifests", "a", "ArjanFlac", "TailboardEngine", "2.0.0", "ArjanFlac.TailboardEngine.yaml")
	if err := readWingetYAML(versionPath, &versionManifest); err != nil {
		t.Fatal(err)
	}
	if versionManifest.PackageVersion != "2.0.0" {
		t.Fatalf("unexpected winget version: %q", versionManifest.PackageVersion)
	}
}

func writePackageManagerFixture(t *testing.T, version string) string {
	t.Helper()

	distDir := filepath.Join(t.TempDir(), "dist", "release")
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatal(err)
	}

	manifest := Manifest{
		Version:         version,
		SourceDateEpoch: 1712083200,
		ChecksumsFile:   checksumsFileName(version),
		NotesFile:       notesFileName(version),
		Artifacts: []Artifact{
			fixtureArtifact("tg-clipd", version, "darwin", "amd64", "tar.gz", "1111111111111111111111111111111111111111111111111111111111111111"),
			fixtureArtifact("tg-clipd", version, "darwin", "arm64", "tar.gz", "2222222222222222222222222222222222222222222222222222222222222222"),
			fixtureArtifact("tg-clipd", version, "linux", "amd64", "tar.gz", "3333333333333333333333333333333333333333333333333333333333333333"),
			fixtureArtifact("tg-clipd", version, "linux", "arm64", "tar.gz", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
			fixtureArtifact("tg-clipd", version, "windows", "amd64", "zip", "4444444444444444444444444444444444444444444444444444444444444444"),
			fixtureArtifact("tg-clipboard", version, "darwin", "amd64", "tar.gz", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
			fixtureArtifact("tg-clipboard", version, "darwin", "arm64", "tar.gz", "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"),
			fixtureArtifact("tg-clipboard", version, "linux", "amd64", "tar.gz", "5555555555555555555555555555555555555555555555555555555555555555"),
			fixtureArtifact("tg-clipboard", version, "linux", "arm64", "tar.gz", "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"),
			fixtureArtifact("tg-clipboard", version, "windows", "amd64", "zip", "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"),
			fixtureArtifact("tg-clip", version, "darwin", "amd64", "tar.gz", "6666666666666666666666666666666666666666666666666666666666666666"),
			fixtureArtifact("tg-clip", version, "darwin", "arm64", "tar.gz", "7777777777777777777777777777777777777777777777777777777777777777"),
			fixtureArtifact("tg-clip", version, "linux", "amd64", "tar.gz", "8888888888888888888888888888888888888888888888888888888888888888"),
			fixtureArtifact("tg-clip", version, "linux", "arm64", "tar.gz", "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"),
			fixtureArtifact("tg-clip", version, "windows", "amd64", "zip", "9999999999999999999999999999999999999999999999999999999999999999"),
		},
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	manifestBytes = append(manifestBytes, '\n')
	if err := os.WriteFile(filepath.Join(distDir, manifestFileName(version)), manifestBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	var checksumBuilder strings.Builder
	for _, artifact := range manifest.Artifacts {
		checksumBuilder.WriteString(artifact.SHA256)
		checksumBuilder.WriteString("  ")
		checksumBuilder.WriteString(artifact.Name)
		checksumBuilder.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(distDir, checksumsFileName(version)), []byte(checksumBuilder.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	return distDir
}

func fixtureArtifact(binary, version, goos, goarch, format, sha string) Artifact {
	target := Target{Binary: binary, GOOS: goos, GOARCH: goarch}
	name := target.ArchiveName(version)
	return Artifact{
		Name:   name,
		Path:   name,
		SHA256: sha,
		Target: target,
		Format: format,
	}
}

func TestWriteWingetYAMLIncludesSchemaComment(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "manifest.yaml")
	if err := writeWingetYAML(path, wingetVersionManifest{
		PackageIdentifier: "ArjanFlac.TailboardEngine",
		PackageVersion:    "1.0.0",
		DefaultLocale:     "en-US",
		ManifestType:      "version",
		ManifestVersion:   wingetManifestVersion,
	}); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "https://aka.ms/winget-manifest.version."+wingetManifestVersion+".schema.json") {
		t.Fatalf("expected schema comment in:\n%s", content)
	}

	var decoded wingetVersionManifest
	if err := yaml.Unmarshal([]byte(strings.TrimSpace(strings.SplitN(string(content), "\n\n", 2)[1])), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.PackageIdentifier != "ArjanFlac.TailboardEngine" {
		t.Fatalf("unexpected decoded identifier: %q", decoded.PackageIdentifier)
	}
}
