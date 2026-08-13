package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveXgolsUsesXGOLSPath(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, exeName("xgols"))
	if err := os.WriteFile(bin, []byte("dummy"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("XGOLS_PATH", bin)
	t.Setenv("XGOLS_GOBIN", "")
	t.Setenv("XGOLS_PREFER_SYSTEM", "0")

	got, err := resolveXgols()
	if err != nil {
		t.Fatal(err)
	}
	if got != bin {
		t.Fatalf("got %q, want %q", got, bin)
	}
}

func TestResolveXgolsUsesManagedGobinWhenStampMatches(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, exeName("xgols"))
	if err := os.WriteFile(bin, []byte("dummy"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("XGOLS_PATH", "")
	t.Setenv("XGOLS_GOBIN", dir)
	t.Setenv("XGOLS_MODULE", "github.com/goplus/xgols@v0.14.3")
	t.Setenv("XGOLS_PREFER_SYSTEM", "0")
	if err := writeInstalledStamp(dir, "github.com/goplus/xgols@v0.14.3"); err != nil {
		t.Fatal(err)
	}

	got, err := resolveXgols()
	if err != nil {
		t.Fatal(err)
	}
	if got != bin {
		t.Fatalf("got %q, want %q", got, bin)
	}
}

func TestReuseManagedRejectsStampMismatch(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, exeName("xgols"))
	if err := os.WriteFile(bin, []byte("dummy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeInstalledStamp(dir, "github.com/goplus/xgols@v0.14.0"); err != nil {
		t.Fatal(err)
	}

	if got := reuseManaged(dir, "github.com/goplus/xgols@v0.14.3"); got != "" {
		t.Fatalf("expected empty reuse, got %q", got)
	}
}

func TestReuseManagedRejectsMissingStamp(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, exeName("xgols"))
	if err := os.WriteFile(bin, []byte("dummy"), 0o755); err != nil {
		t.Fatal(err)
	}

	if got := reuseManaged(dir, "github.com/goplus/xgols@v0.14.3"); got != "" {
		t.Fatalf("expected empty reuse without stamp, got %q", got)
	}
}

func TestRequestedModule(t *testing.T) {
	t.Setenv("XGOLS_MODULE", "")
	if got := requestedModule(); got != defaultXgolsModule {
		t.Fatalf("got %q, want %q", got, defaultXgolsModule)
	}

	t.Setenv("XGOLS_MODULE", "github.com/goplus/xgols")
	if got := requestedModule(); got != "github.com/goplus/xgols@latest" {
		t.Fatalf("got %q", got)
	}

	t.Setenv("XGOLS_MODULE", "github.com/goplus/xgols@v0.14.3")
	if got := requestedModule(); got != "github.com/goplus/xgols@v0.14.3" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveXgolsMissingPathErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-"+exeName("xgols"))
	t.Setenv("XGOLS_PATH", missing)
	t.Setenv("XGOLS_PREFER_SYSTEM", "0")

	if _, err := resolveXgols(); err == nil {
		t.Fatal("expected error for missing XGOLS_PATH")
	}
}

func TestPreferSystemBinaryDefaultsOff(t *testing.T) {
	t.Setenv("XGOLS_PREFER_SYSTEM", "")
	if preferSystemBinary() {
		t.Fatal("expected local compile by default")
	}
	t.Setenv("XGOLS_PREFER_SYSTEM", "1")
	if !preferSystemBinary() {
		t.Fatal("expected prefer system when XGOLS_PREFER_SYSTEM=1")
	}
}

func TestExeName(t *testing.T) {
	got := exeName("xgols")
	if runtime.GOOS == "windows" {
		if got != "xgols.exe" {
			t.Fatalf("got %q, want xgols.exe", got)
		}
		return
	}
	if got != "xgols" {
		t.Fatalf("got %q, want xgols", got)
	}
}
