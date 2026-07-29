package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRewriteInitializeUsesNestedXGoModules(t *testing.T) {
	root := t.TempDir()
	module := filepath.Join(root, "backend")
	if err := os.MkdirAll(filepath.Join(module, "cmd"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(module, "go.mod"), []byte("module example.com/backend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(module, "cmd", "main.yap"), []byte("echo \"hello\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"rootUri": filePathURI(root),
			"workspaceFolders": []map[string]string{
				{"uri": filePathURI(root), "name": filepath.Base(root)},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rewritten := rewriteInitialize(body)

	var message initializeMessage
	if err := json.Unmarshal(rewritten, &message); err != nil {
		t.Fatal(err)
	}
	if len(message.Params.WorkspaceFolders) != 1 {
		t.Fatalf("got %d workspace folders, want 1", len(message.Params.WorkspaceFolders))
	}
	if got := fileURIPath(message.Params.WorkspaceFolders[0].URI); got != module {
		t.Fatalf("got workspace folder %q, want %q", got, module)
	}
}

func TestRewriteInitializeLeavesModuleRootUnchanged(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/root\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.yap"), []byte("echo \"hello\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"rootUri": filePathURI(root),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := rewriteInitialize(body); string(got) != string(body) {
		t.Fatal("module-root initialize request was unexpectedly rewritten")
	}
}
