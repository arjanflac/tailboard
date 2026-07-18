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
	content, err := render(systemdTemplate, map[string]string{"Description": "Agent", "Command": `"/bin/clipd"`})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `ExecStart="/bin/clipd"`) {
		t.Fatalf("unexpected unit: %s", content)
	}
}
