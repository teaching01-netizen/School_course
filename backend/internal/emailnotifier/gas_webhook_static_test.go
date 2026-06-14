package emailnotifier

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGASWebhookSendsHTMLBody(t *testing.T) {
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

	if !strings.Contains(script, "htmlBody: body") {
		t.Fatalf("GAS webhook must contain %q", "htmlBody: body")
	}
}
