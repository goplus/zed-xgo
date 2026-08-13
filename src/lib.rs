use std::fs;
use std::path::{Path, PathBuf};

use zed_extension_api::{
    self as zed, serde_json, settings::LspSettings, LanguageServerId, Os, Result,
};

const DEFAULT_XGOLS_MODULE: &str = "github.com/goplus/xgols";
const DEFAULT_XGOLS_VERSION: &str = "v0.14.1";
const ADAPTER_DIR: &str = "xgols-zed-src";
const MANAGED_BIN_DIR: &str = "bin";
const ADAPTER_REV_FILE: &str = "xgols-zed.rev";

struct XgoExtension {}

#[derive(Default)]
struct XgoLspSettings {
    prefer_system_binary: bool,
    xgols_version: Option<String>,
    xgols_module: Option<String>,
}

impl XgoExtension {
    fn parse_settings(
        language_server_id: &LanguageServerId,
        worktree: &zed::Worktree,
    ) -> XgoLspSettings {
        let mut parsed = XgoLspSettings {
            prefer_system_binary: false,
            ..XgoLspSettings::default()
        };

        let Ok(lsp_settings) = LspSettings::for_worktree(language_server_id.as_ref(), worktree)
        else {
            return parsed;
        };
        let Some(settings) = lsp_settings.settings else {
            return parsed;
        };

        if let Some(value) = settings
            .get("prefer_system_binary")
            .and_then(|v| v.as_bool())
        {
            parsed.prefer_system_binary = value;
        }
        if let Some(value) = settings
            .get("xgols_version")
            .and_then(|v| v.as_str())
            .map(str::to_string)
        {
            parsed.xgols_version = Some(value);
        }
        if let Some(value) = settings
            .get("xgols_module")
            .and_then(|v| v.as_str())
            .map(str::to_string)
        {
            parsed.xgols_module = Some(value);
        }

        parsed
    }

    fn module_spec(settings: &XgoLspSettings) -> String {
        let module = settings
            .xgols_module
            .as_deref()
            .unwrap_or(DEFAULT_XGOLS_MODULE);
        if module.contains('@') {
            return module.to_string();
        }
        let version = settings
            .xgols_version
            .as_deref()
            .unwrap_or(DEFAULT_XGOLS_VERSION);
        format!("{module}@{version}")
    }

    fn managed_bin_dir() -> Result<PathBuf> {
        let dir = PathBuf::from(MANAGED_BIN_DIR);
        fs::create_dir_all(&dir).map_err(|e| format!("create managed bin dir: {e}"))?;
        Ok(dir)
    }

    fn absolute(path: &Path) -> Result<String> {
        let cwd = std::env::current_dir().map_err(|e| format!("current_dir: {e}"))?;
        Ok(cwd.join(path).to_string_lossy().into_owned())
    }

    fn adapter_binary_name() -> &'static str {
        match zed::current_platform() {
            (Os::Windows, _) => "xgols-zed.exe",
            _ => "xgols-zed",
        }
    }

    fn xgols_binary_name() -> &'static str {
        match zed::current_platform() {
            (Os::Windows, _) => "xgols.exe",
            _ => "xgols",
        }
    }

    fn adapter_source_fingerprint() -> String {
        fn djb2(parts: &[&str]) -> u64 {
            let mut hash: u64 = 5381;
            for part in parts {
                hash = hash.wrapping_mul(33).wrapping_add(part.len() as u64);
                for byte in part.as_bytes() {
                    hash = hash.wrapping_mul(33).wrapping_add(u64::from(*byte));
                }
            }
            hash
        }
        format!(
            "{:016x}",
            djb2(&[
                include_str!("../tools/xgols-zed/go.mod"),
                include_str!("../tools/xgols-zed/main.go"),
                include_str!("../tools/xgols-zed/ensure.go"),
            ])
        )
    }

    fn managed_adapter_up_to_date() -> bool {
        let adapter = PathBuf::from(MANAGED_BIN_DIR).join(Self::adapter_binary_name());
        if !adapter.is_file() {
            return false;
        }
        let Ok(stamp) = fs::read_to_string(PathBuf::from(MANAGED_BIN_DIR).join(ADAPTER_REV_FILE))
        else {
            return false;
        };
        stamp.trim() == Self::adapter_source_fingerprint()
    }

    fn needs_local_compile(settings: &XgoLspSettings) -> bool {
        !Self::managed_xgols_ready(settings) || !Self::managed_adapter_up_to_date()
    }

    fn managed_xgols_ready(settings: &XgoLspSettings) -> bool {
        let dir = PathBuf::from(MANAGED_BIN_DIR);
        let bin = dir.join(Self::xgols_binary_name());
        if !bin.is_file() {
            return false;
        }
        let Ok(stamp) = fs::read_to_string(dir.join("xgols.installed")) else {
            return false;
        };
        stamp.trim() == Self::module_spec(settings)
    }

    fn write_adapter_sources(&self) -> Result<PathBuf> {
        let dir = PathBuf::from(ADAPTER_DIR);
        fs::create_dir_all(&dir).map_err(|e| format!("create adapter source dir: {e}"))?;

        let files = [
            ("go.mod", include_str!("../tools/xgols-zed/go.mod")),
            ("main.go", include_str!("../tools/xgols-zed/main.go")),
            ("ensure.go", include_str!("../tools/xgols-zed/ensure.go")),
        ];
        for (name, contents) in files {
            let path = dir.join(name);
            fs::write(&path, contents).map_err(|e| format!("write {name}: {e}"))?;
        }
        Ok(dir)
    }

    fn push_bootstrap_env(
        env: &mut Vec<(String, String)>,
        settings: &XgoLspSettings,
        managed_bin_abs: String,
    ) {
        env.push(("XGOLS_MODULE".into(), Self::module_spec(settings)));
        env.push(("XGOLS_GOBIN".into(), managed_bin_abs));
        env.push((
            "XGOLS_PREFER_SYSTEM".into(),
            if settings.prefer_system_binary {
                "1".into()
            } else {
                "0".into()
            },
        ));
    }

    fn adapter_command(
        &mut self,
        worktree: &zed::Worktree,
        settings: &XgoLspSettings,
        user_args: Vec<String>,
    ) -> Result<zed::Command> {
        let managed_bin = Self::managed_bin_dir()?;
        let managed_bin_abs = Self::absolute(&managed_bin)?;

        let mut env = worktree.shell_env();
        Self::push_bootstrap_env(&mut env, settings, managed_bin_abs);

        // Always refresh embedded adapter sources so a Dev Extension rebuild
        // can detect source changes without deleting bin/ by hand.
        let src_dir = self.write_adapter_sources()?;
        let managed_adapter = managed_bin.join(Self::adapter_binary_name());
        if Self::managed_adapter_up_to_date() {
            let path = Self::absolute(&managed_adapter)?;
            return Ok(zed::Command {
                command: path,
                args: user_args,
                env,
            });
        }

        let go = worktree.which("go").ok_or_else(|| {
            "Go was not found in PATH. Install Go from https://go.dev/dl/, then restart Zed. \
             This extension compiles xgols locally (it does not download GitHub binaries)."
                .to_string()
        })?;

        let src_abs = Self::absolute(&src_dir)?;
        let out_abs = Self::absolute(&managed_adapter)?;
        let rev_abs = Self::absolute(&managed_bin.join(ADAPTER_REV_FILE))?;
        let fingerprint = Self::adapter_source_fingerprint();

        match zed::current_platform().0 {
            Os::Windows => {
                let script = format!(
                    r#"@echo off
echo xgols-zed: compiling adapter with go build
"{go}" build -C "{src}" -o "{out}" .
if errorlevel 1 (
  echo xgols-zed: go build failed. Install Go from https://go.dev/dl/ then restart Zed.
  exit /b 1
)
> "{rev}" echo {fp}
"{out}" %*"#,
                    go = go,
                    src = src_abs,
                    out = out_abs,
                    rev = rev_abs,
                    fp = fingerprint,
                );
                let script_path = PathBuf::from("run-xgols-zed.cmd");
                fs::write(&script_path, script)
                    .map_err(|e| format!("write bootstrap script: {e}"))?;
                let script_abs = Self::absolute(&script_path)?;
                Ok(zed::Command {
                    command: script_abs,
                    args: user_args,
                    env,
                })
            }
            _ => {
                let script = format!(
                    r#"#!/bin/sh
set -e
echo "xgols-zed: compiling adapter with go build" >&2
"{go}" build -C "{src}" -o "{out}" .
printf '%s\n' "{fp}" > "{rev}"
exec "{out}" "$@"
"#,
                    go = go,
                    src = src_abs,
                    out = out_abs,
                    rev = rev_abs,
                    fp = fingerprint,
                );
                let script_path = PathBuf::from("run-xgols-zed.sh");
                fs::write(&script_path, script)
                    .map_err(|e| format!("write bootstrap script: {e}"))?;
                zed::make_file_executable("run-xgols-zed.sh")?;
                let script_abs = Self::absolute(&script_path)?;
                Ok(zed::Command {
                    command: script_abs,
                    args: user_args,
                    env,
                })
            }
        }
    }
}

impl zed::Extension for XgoExtension {
    fn new() -> Self {
        Self {}
    }

    fn language_server_command(
        &mut self,
        language_server_id: &LanguageServerId,
        worktree: &zed::Worktree,
    ) -> Result<zed::Command> {
        let lsp_settings = LspSettings::for_worktree(language_server_id.as_ref(), worktree).ok();
        let user_args = lsp_settings
            .as_ref()
            .and_then(|value| value.binary.as_ref())
            .and_then(|binary| binary.arguments.clone())
            .unwrap_or_default();

        if let Some(path) = lsp_settings
            .as_ref()
            .and_then(|value| value.binary.as_ref())
            .and_then(|binary| binary.path.clone())
        {
            return Ok(zed::Command {
                command: path,
                args: user_args,
                env: worktree.shell_env(),
            });
        }

        let settings = Self::parse_settings(language_server_id, worktree);
        let compiling = Self::needs_local_compile(&settings);

        zed::set_language_server_installation_status(
            language_server_id,
            &if compiling {
                zed::LanguageServerInstallationStatus::Downloading
            } else {
                zed::LanguageServerInstallationStatus::CheckingForUpdate
            },
        );

        match self.adapter_command(worktree, &settings, user_args) {
            Ok(command) => {
                if !compiling {
                    zed::set_language_server_installation_status(
                        language_server_id,
                        &zed::LanguageServerInstallationStatus::None,
                    );
                }
                Ok(command)
            }
            Err(err) => {
                zed::set_language_server_installation_status(
                    language_server_id,
                    &zed::LanguageServerInstallationStatus::Failed(err.clone()),
                );
                Err(err)
            }
        }
    }

    fn language_server_initialization_options(
        &mut self,
        language_server_id: &LanguageServerId,
        worktree: &zed::Worktree,
    ) -> Result<Option<serde_json::Value>> {
        Ok(
            LspSettings::for_worktree(language_server_id.as_ref(), worktree)
                .ok()
                .and_then(|value| value.initialization_options),
        )
    }

    fn language_server_workspace_configuration(
        &mut self,
        language_server_id: &LanguageServerId,
        worktree: &zed::Worktree,
    ) -> Result<Option<serde_json::Value>> {
        Ok(
            LspSettings::for_worktree(language_server_id.as_ref(), worktree)
                .ok()
                .and_then(|value| value.settings),
        )
    }
}

zed::register_extension!(XgoExtension);
