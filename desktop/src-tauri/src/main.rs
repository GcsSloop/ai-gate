#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use once_cell::sync::Lazy;
use std::path::{Path, PathBuf};
use std::process::{Child, Command, Stdio};
use std::sync::Mutex;
use tauri::Manager;

static SIDECAR_CHILD: Lazy<Mutex<Option<Child>>> = Lazy::new(|| Mutex::new(None));

fn main() {
    tauri::Builder::default()
        .setup(|app| {
            let sidecar_path = resolve_sidecar_path(app.handle())?;
            let home_dir = app
                .path()
                .home_dir()
                .map_err(|e| format!("resolve home_dir failed: {e}"))?;
            let data_dir = home_dir.join(".aigate").join("data");
            std::fs::create_dir_all(&data_dir).map_err(|e| format!("create data_dir failed: {e}"))?;

            let database_path = data_dir.join("aigate.sqlite");
            let mut command = Command::new(&sidecar_path);
            command
                .env("CODEX_ROUTER_LISTEN_ADDR", "127.0.0.1:6789")
                .env("CODEX_ROUTER_DATABASE_PATH", database_path)
                .stdout(Stdio::null())
                .stderr(Stdio::null());

            let child = command
                .spawn()
                .map_err(|e| format!("spawn sidecar {} failed: {e}", sidecar_path.display()))?;

            let mut guard = SIDECAR_CHILD
                .lock()
                .map_err(|_| "sidecar child lock poisoned".to_string())?;
            *guard = Some(child);
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("error while building tauri application")
        .run(|_app_handle, event| {
            match event {
                tauri::RunEvent::ExitRequested { .. } | tauri::RunEvent::Exit => {
                    shutdown_sidecar();
                }
                _ => {}
            }
        });
}

fn resolve_sidecar_path(app: &tauri::AppHandle) -> Result<PathBuf, String> {
    let mut candidates: Vec<PathBuf> = Vec::new();

    if cfg!(debug_assertions) {
        if let Ok(manifest_dir) = std::env::var("CARGO_MANIFEST_DIR") {
            candidates.push(Path::new(&manifest_dir).join("bin/routerd-universal-apple-darwin"));
        }
    }

    if let Ok(resources_dir) = app.path().resource_dir() {
        candidates.push(resources_dir.join("bin/routerd-universal-apple-darwin"));
    }

    if let Ok(exe) = std::env::current_exe() {
        if let Some(macos_dir) = exe.parent() {
            candidates.push(
                macos_dir
                    .join("../Resources/bin/routerd-universal-apple-darwin")
                    .to_path_buf(),
            );
        }
    }

    for candidate in candidates {
        if candidate.exists() {
            return Ok(candidate);
        }
    }

    Err("routerd sidecar not found, expected bin/routerd-universal-apple-darwin".to_string())
}

fn shutdown_sidecar() {
    let mut guard = match SIDECAR_CHILD.lock() {
        Ok(guard) => guard,
        Err(_) => return,
    };

    if let Some(child) = guard.as_mut() {
        let _ = child.kill();
        let _ = child.wait();
    }
    *guard = None;
}
