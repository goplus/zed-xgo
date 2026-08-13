package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	defaultXgolsModule = "github.com/goplus/xgols@v0.14.1"
	installedStampName = "xgols.installed"
	xgoInstallHint     = "Install XGo from https://xgo.dev/ so `xgo` is on PATH, then restart Zed."
	goInstallHint      = "Install Go from https://go.dev/dl/ so `go` is on PATH, then restart Zed."
)

// resolveXgols returns a path to an xgols binary, installing it when needed.
//
// This compiles xgols on the user's machine (same model as vscode-xgo).
// It does not download GitHub release binaries.
//
// Resolution order:
//  1. XGOLS_PATH, if it points to an existing file
//  2. Managed XGOLS_GOBIN/xgols, if it exists and matches XGOLS_MODULE
//  3. PATH lookup for "xgols" only when XGOLS_PREFER_SYSTEM=1
//  4. Fetch the module and compile into XGOLS_GOBIN via xgo/gop/go
//     (no GitHub release zip; same model as vscode-xgo)
func resolveXgols() (string, error) {
	if path := strings.TrimSpace(os.Getenv("XGOLS_PATH")); path != "" {
		if fileExists(path) {
			return path, nil
		}
		return "", fmt.Errorf("XGOLS_PATH does not exist: %s", path)
	}

	requested := requestedModule()
	gobin := strings.TrimSpace(os.Getenv("XGOLS_GOBIN"))

	if reused := reuseManaged(gobin, requested); reused != "" {
		return reused, nil
	}
	if gobin != "" && fileExists(filepath.Join(gobin, exeName("xgols"))) {
		fmt.Fprintf(os.Stderr, "xgols-zed: managed xgols does not match %s; compiling locally\n", requested)
	}

	if preferSystemBinary() {
		if path, err := exec.LookPath("xgols"); err == nil {
			fmt.Fprintf(os.Stderr, "xgols-zed: using system xgols at %s (set prefer_system_binary=false to compile %s)\n", path, requested)
			return path, nil
		}
	}

	return installXgols(gobin, requested)
}

func requestedModule() string {
	module := strings.TrimSpace(os.Getenv("XGOLS_MODULE"))
	if module == "" {
		module = defaultXgolsModule
	}
	if !strings.Contains(module, "@") {
		module += "@latest"
	}
	return module
}

func reuseManaged(gobin, requested string) string {
	if gobin == "" {
		return ""
	}
	path := filepath.Join(gobin, exeName("xgols"))
	if !fileExists(path) {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(gobin, installedStampName))
	if err != nil {
		return ""
	}
	if strings.TrimSpace(string(data)) != requested {
		return ""
	}
	return path
}

func writeInstalledStamp(gobin, requested string) error {
	return os.WriteFile(filepath.Join(gobin, installedStampName), []byte(requested+"\n"), 0o644)
}

func preferSystemBinary() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("XGOLS_PREFER_SYSTEM"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func installXgols(gobin, module string) (string, error) {
	if gobin == "" {
		var err error
		gobin, err = defaultGoBin()
		if err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(gobin, 0o755); err != nil {
		return "", fmt.Errorf("create GOBIN %s: %w", gobin, err)
	}

	outPath := filepath.Join(gobin, exeName("xgols"))
	xgoPath, hasXgo := lookPath("xgo")
	gopPath, hasGop := lookPath("gop")
	_, hasGo := lookPath("go")

	if !hasXgo && !hasGop && !hasGo {
		return "", fmt.Errorf("cannot compile %s: `xgo` and `go` were not found in PATH. %s %s", module, goInstallHint, xgoInstallHint)
	}

	fmt.Fprintf(os.Stderr, "xgols-zed: fetching and compiling %s into %s (first launch can take a while)\n", module, gobin)

	// Prefer xgo/gop (handles go.mod replace directives). Fall back to
	// `go mod download` + `go build -C` because plain `go install module@version`
	// rejects modules that contain replace directives (xgols does).
	var lastErr error
	if hasXgo {
		if err := runInstall(xgoPath, []string{"install", "-v", module}, gobin); err == nil && fileExists(outPath) {
			_ = writeInstalledStamp(gobin, module)
			return outPath, nil
		} else if err != nil {
			lastErr = fmt.Errorf("xgo install failed: %w", err)
			fmt.Fprintf(os.Stderr, "xgols-zed: %v\n", lastErr)
		}
	}
	if hasGop {
		if err := runInstall(gopPath, []string{"install", "-v", module}, gobin); err == nil && fileExists(outPath) {
			_ = writeInstalledStamp(gobin, module)
			return outPath, nil
		} else if err != nil {
			lastErr = fmt.Errorf("gop install failed: %w", err)
			fmt.Fprintf(os.Stderr, "xgols-zed: %v\n", lastErr)
		}
	}
	if hasGo {
		if err := installWithGoBuild(module, outPath); err == nil && fileExists(outPath) {
			_ = writeInstalledStamp(gobin, module)
			return outPath, nil
		} else if err != nil {
			lastErr = fmt.Errorf("local compile failed: %w", err)
		}
	}

	if lastErr != nil {
		if !hasXgo {
			return "", fmt.Errorf("%w. %s", lastErr, xgoInstallHint)
		}
		return "", fmt.Errorf("install %s: %w", module, lastErr)
	}
	return "", fmt.Errorf("install succeeded but %s was not found", outPath)
}

func lookPath(name string) (string, bool) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", false
	}
	return path, true
}

func runInstall(bin string, args []string, gobin string) error {
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "GOBIN="+gobin, "GO111MODULE=on")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

type moduleDownloadJSON struct {
	Path    string
	Version string
	Dir     string
	Error   string
}

func installWithGoBuild(module, outPath string) error {
	fmt.Fprintf(os.Stderr, "xgols-zed: falling back to go mod download + go build for %s\n", module)
	cmd := exec.Command("go", "mod", "download", "-json", module)
	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	out, err := cmd.Output()
	if err != nil {
		// go mod download -json still prints JSON on failure
		var failed moduleDownloadJSON
		if jsonErr := json.Unmarshal(out, &failed); jsonErr == nil && failed.Error != "" {
			return fmt.Errorf("go mod download %s: %s", module, failed.Error)
		}
		return fmt.Errorf("go mod download %s: %w", module, err)
	}

	var info moduleDownloadJSON
	if err := json.Unmarshal(out, &info); err != nil {
		return fmt.Errorf("parse go mod download json: %w", err)
	}
	if info.Error != "" {
		return fmt.Errorf("go mod download %s: %s", module, info.Error)
	}
	if info.Dir == "" {
		return fmt.Errorf("go mod download %s: empty module directory", module)
	}

	build := exec.Command("go", "build", "-C", info.Dir, "-o", outPath, ".")
	build.Env = append(os.Environ(), "GO111MODULE=on")
	build.Stdout = os.Stderr
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("go build -C %s: %w", info.Dir, err)
	}
	return nil
}

func defaultGoBin() (string, error) {
	if gobin := strings.TrimSpace(os.Getenv("GOBIN")); gobin != "" {
		return gobin, nil
	}
	cmd := exec.Command("go", "env", "GOPATH")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("resolve GOPATH: %w. %s", err, goInstallHint)
	}
	gopath := strings.TrimSpace(string(out))
	if gopath == "" {
		return "", fmt.Errorf("GOPATH is empty; set GOBIN or GOPATH")
	}
	parts := filepath.SplitList(gopath)
	return filepath.Join(parts[0], "bin"), nil
}

func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
