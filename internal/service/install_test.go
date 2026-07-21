package service

import (
	"strings"
	"testing"
)

func TestServiceTemplatesEscapeValues(t *testing.T) {
	if got := xmlEscape(`a&<b>"`); got != "a&amp;&lt;b&gt;&quot;" {
		t.Fatalf("xml escape = %q", got)
	}
	if got := systemdQuote(`/tmp/a b%name`); got != `"/tmp/a b%%name"` {
		t.Fatalf("systemd quote = %q", got)
	}
	content, err := render(systemdTemplate, map[string]string{"Description": "Agent", "Command": `"/bin/tg-clipd"`})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `ExecStart="/bin/tg-clipd"`) {
		t.Fatalf("unexpected unit: %s", content)
	}
}

func TestLaunchdPathIncludesExecutableAndCommonPackageManagerLocations(t *testing.T) {
	path := launchdPath("/Users/test/.local/bin/tg-clipd", "/Users/test")
	for _, want := range []string{
		"/Users/test/.local/bin",
		"/opt/homebrew/bin",
		"/usr/local/bin",
		"/usr/bin",
	} {
		if !strings.Contains(path, want) {
			t.Errorf("launchd PATH %q does not include %q", path, want)
		}
	}
	if strings.Count(path, "/Users/test/.local/bin") != 1 {
		t.Fatalf("launchd PATH contains duplicate executable directory: %q", path)
	}

	content, err := render(launchdTemplate, map[string]string{
		"Label": "com.thalys.tgclipboard.tg-clipd", "Arguments": "\n        <string>/Users/test/.local/bin/tg-clipd</string>",
		"Stdout": "/tmp/tg-clipd.log", "Stderr": "/tmp/tg-clipd.error.log", "Path": path, "Home": "/Users/test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "<key>PATH</key><string>"+path+"</string>") {
		t.Fatalf("launchd plist does not set PATH: %s", content)
	}
	if !strings.Contains(string(content), "<key>HOME</key><string>/Users/test</string>") {
		t.Fatalf("launchd plist does not set HOME: %s", content)
	}
}
