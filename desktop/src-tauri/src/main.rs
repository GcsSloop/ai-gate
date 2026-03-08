#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use once_cell::sync::Lazy;
use serde_json::Value;
use std::io::{Read, Write};
use std::net::TcpStream;
use std::path::{Path, PathBuf};
use std::process::{Child, Command, Stdio};
use std::sync::Mutex;
use std::sync::atomic::{AtomicBool, Ordering};
use std::thread;
use std::time::Duration;
use tauri::image::Image;
use tauri::menu::{Menu, MenuBuilder};
use tauri::tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent};
use tauri::{AppHandle, Manager, Runtime};

static SIDECAR_CHILD: Lazy<Mutex<Option<Child>>> = Lazy::new(|| Mutex::new(None));
static ALLOW_DIRECT_EXIT: AtomicBool = AtomicBool::new(false);

const BACKEND_ADDR: &str = "127.0.0.1:6789";
const TRAY_ID: &str = "aigate-tray";
const MENU_OPEN_MAIN: &str = "open-main";
const MENU_PROXY_STATUS: &str = "proxy-status";
const MENU_PROXY_ENABLE: &str = "proxy-enable";
const MENU_PROXY_DISABLE: &str = "proxy-disable";
const MENU_QUIT: &str = "quit";
const MENU_ACCOUNT_PREFIX: &str = "account-select:";

#[derive(Clone, Default)]
struct ProxyStatusSnapshot {
    enabled: bool,
}

#[derive(Clone, Default)]
struct AccountSummary {
    id: i64,
    name: String,
    is_active: bool,
}

struct HttpResponse {
    status: u16,
    body: String,
}

fn main() {
    tauri::Builder::default()
        .invoke_handler(tauri::generate_handler![force_exit_app])
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
                .env("CODEX_ROUTER_LISTEN_ADDR", BACKEND_ADDR)
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
            drop(guard);

            setup_tray(app.handle())?;
            start_tray_sync_task(app.handle().clone());
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("error while building tauri application")
        .run(|app_handle, event| match event {
            tauri::RunEvent::WindowEvent { label, event, .. } => {
                if label == "main" {
                    if let tauri::WindowEvent::CloseRequested { api, .. } = event {
                        api.prevent_close();
                        if let Some(window) = app_handle.get_webview_window("main") {
                            let _ = window.hide();
                        }
                    }
                }
            }
            tauri::RunEvent::ExitRequested { api, .. } => {
                if ALLOW_DIRECT_EXIT.load(Ordering::Relaxed) {
                    shutdown_sidecar();
                    return;
                }
                match try_disable_proxy_before_exit() {
                    Ok(()) => shutdown_sidecar(),
                    Err(message) => {
                        api.prevent_exit();
                        show_main_window(app_handle);
                        emit_exit_conflict(app_handle, &message);
                    }
                }
            }
            tauri::RunEvent::Exit => {
                shutdown_sidecar();
            }
            _ => {}
        });
}

#[tauri::command]
fn force_exit_app<R: Runtime>(app: AppHandle<R>) {
    request_app_exit(&app);
}

fn request_app_exit<R: Runtime>(app: &AppHandle<R>) {
    ALLOW_DIRECT_EXIT.store(true, Ordering::Relaxed);
    app.exit(0);
}

fn setup_tray<R: Runtime>(app: &AppHandle<R>) -> Result<(), String> {
    let tray_menu = build_tray_menu(app)?;
    let mut builder = TrayIconBuilder::with_id(TRAY_ID)
        .menu(&tray_menu)
        .tooltip("AI Gate")
        .show_menu_on_left_click(true)
        .on_menu_event(|app, event| {
            let id = event.id().as_ref().to_string();
            handle_tray_menu_action(app, &id);
        })
        .on_tray_icon_event(|tray, event| {
            if let TrayIconEvent::Click {
                button,
                button_state,
                ..
            } = event
            {
                if (button == MouseButton::Left || button == MouseButton::Right)
                    && button_state == MouseButtonState::Up
                {
                    refresh_tray_menu(tray.app_handle());
                }
            }
        });

    if let Ok(icon) = Image::from_bytes(include_bytes!("../icons/tray-icon.png")) {
        builder = builder.icon(icon);
    } else if let Some(icon) = app.default_window_icon().cloned() {
        builder = builder.icon(icon);
    }

    builder
        .build(app)
        .map_err(|e| format!("build tray icon failed: {e}"))?;
    Ok(())
}

fn start_tray_sync_task<R: Runtime>(app: AppHandle<R>) {
    thread::spawn(move || loop {
        thread::sleep(Duration::from_secs(2));
        refresh_tray_menu(&app);
    });
}

fn build_tray_menu<R: Runtime>(app: &AppHandle<R>) -> Result<Menu<R>, String> {
    let proxy = fetch_proxy_status();
    let accounts = fetch_accounts();

    let proxy_status_text = if proxy.enabled {
        "代理状态：已开启"
    } else {
        "代理状态：未开启"
    };

    let mut builder = MenuBuilder::new(app)
        .text(MENU_OPEN_MAIN, "打开主界面")
        .separator()
        .text(MENU_PROXY_STATUS, proxy_status_text)
        .text(MENU_PROXY_ENABLE, "开启代理")
        .text(MENU_PROXY_DISABLE, "关闭代理")
        .separator();

    if accounts.is_empty() {
        builder = builder.text("accounts-empty", "Codex（无账户，请在主界面添加）");
    } else {
        for account in accounts {
            let id = format!("{MENU_ACCOUNT_PREFIX}{}", account.id);
            let label = if account.is_active {
                format!("✓ {}", account.name)
            } else {
                account.name
            };
            builder = builder.text(id, label);
        }
    }

    builder
        .separator()
        .text(MENU_QUIT, "退出")
        .build()
        .map_err(|e| format!("build tray menu failed: {e}"))
}

fn refresh_tray_menu<R: Runtime>(app: &AppHandle<R>) {
    let Some(tray) = app.tray_by_id(TRAY_ID) else {
        return;
    };
    let Ok(menu) = build_tray_menu(app) else {
        return;
    };
    let _ = tray.set_menu(Some(menu));
}

fn handle_tray_menu_action<R: Runtime>(app: &AppHandle<R>, id: &str) {
    match id {
        MENU_OPEN_MAIN => {
            show_main_window(app);
        }
        MENU_PROXY_ENABLE => {
            let _ = request_backend("POST", "/ai-router/api/settings/proxy/enable", "");
        }
        MENU_PROXY_DISABLE => {
            let _ = request_backend("POST", "/ai-router/api/settings/proxy/disable?skip_restore=1", "");
        }
        MENU_QUIT => {
            request_app_exit(app);
        }
        _ => {
            if let Some(account_id) = parse_account_menu_id(id) {
                let body = format!("{{\"is_active\":true}}");
                let _ = request_backend(
                    "PUT",
                    &format!("/ai-router/api/accounts/{account_id}"),
                    &body,
                );
            }
        }
    }

    if id != MENU_OPEN_MAIN {
        refresh_tray_menu(app);
    }
}

fn show_main_window<R: Runtime>(app: &AppHandle<R>) {
    if let Some(window) = app.get_webview_window("main") {
        let _ = window.show();
        let _ = window.unminimize();
        let _ = window.set_focus();
    }
}

fn try_disable_proxy_before_exit() -> Result<(), String> {
    let response = request_backend("POST", "/ai-router/api/settings/proxy/disable", "")
        .map_err(|e| format!("无法连接后端，无法自动恢复配置：{e}"))?;
    if response.status == 200 {
        return Ok(());
    }
    if response.status == 409 {
        let body = response.body.trim().to_string();
        if body == "proxy is not enabled" {
            return Ok(());
        }
        let message = if body.is_empty() {
            "config.toml changed externally; skip auto-restore to avoid overwrite".to_string()
        } else {
            body
        };
        return Err(message);
    }
    let body = response.body.trim();
    if body.is_empty() {
        return Err(format!("退出前关闭代理失败（HTTP {}）", response.status));
    }
    Err(body.to_string())
}

fn fetch_proxy_status() -> ProxyStatusSnapshot {
    let Ok(resp) = request_backend("GET", "/ai-router/api/settings/proxy/status", "") else {
        return ProxyStatusSnapshot::default();
    };
    if resp.status != 200 {
        return ProxyStatusSnapshot::default();
    }
    let Ok(value) = serde_json::from_str::<Value>(&resp.body) else {
        return ProxyStatusSnapshot::default();
    };
    ProxyStatusSnapshot {
        enabled: value
            .get("enabled")
            .and_then(|v| v.as_bool())
            .unwrap_or(false),
    }
}

fn fetch_accounts() -> Vec<AccountSummary> {
    let Ok(resp) = request_backend("GET", "/ai-router/api/accounts", "") else {
        return Vec::new();
    };
    if resp.status != 200 {
        return Vec::new();
    }
    let Ok(value) = serde_json::from_str::<Value>(&resp.body) else {
        return Vec::new();
    };
    let Some(items) = value.as_array() else {
        return Vec::new();
    };

    items
        .iter()
        .filter_map(|item| {
            let id = item.get("id")?.as_i64()?;
            let name = item.get("account_name")?.as_str()?.trim();
            let is_active = item
                .get("is_active")
                .and_then(|v| v.as_bool())
                .unwrap_or(false);
            Some(AccountSummary {
                id,
                name: name.to_string(),
                is_active,
            })
        })
        .collect()
}

fn request_backend(method: &str, path: &str, body: &str) -> Result<HttpResponse, String> {
    let mut stream = TcpStream::connect(BACKEND_ADDR).map_err(|e| format!("connect backend failed: {e}"))?;
    let request = format!(
        "{method} {path} HTTP/1.1\r\nHost: {BACKEND_ADDR}\r\nConnection: close\r\nContent-Type: application/json\r\nContent-Length: {}\r\n\r\n{body}",
        body.len()
    );
    stream
        .write_all(request.as_bytes())
        .map_err(|e| format!("write request failed: {e}"))?;

    let mut response = String::new();
    stream
        .read_to_string(&mut response)
        .map_err(|e| format!("read response failed: {e}"))?;

    let status = response
        .lines()
        .next()
        .and_then(|line| line.split_whitespace().nth(1))
        .and_then(|code| code.parse::<u16>().ok())
        .unwrap_or(0);
    let body = response
        .split_once("\r\n\r\n")
        .map(|(_, body)| body.to_string())
        .unwrap_or_default();

    Ok(HttpResponse { status, body })
}

fn parse_account_menu_id(id: &str) -> Option<i64> {
    id.strip_prefix(MENU_ACCOUNT_PREFIX)?.parse::<i64>().ok()
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

fn emit_exit_conflict(app: &tauri::AppHandle, message: &str) {
    let escaped = serde_json::to_string(message).unwrap_or_else(|_| "\"配置冲突\"".to_string());
    if let Some(window) = app.get_webview_window("main") {
        let script = format!(
            "window.dispatchEvent(new CustomEvent('aigate-exit-conflict', {{ detail: {{ message: {} }} }}));",
            escaped
        );
        let _ = window.eval(&script);
    }
}

#[cfg(test)]
mod tests {
    use super::parse_account_menu_id;

    #[test]
    fn parse_account_menu_id_accepts_valid_ids() {
        assert_eq!(parse_account_menu_id("account-select:7"), Some(7));
    }

    #[test]
    fn parse_account_menu_id_rejects_invalid_ids() {
        assert_eq!(parse_account_menu_id("account-select:abc"), None);
        assert_eq!(parse_account_menu_id("proxy-enable"), None);
    }
}
