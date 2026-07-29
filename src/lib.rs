use std::fs;

use zed_extension_api::{self as zed, settings::LspSettings, LanguageServerId, Result};

struct XgoExtension {
    cached_binary_path: Option<String>,
}

impl XgoExtension {
    fn binary_path(
        &mut self,
        language_server_id: &LanguageServerId,
        worktree: &zed::Worktree,
    ) -> Result<String> {
        let settings = LspSettings::for_worktree(language_server_id.as_ref(), worktree);
        let configured = settings.ok().and_then(|value| value.binary);
        if let Some(path) = configured.and_then(|binary| binary.path) {
            return Ok(path);
        }

        if let Some(path) = worktree.which("xgols-zed") {
            return Ok(path);
        }

        if let Some(path) = worktree.which("xgols") {
            return Ok(path);
        }

        if let Some(path) = &self.cached_binary_path {
            if fs::metadata(path).is_ok_and(|metadata| metadata.is_file()) {
                return Ok(path.clone());
            }
        }

        Err("No XGo language server was found. Install xgols-zed or xgols in PATH, or configure lsp.xgols.binary.path in Zed settings.".into())
    }
}

impl zed::Extension for XgoExtension {
    fn new() -> Self {
        Self {
            cached_binary_path: None,
        }
    }

    fn language_server_command(
        &mut self,
        language_server_id: &LanguageServerId,
        worktree: &zed::Worktree,
    ) -> Result<zed::Command> {
        let path = self.binary_path(language_server_id, worktree)?;
        let settings = LspSettings::for_worktree(language_server_id.as_ref(), worktree);
        let args = settings
            .ok()
            .and_then(|value| value.binary)
            .and_then(|binary| binary.arguments)
            .unwrap_or_default();

        Ok(zed::Command {
            command: path,
            args,
            env: vec![],
        })
    }

    fn language_server_initialization_options(
        &mut self,
        language_server_id: &LanguageServerId,
        worktree: &zed::Worktree,
    ) -> Result<Option<zed::serde_json::Value>> {
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
    ) -> Result<Option<zed::serde_json::Value>> {
        Ok(
            LspSettings::for_worktree(language_server_id.as_ref(), worktree)
                .ok()
                .and_then(|value| value.settings),
        )
    }
}

zed::register_extension!(XgoExtension);
