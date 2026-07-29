# XGo for Zed

Experimental XGo language support for [Zed](https://zed.dev), powered by the official
[`xgols`](https://github.com/goplus/xgols) language server.

## Features

- Recognizes `.xgo`, `.gox`, `.gop`, `.yap`, `.spx`, `.gsh`, `.rdx`, and `.gmx`
- Tree-sitter-based syntax highlighting, indentation, brackets, outline, and text objects
- LSP completion, hover, diagnostics, formatting, and code navigation through `xgols`
- Optional nested-module workspace adapter for directories containing multiple repositories

## Requirements

- Go
- XGo
- `xgols`

Install the official language server:

```sh
xgo install github.com/goplus/xgols@latest
```

Make sure the installation directory, usually `$(go env GOPATH)/bin`, is available in
the environment used to start Zed.

## Install as a development extension

1. Clone this repository.
2. Open Zed's command palette.
3. Run `Extensions: Install Dev Extension`.
4. Select the repository root.

When an XGo module itself is opened as the Zed project root, the extension starts
`xgols` directly.

## Aggregate workspaces

If the Zed project root contains several independent repositories and the real XGo
module is nested below it, build the optional `xgols-zed` adapter:

```sh
cd tools/xgols-zed
go build -o "$(go env GOPATH)/bin/xgols-zed" .
```

The extension prefers `xgols-zed` when it is available and otherwise falls back to
`xgols`.

The adapter does not implement language intelligence. It only rewrites the first LSP
`initialize` request so that `workspaceFolders` points to nested directories that:

1. contain a `go.mod`; and
2. contain at least one XGo source file.

All later LSP traffic is forwarded unchanged to the official `xgols` process.

You can configure an explicit adapter path in Zed:

```json
{
  "lsp": {
    "xgols": {
      "binary": {
        "path": "/absolute/path/to/xgols-zed"
      }
    }
  }
}
```

## Current limitations

- The extension currently reuses the Go Tree-sitter grammar. XGo-specific syntax may
  require dedicated grammar rules in the future.
- The optional workspace adapter uses bounded automatic module discovery and does not
  parse `go.work` yet.
- The extension does not yet provide the VS Code extension's debugger, test UI, status
  bar, or automatic tool installation.
- Some mixed Go/XGo implementation queries remain limited by `xgols`.

## Development

Check the Rust extension:

```sh
rustup target add wasm32-wasip2
cargo fmt --check
cargo check --target wasm32-wasip2
```

Check the workspace adapter:

```sh
cd tools/xgols-zed
gofmt -w .
go test ./...
```

Generated extension binaries, grammar checkouts, and Cargo build outputs are ignored
and should not be committed.

## License

Apache License 2.0. See `THIRD_PARTY_NOTICES.md` for third-party attributions.
