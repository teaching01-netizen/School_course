package emailnotifier

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGASWebhookRequiresSharedSecretAndSendsHTMLBody(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "gas-webhook", "Code.gs"))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read GAS script: %v", err)
	}
	script := string(data)

	for _, want := range []string{
		"PropertiesService.getScriptProperties().getProperty(\"WEBHOOK_SECRET\")",
		"payload.secret",
		"htmlBody: body",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("GAS webhook must contain %q", want)
		}
	}
}
