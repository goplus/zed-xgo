# XGo for Zed

Experimental XGo language support for [Zed](https://zed.dev), powered by the official
[`xgols`](https://github.com/goplus/xgols) language server.

Works the same on **Windows, macOS, and Linux**: `xgols` is fetched and compiled
**on your machine** with your `xgo`/`go` (the same model as
[vscode-xgo](https://github.com/goplus/vscode-xgo)). No GitHub release zips.

## What you do

1. Install [Go](https://go.dev/dl/) and [XGo](https://xgo.dev), and keep `go` / `xgo` on `PATH`.
2. Restart Zed so it sees the new PATH.
3. Install this extension (Zed marketplace, or **Install Dev Extension** while developing).
4. Open an XGo file (`.xgo`, `.gop`, `.gox`, `.spx`, …).

The rest is automatic. The first open compiles the host adapter and `xgols`
(can take a while). Later opens reuse the cache. You do not run `xgo install`
yourself.

If the extension sources change, the adapter is rebuilt automatically. If the
pinned `xgols` version changes, `xgols` is compiled again. You do not delete
`bin` by hand.

## Requirements

- [Go](https://go.dev/dl/)
- [XGo](https://xgo.dev) — `xgo install` understands `xgols`'s `replace` directives

`xgols` itself is installed automatically. This extension version pins
`github.com/goplus/xgols@v0.14.1` (override with settings if you need another tag).

## How it works

Zed's WASM extension cannot compile anything. On LSP start it launches
`xgols-zed`, which:

1. Compiles itself with `go build` when missing or when adapter sources changed
2. Runs `xgo install github.com/goplus/xgols@v0.14.1` into the extension `bin`
   directory (falls back to `gop install`, then `go mod download` + `go build -C`)
3. Starts that locally built `xgols`

## Settings

```json
{
  "lsp": {
    "xgols": {
      "settings": {
        "prefer_system_binary": false,
        "xgols_module": "github.com/goplus/xgols",
        "xgols_version": "v0.14.1"
      },
      "binary": {
        "path": "/absolute/path/to/xgols-zed"
      }
    }
  }
}
```

- `prefer_system_binary`: default `false` so the extension compiles `xgols` with
  your toolchain. Set `true` only to reuse a PATH-installed `xgols`.
- `xgols_version`: module version / tag (`v0.14.1`, `latest`, …)
- `xgols_module`: override the module path (may already include `@version`)
- `binary.path`: bypass bootstrap and run this executable directly

## Aggregate workspaces

When the Zed project root contains several independent repositories, `xgols-zed`
rewrites the first LSP `initialize` request so `workspaceFolders` points at nested
directories that:

1. contain a `go.mod`; and
2. contain at least one XGo source file.

Prefer opening a single module root rather than a huge monorepo.

## Current limitations

- The extension reuses the Go Tree-sitter grammar.
- Nested-module discovery is bounded and does not parse `go.work` yet.
- Debugger / test UI from the VS Code extension are not ported yet.
- Some Go modules cannot be extracted on Windows because of illegal NTFS paths
  in upstream fixtures. That is a module-cache issue, not an LSP install failure.
  macOS and Linux are not affected by those filenames.

## Development

```sh
rustup target add wasm32-wasip2
cargo fmt --check
cargo check --target wasm32-wasip2

cd tools/xgols-zed
gofmt -w .
go test ./...
```

After changing `src/lib.rs`, click **Rebuild** on the Dev Extension (or reinstall it)
so Zed picks up the new WASM. The next time you open an XGo file, a changed
adapter is rebuilt automatically.

## License

Apache License 2.0. See `THIRD_PARTY_NOTICES.md` for third-party attributions.
