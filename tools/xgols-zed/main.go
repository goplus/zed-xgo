// Command xgols-zed proxies the standard xgols language server and adjusts
// Zed's workspace folders when a worktree contains multiple nested Go/XGo
// modules.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type initializeMessage struct {
	Method string `json:"method"`
	Params struct {
		RootURI          string            `json:"rootUri"`
		WorkspaceFolders []workspaceFolder `json:"workspaceFolders"`
	} `json:"params"`
}

type workspaceFolder struct {
	URI  string `json:"uri"`
	Name string `json:"name"`
}

var errSourceFound = errors.New("XGo source found")

func main() {
	reader := bufio.NewReader(os.Stdin)
	body, err := readMessage(reader)
	if err != nil {
		fatalf("read initialize message: %v", err)
	}
	body = rewriteInitialize(body)

	xgolsPath, err := exec.LookPath("xgols")
	if err != nil {
		fatalf("find xgols in PATH: %v", err)
	}
	command := exec.Command(xgolsPath, os.Args[1:]...)
	command.Stderr = os.Stderr
	childIn, err := command.StdinPipe()
	if err != nil {
		fatalf("open xgols stdin: %v", err)
	}
	childOut, err := command.StdoutPipe()
	if err != nil {
		fatalf("open xgols stdout: %v", err)
	}
	if err := command.Start(); err != nil {
		fatalf("start xgols: %v", err)
	}

	if err := writeMessage(childIn, body); err != nil {
		_ = command.Process.Kill()
		fatalf("forward initialize message: %v", err)
	}

	go func() {
		_, _ = io.Copy(childIn, reader)
		_ = childIn.Close()
	}()
	go func() {
		_, _ = io.Copy(os.Stdout, childOut)
	}()

	if err := command.Wait(); err != nil {
		fatalf("xgols exited: %v", err)
	}
}

func rewriteInitialize(body []byte) []byte {
	var message initializeMessage
	if json.Unmarshal(body, &message) != nil || message.Method != "initialize" {
		return body
	}
	root := fileURIPath(message.Params.RootURI)
	if root == "" {
		return body
	}
	modules := discoverXGoModules(root)
	if len(modules) == 0 || (len(modules) == 1 && modules[0] == root) {
		return body
	}

	folders := make([]workspaceFolder, 0, len(modules))
	for _, module := range modules {
		folders = append(folders, workspaceFolder{
			URI:  filePathURI(module),
			Name: filepath.Base(module),
		})
	}

	var raw map[string]any
	if json.Unmarshal(body, &raw) != nil {
		return body
	}
	params, ok := raw["params"].(map[string]any)
	if !ok {
		return body
	}
	params["workspaceFolders"] = folders
	rewritten, err := json.Marshal(raw)
	if err != nil {
		return body
	}
	return rewritten
}

func discoverXGoModules(root string) []string {
	var modules []string
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		if path != root && shouldSkipDir(entry.Name()) {
			return filepath.SkipDir
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return filepath.SkipDir
		}
		if relative != "." && strings.Count(filepath.ToSlash(relative), "/") >= 3 {
			return filepath.SkipDir
		}
		if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil && moduleHasXGoSource(path) {
			modules = append(modules, path)
		}
		return nil
	})
	sort.Strings(modules)
	return modules
}

func moduleHasXGoSource(root string) bool {
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if path != root && shouldSkipDir(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if isXGoSource(path) {
			return errSourceFound
		}
		return nil
	})
	return errors.Is(err, errSourceFound)
}

func shouldSkipDir(name string) bool {
	return strings.HasPrefix(name, ".") ||
		name == "node_modules" ||
		name == "vendor" ||
		name == "target" ||
		name == "dist" ||
		name == "research"
}

func isXGoSource(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".xgo", ".gox", ".gop", ".spx", ".yap", ".gsh", ".rdx", ".gmx":
		return true
	default:
		return false
	}
}

func readMessage(reader *bufio.Reader) ([]byte, error) {
	contentLength := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			if _, err := fmt.Sscanf(strings.TrimSpace(value), "%d", &contentLength); err != nil {
				return nil, fmt.Errorf("invalid Content-Length: %w", err)
			}
		}
	}
	if contentLength < 0 {
		return nil, errors.New("missing Content-Length")
	}
	body := make([]byte, contentLength)
	_, err := io.ReadFull(reader, body)
	return body, err
}

func writeMessage(writer io.Writer, body []byte) error {
	var framed bytes.Buffer
	_, _ = fmt.Fprintf(&framed, "Content-Length: %d\r\n\r\n", len(body))
	_, _ = framed.Write(body)
	_, err := io.Copy(writer, &framed)
	return err
}

func fileURIPath(raw string) string {
	uri, err := url.Parse(raw)
	if err != nil || uri.Scheme != "file" || uri.Path == "" {
		return ""
	}
	path, err := url.PathUnescape(uri.Path)
	if err != nil {
		return ""
	}
	return filepath.Clean(path)
}

func filePathURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "xgols-zed: "+format+"\n", args...)
	os.Exit(1)
}
