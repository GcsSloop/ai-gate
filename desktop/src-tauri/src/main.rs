#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use once_cell::sync::Lazy;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::io::{Read, Write};
use std::net::{TcpStream, ToSocketAddrs};
use std::path::{Path, PathBuf};
use std::process::{Child, ChildStdin, Command, Stdio};
use std::sync::Mutex;
use std::thread::JoinHandle;
use std::time::Duration;
use tauri::image::Image;
use tauri::menu::{Menu, MenuBuilder};
use tauri::tray::TrayIconBuilder;
use tauri::{AppHandle, Emitter, Manager, Runtime};

static SIDECAR_CHILD: Lazy<Mutex<Option<Child>>> = Lazy::new(|| Mutex::new(None));
static SIDECAR_HEARTBEAT: Lazy<Mutex<Option<JoinHandle<()>>>> = Lazy::new(|| Mutex::new(None));
static DESKTOP_RUNTIME: Lazy<Mutex<DesktopRuntime>> =
    Lazy::new(|| Mutex::new(DesktopRuntime::default()));

const DEFAULT_PROXY_HOST: &str = "127.0.0.1";
const DEFAULT_PROXY_PORT: u16 = 6789;
const SETTINGS_CACHE_FILE: &str = "desktop-settings.json";
const LAUNCH_AGENT_LABEL: &str = "com.aigate.desktop";
const TRAY_ID: &str = "aigate-tray";
const MENU_OPEN_MAIN: &str = "open-main";
const MENU_PROXY_STATUS: &str = "proxy-status";
const MENU_PROXY_ENABLE: &str = "proxy-enable";
const MENU_PROXY_DISABLE: &str = "proxy-disable";
const MENU_QUIT: &str = "quit";
const MENU_ACCOUNT_PREFIX: &str = "account-select:";
const BACKEND_STATE_CHANGED_EVENT: &str = "aigate-backend-state-changed";
const ABOUT_DESCRIPTION: &str =
    "AI Gate 是一个本地桌面代理与账号编排工具，用于统一管理路由、故障转移与数据备份。";
const ABOUT_AUTHOR: &str = "GcsSloop";
const BACKEND_REQUEST_TIMEOUT_MS: u64 = 1500;
const SIDECAR_HEARTBEAT_INTERVAL: Duration = Duration::from_secs(1);

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq, Eq)]
#[serde(default)]
struct DesktopSettingsCache {
    launch_at_login: bool,
    silent_start: bool,
    close_to_tray: bool,
    proxy_host: String,
    proxy_port: u16,
}

impl Default for DesktopSettingsCache {
    fn default() -> Self {
        Self {
            launch_at_login: false,
            silent_start: false,
            close_to_tray: true,
            proxy_host: DEFAULT_PROXY_HOST.to_string(),
            proxy_port: DEFAULT_PROXY_PORT,
        }
    }
}

impl DesktopSettingsCache {
    fn from_app_settings(value: AppSettingsPayload) -> Self {
        let defaults = Self::default();
        let proxy_host = value.proxy_host.trim();
        let proxy_port = if value.proxy_port == 0 {
            defaults.proxy_port
        } else {
            value.proxy_port
        };
        Self {
            launch_at_login: value.launch_at_login,
            silent_start: value.silent_start,
            close_to_tray: value.close_to_tray,
            proxy_host: if proxy_host.is_empty() {
                defaults.proxy_host
            } else {
                proxy_host.to_string()
            },
            proxy_port,
        }
    }

    fn backend_addr(&self) -> String {
        format!("{}:{}", self.proxy_host, self.proxy_port)
    }

    fn backend_api_base(&self) -> String {
        format!("http://{}/ai-router/api", self.backend_addr())
    }
}

#[derive(Clone, Debug, Serialize, Deserialize)]
struct AppSettingsPayload {
    launch_at_login: bool,
    silent_start: bool,
    close_to_tray: bool,
    show_proxy_switch_on_home: bool,
    proxy_host: String,
    proxy_port: u16,
    auto_failover_enabled: bool,
    auto_backup_interval_hours: i32,
    backup_retention_count: i32,
}

#[derive(Clone, Debug, Serialize)]
struct DesktopShellContext {
    backend_addr: String,
    backend_api_base: String,
    launch_at_login: bool,
    silent_start: bool,
    close_to_tray: bool,
}

impl DesktopShellContext {
    fn from_cache(cache: &DesktopSettingsCache) -> Self {
        Self {
            backend_addr: cache.backend_addr(),
            backend_api_base: cache.backend_api_base(),
            launch_at_login: cache.launch_at_login,
            silent_start: cache.silent_start,
            close_to_tray: cache.close_to_tray,
        }
    }
}

#[derive(Clone, Debug, Serialize)]
struct AppMetadataPayload {
    name: String,
    version: String,
    description: String,
    author: String,
}

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

#[derive(Clone, Default)]
struct TrayStateSnapshot {
    proxy: ProxyStatusSnapshot,
    accounts: Vec<AccountSummary>,
    active_account_name: Option<String>,
}

struct HttpResponse {
    status: u16,
    body: String,
}

#[derive(Clone, Default)]
struct DesktopRuntime {
    sidecar_path: PathBuf,
    database_path: PathBuf,
    settings_path: PathBuf,
    settings_cache: DesktopSettingsCache,
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum WindowCloseAction {
    MinimizeWindow,
    ExitApp,
}

fn main() {
    tauri::Builder::default()
        .invoke_handler(tauri::generate_handler![
            force_exit_app,
            refresh_tray_state,
            get_desktop_shell_context,
            apply_app_settings,
            get_app_metadata
        ])
        .setup(|app| {
            let cache = initialize_runtime(app.handle())?;
            if let Err(err) = sync_launch_agent(cache.launch_at_login) {
                eprintln!("sync launch agent failed: {err}");
            }
            spawn_sidecar()?;
            setup_tray(app.handle())?;
            if cache.silent_start {
                if let Some(window) = app.get_webview_window("main") {
                    let _ = window.hide();
                }
            }
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("error while building tauri application")
        .run(|app_handle, event| match event {
            tauri::RunEvent::WindowEvent { label, event, .. } => {
                if label == "main" {
                    if let tauri::WindowEvent::CloseRequested { api, .. } = event {
                        match window_close_action(current_settings_cache().close_to_tray) {
                            WindowCloseAction::MinimizeWindow => {
                                api.prevent_close();
                                if let Some(window) = app_handle.get_webview_window("main") {
                                    let _ = window.minimize();
                                }
                            }
                            WindowCloseAction::ExitApp => {
                                api.prevent_close();
                                shutdown_sidecar();
                                app_handle.exit(0);
                            }
                        }
                    }
                }
            }
            tauri::RunEvent::Reopen { .. } => {
                show_main_window(app_handle);
            }
            tauri::RunEvent::Exit => {
                shutdown_sidecar();
            }
            _ => {}
        });
}

fn window_close_action(close_to_tray: bool) -> WindowCloseAction {
    if close_to_tray {
        WindowCloseAction::MinimizeWindow
    } else {
        WindowCloseAction::ExitApp
    }
}

#[tauri::command]
fn force_exit_app<R: Runtime>(app: AppHandle<R>) {
    shutdown_sidecar();
    app.exit(0);
}

#[tauri::command]
fn refresh_tray_state<R: Runtime>(app: AppHandle<R>) -> Result<(), String> {
    refresh_tray_state_from_backend(&app);
    Ok(())
}

#[tauri::command]
fn get_desktop_shell_context() -> Result<DesktopShellContext, String> {
    Ok(DesktopShellContext::from_cache(&current_settings_cache()))
}

#[tauri::command]
fn apply_app_settings<R: Runtime>(
    app: AppHandle<R>,
    payload: AppSettingsPayload,
) -> Result<DesktopShellContext, String> {
    let cache = DesktopSettingsCache::from_app_settings(payload);
    let restart_required = persist_runtime_settings(cache.clone())?;
    sync_launch_agent(cache.launch_at_login)?;
    if restart_required {
        restart_sidecar()?;
    }
    refresh_tray_state_from_backend(&app);
    emit_backend_state_changed(&app);
    Ok(DesktopShellContext::from_cache(&cache))
}

#[tauri::command]
fn get_app_metadata<R: Runtime>(app: AppHandle<R>) -> AppMetadataPayload {
    let package = app.package_info();
    AppMetadataPayload {
        name: package.name.clone(),
        version: package.version.to_string(),
        description: ABOUT_DESCRIPTION.to_string(),
        author: ABOUT_AUTHOR.to_string(),
    }
}

fn initialize_runtime(app: &tauri::AppHandle) -> Result<DesktopSettingsCache, String> {
    let sidecar_path = resolve_sidecar_path(app)?;
    let home_dir = app
        .path()
        .home_dir()
        .map_err(|e| format!("resolve home_dir failed: {e}"))?;
    let data_dir = home_dir.join(".aigate").join("data");
    std::fs::create_dir_all(&data_dir).map_err(|e| format!("create data_dir failed: {e}"))?;

    let database_path = data_dir.join("aigate.sqlite");
    let settings_path = data_dir.join(SETTINGS_CACHE_FILE);
    let settings_cache = load_settings_cache(&settings_path);

    let mut runtime = DESKTOP_RUNTIME
        .lock()
        .map_err(|_| "desktop runtime lock poisoned".to_string())?;
    *runtime = DesktopRuntime {
        sidecar_path,
        database_path,
        settings_path,
        settings_cache: settings_cache.clone(),
    };
    Ok(settings_cache)
}

fn load_settings_cache(settings_path: &Path) -> DesktopSettingsCache {
    let Ok(raw) = std::fs::read_to_string(settings_path) else {
        return DesktopSettingsCache::default();
    };
    serde_json::from_str::<DesktopSettingsCache>(&raw).unwrap_or_default()
}

fn persist_runtime_settings(cache: DesktopSettingsCache) -> Result<bool, String> {
    let mut runtime = DESKTOP_RUNTIME
        .lock()
        .map_err(|_| "desktop runtime lock poisoned".to_string())?;
    let previous_addr = runtime.settings_cache.backend_addr();
    persist_settings_cache(&runtime.settings_path, &cache)?;
    runtime.settings_cache = cache.clone();
    Ok(previous_addr != cache.backend_addr())
}

fn persist_settings_cache(
    settings_path: &Path,
    cache: &DesktopSettingsCache,
) -> Result<(), String> {
    if let Some(parent) = settings_path.parent() {
        std::fs::create_dir_all(parent)
            .map_err(|e| format!("create settings cache dir failed: {e}"))?;
    }
    let raw = serde_json::to_vec_pretty(cache)
        .map_err(|e| format!("serialize settings cache failed: {e}"))?;
    std::fs::write(settings_path, raw).map_err(|e| format!("write settings cache failed: {e}"))
}

fn current_settings_cache() -> DesktopSettingsCache {
    DESKTOP_RUNTIME
        .lock()
        .map(|runtime| runtime.settings_cache.clone())
        .unwrap_or_default()
}

fn current_backend_addr() -> String {
    current_settings_cache().backend_addr()
}

fn spawn_sidecar() -> Result<(), String> {
    let runtime = DESKTOP_RUNTIME
        .lock()
        .map_err(|_| "desktop runtime lock poisoned".to_string())?
        .clone();

    let mut command = Command::new(&runtime.sidecar_path);
    command
        .env(
            "CODEX_ROUTER_LISTEN_ADDR",
            runtime.settings_cache.backend_addr(),
        )
        .env("CODEX_ROUTER_DATABASE_PATH", runtime.database_path)
        .env("CODEX_ROUTER_PARENT_HEARTBEAT", "stdin")
        .stdin(Stdio::piped())
        .stdout(Stdio::null())
        .stderr(Stdio::null());

    let mut child = command.spawn().map_err(|e| {
        format!(
            "spawn sidecar {} failed: {e}",
            runtime.sidecar_path.display()
        )
    })?;
    let stdin = child
        .stdin
        .take()
        .ok_or_else(|| "capture sidecar stdin failed".to_string())?;
    if let Err(err) = start_sidecar_heartbeat(stdin) {
        let _ = child.kill();
        let _ = child.wait();
        return Err(err);
    }

    let mut guard = SIDECAR_CHILD
        .lock()
        .map_err(|_| "sidecar child lock poisoned".to_string())?;
    *guard = Some(child);
    Ok(())
}

fn restart_sidecar() -> Result<(), String> {
    shutdown_sidecar();
    spawn_sidecar()
}

fn sync_launch_agent(enabled: bool) -> Result<(), String> {
    if !cfg!(target_os = "macos") {
        return Ok(());
    }

    let home = std::env::var("HOME").map_err(|e| format!("resolve HOME failed: {e}"))?;
    let launch_agent_path = launch_agent_path(Path::new(&home));
    if enabled {
        if let Some(parent) = launch_agent_path.parent() {
            std::fs::create_dir_all(parent)
                .map_err(|e| format!("create LaunchAgents dir failed: {e}"))?;
        }
        let current_exe =
            std::env::current_exe().map_err(|e| format!("resolve current exe failed: {e}"))?;
        let plist = build_launch_agent_plist(&current_exe);
        std::fs::write(&launch_agent_path, plist)
            .map_err(|e| format!("write launch agent failed: {e}"))?;
        return Ok(());
    }

    if launch_agent_path.exists() {
        std::fs::remove_file(&launch_agent_path)
            .map_err(|e| format!("remove launch agent failed: {e}"))?;
    }
    Ok(())
}

fn launch_agent_path(home: &Path) -> PathBuf {
    home.join("Library")
        .join("LaunchAgents")
        .join(format!("{LAUNCH_AGENT_LABEL}.plist"))
}

fn build_launch_agent_plist(executable: &Path) -> String {
    let escaped_path = escape_xml(executable.to_string_lossy().as_ref());
    format!(
        r#"<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>{LAUNCH_AGENT_LABEL}</string>
  <key>ProgramArguments</key>
  <array>
    <string>{escaped_path}</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <false/>
</dict>
</plist>
"#
    )
}

fn escape_xml(value: &str) -> String {
    value
        .replace('&', "&amp;")
        .replace('<', "&lt;")
        .replace('>', "&gt;")
        .replace('"', "&quot;")
        .replace('\'', "&apos;")
}

fn setup_tray<R: Runtime>(app: &AppHandle<R>) -> Result<(), String> {
    let tray_state = build_tray_state().unwrap_or_default();
    let tray_menu = build_tray_menu(app, &tray_state)?;
    let tray_title = format_tray_title(
        tray_state.proxy.enabled,
        tray_state.active_account_name.as_deref(),
    );
    let mut builder = TrayIconBuilder::with_id(TRAY_ID)
        .menu(&tray_menu)
        .title(tray_title)
        .tooltip("AI Gate")
        .show_menu_on_left_click(true)
        .on_menu_event(|app, event| {
            let id = event.id().as_ref().to_string();
            handle_tray_menu_action(app, &id);
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

fn build_tray_menu<R: Runtime>(
    app: &AppHandle<R>,
    tray_state: &TrayStateSnapshot,
) -> Result<Menu<R>, String> {
    let proxy_status_text = if tray_state.proxy.enabled {
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

    if tray_state.accounts.is_empty() {
        builder = builder.text("accounts-empty", "Codex（无账户，请在主界面添加）");
    } else {
        for account in &tray_state.accounts {
            let id = format!("{MENU_ACCOUNT_PREFIX}{}", account.id);
            let label = if account.is_active {
                format!("✓ {}", account.name)
            } else {
                account.name.clone()
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

fn refresh_tray_state_from_backend<R: Runtime>(app: &AppHandle<R>) {
    let Ok(tray_state) = build_tray_state() else {
        return;
    };
    apply_tray_state(app, &tray_state);
}

fn apply_tray_state<R: Runtime>(app: &AppHandle<R>, tray_state: &TrayStateSnapshot) {
    let Some(tray) = app.tray_by_id(TRAY_ID) else {
        return;
    };
    let Ok(menu) = build_tray_menu(app, tray_state) else {
        return;
    };
    let _ = tray.set_menu(Some(menu));
    let _ = tray.set_title(Some(format_tray_title(
        tray_state.proxy.enabled,
        tray_state.active_account_name.as_deref(),
    )));
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
            let _ = request_backend(
                "POST",
                "/ai-router/api/settings/proxy/disable?skip_restore=1",
                "",
            );
        }
        MENU_QUIT => {
            shutdown_sidecar();
            app.exit(0);
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

    if should_refresh_tray_after_action(id) {
        refresh_tray_state_from_backend(app);
        emit_backend_state_changed(app);
    }
}

fn should_refresh_tray_after_action(id: &str) -> bool {
    id != MENU_OPEN_MAIN && id != MENU_QUIT
}

fn show_main_window<R: Runtime>(app: &AppHandle<R>) {
    if let Some(window) = app.get_webview_window("main") {
        let _ = window.show();
        let _ = window.unminimize();
        let _ = window.set_focus();
    }
}

fn parse_proxy_status_response(resp: &HttpResponse) -> Result<ProxyStatusSnapshot, String> {
    if resp.status != 200 {
        return Err(format!("unexpected proxy status code {}", resp.status));
    }
    let value =
        serde_json::from_str::<Value>(&resp.body).map_err(|e| format!("parse proxy status: {e}"))?;
    let enabled = value
        .get("enabled")
        .and_then(|v| v.as_bool())
        .ok_or_else(|| "proxy status payload missing enabled field".to_string())?;
    Ok(ProxyStatusSnapshot { enabled })
}

fn fetch_proxy_status() -> Result<ProxyStatusSnapshot, String> {
    let resp = request_backend("GET", "/ai-router/api/settings/proxy/status", "")?;
    parse_proxy_status_response(&resp)
}

fn parse_accounts_response(resp: &HttpResponse) -> Result<Vec<AccountSummary>, String> {
    if resp.status != 200 {
        return Err(format!("unexpected accounts status code {}", resp.status));
    }
    let value =
        serde_json::from_str::<Value>(&resp.body).map_err(|e| format!("parse accounts: {e}"))?;
    let items = value
        .as_array()
        .ok_or_else(|| "accounts payload must be an array".to_string())?;

    Ok(items
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
        .collect())
}

fn fetch_accounts() -> Result<Vec<AccountSummary>, String> {
    let resp = request_backend("GET", "/ai-router/api/accounts", "")?;
    parse_accounts_response(&resp)
}

fn build_tray_state() -> Result<TrayStateSnapshot, String> {
    let proxy = fetch_proxy_status()?;
    let accounts = fetch_accounts()?;
    let active_account_name = accounts
        .iter()
        .find(|account| account.is_active)
        .map(|account| account.name.clone());
    Ok(TrayStateSnapshot {
        proxy,
        accounts,
        active_account_name,
    })
}

fn format_tray_title(proxy_enabled: bool, active_account_name: Option<&str>) -> String {
    let indicator = if proxy_enabled { "§" } else { "·" };
    let account_name = active_account_name
        .map(str::trim)
        .filter(|name| !name.is_empty())
        .unwrap_or("无账户");
    format!("{indicator} {account_name}")
}

fn emit_backend_state_changed<R: Runtime>(app: &AppHandle<R>) {
    let _ = app.emit_to("main", BACKEND_STATE_CHANGED_EVENT, ());
}

fn request_backend(method: &str, path: &str, body: &str) -> Result<HttpResponse, String> {
    request_backend_with_timeout(
        &current_backend_addr(),
        method,
        path,
        body,
        Duration::from_millis(BACKEND_REQUEST_TIMEOUT_MS),
    )
}

fn request_backend_with_timeout(
    backend_addr: &str,
    method: &str,
    path: &str,
    body: &str,
    timeout: Duration,
) -> Result<HttpResponse, String> {
    let mut addrs = backend_addr
        .to_socket_addrs()
        .map_err(|e| format!("resolve backend address failed: {e}"))?;
    let socket_addr = addrs
        .next()
        .ok_or_else(|| format!("resolve backend address failed: {backend_addr}"))?;
    let mut stream = TcpStream::connect_timeout(&socket_addr, timeout)
        .map_err(|e| map_backend_io_error("connect", e))?;
    stream
        .set_read_timeout(Some(timeout))
        .map_err(|e| format!("configure backend read timeout failed: {e}"))?;
    stream
        .set_write_timeout(Some(timeout))
        .map_err(|e| format!("configure backend write timeout failed: {e}"))?;
    let request = format!(
        "{method} {path} HTTP/1.1\r\nHost: {backend_addr}\r\nConnection: close\r\nContent-Type: application/json\r\nContent-Length: {}\r\n\r\n{body}",
        body.len()
    );
    stream
        .write_all(request.as_bytes())
        .map_err(|e| map_backend_io_error("write", e))?;

    let mut response = String::new();
    stream
        .read_to_string(&mut response)
        .map_err(|e| map_backend_io_error("read", e))?;

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

fn map_backend_io_error(stage: &str, error: std::io::Error) -> String {
    if matches!(
        error.kind(),
        std::io::ErrorKind::TimedOut | std::io::ErrorKind::WouldBlock
    ) {
        format_timeout_error(stage)
    } else {
        format!("{stage} backend failed: {error}")
    }
}

fn format_timeout_error(stage: &str) -> String {
    format!("{stage} backend timed out while waiting for the sidecar")
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

    if let Ok(mut heartbeat_guard) = SIDECAR_HEARTBEAT.lock() {
        if let Some(handle) = heartbeat_guard.take() {
            let _ = handle.join();
        }
    }
}

fn start_sidecar_heartbeat(mut stdin: ChildStdin) -> Result<(), String> {
    let handle = std::thread::Builder::new()
        .name("sidecar-heartbeat".to_string())
        .spawn(move || loop {
            if stdin.write_all(b"hb\n").is_err() {
                break;
            }
            if stdin.flush().is_err() {
                break;
            }
            std::thread::sleep(SIDECAR_HEARTBEAT_INTERVAL);
        })
        .map_err(|e| format!("start sidecar heartbeat failed: {e}"))?;

    let mut guard = SIDECAR_HEARTBEAT
        .lock()
        .map_err(|_| "sidecar heartbeat lock poisoned".to_string())?;
    *guard = Some(handle);
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::{
        build_launch_agent_plist, format_timeout_error, format_tray_title, map_backend_io_error,
        parse_account_menu_id, parse_accounts_response, parse_proxy_status_response,
        should_refresh_tray_after_action, window_close_action, AppSettingsPayload,
        DesktopSettingsCache, HttpResponse, WindowCloseAction,
    };
    use std::path::Path;

    #[test]
    fn parse_account_menu_id_accepts_valid_ids() {
        assert_eq!(parse_account_menu_id("account-select:7"), Some(7));
    }

    #[test]
    fn parse_account_menu_id_rejects_invalid_ids() {
        assert_eq!(parse_account_menu_id("account-select:abc"), None);
        assert_eq!(parse_account_menu_id("proxy-enable"), None);
    }

    #[test]
    fn tray_refresh_skips_open_main_action() {
        assert!(!should_refresh_tray_after_action("open-main"));
    }

    #[test]
    fn tray_refresh_runs_for_stateful_actions() {
        assert!(should_refresh_tray_after_action("proxy-enable"));
        assert!(should_refresh_tray_after_action("proxy-disable"));
        assert!(should_refresh_tray_after_action("account-select:7"));
    }

    #[test]
    fn tray_refresh_skips_non_stateful_actions() {
        assert!(!should_refresh_tray_after_action("open-main"));
        assert!(!should_refresh_tray_after_action("quit"));
    }

    #[test]
    fn window_close_minimizes_when_close_to_tray_is_enabled() {
        assert_eq!(window_close_action(true), WindowCloseAction::MinimizeWindow);
    }

    #[test]
    fn window_close_exits_when_close_to_tray_is_disabled() {
        assert_eq!(window_close_action(false), WindowCloseAction::ExitApp);
    }

    #[test]
    fn tray_title_formats_proxy_enabled_with_account() {
        assert_eq!(format_tray_title(true, Some("team")), "§ team");
    }

    #[test]
    fn tray_title_formats_proxy_disabled_with_account() {
        assert_eq!(format_tray_title(false, Some("team")), "· team");
    }

    #[test]
    fn tray_title_formats_no_account() {
        assert_eq!(format_tray_title(false, None), "· 无账户");
        assert_eq!(format_tray_title(true, None), "§ 无账户");
    }

    #[test]
    fn desktop_settings_cache_defaults_match_app_defaults() {
        let cache = DesktopSettingsCache::default();

        assert!(!cache.launch_at_login);
        assert!(!cache.silent_start);
        assert!(cache.close_to_tray);
        assert_eq!(cache.proxy_host, "127.0.0.1");
        assert_eq!(cache.proxy_port, 6789);
        assert_eq!(cache.backend_addr(), "127.0.0.1:6789");
        assert_eq!(
            cache.backend_api_base(),
            "http://127.0.0.1:6789/ai-router/api"
        );
    }

    #[test]
    fn desktop_settings_cache_tracks_runtime_fields_from_app_settings() {
        let payload = AppSettingsPayload {
            launch_at_login: true,
            silent_start: true,
            close_to_tray: false,
            show_proxy_switch_on_home: false,
            proxy_host: "0.0.0.0".to_string(),
            proxy_port: 18080,
            auto_failover_enabled: true,
            auto_backup_interval_hours: 12,
            backup_retention_count: 7,
        };

        let cache = DesktopSettingsCache::from_app_settings(payload);
        assert!(cache.launch_at_login);
        assert!(cache.silent_start);
        assert!(!cache.close_to_tray);
        assert_eq!(cache.proxy_host, "0.0.0.0");
        assert_eq!(cache.proxy_port, 18080);
        assert_eq!(cache.backend_addr(), "0.0.0.0:18080");
    }

    #[test]
    fn desktop_settings_cache_sanitizes_invalid_proxy_endpoint() {
        let payload = AppSettingsPayload {
            launch_at_login: false,
            silent_start: false,
            close_to_tray: true,
            show_proxy_switch_on_home: true,
            proxy_host: "   ".to_string(),
            proxy_port: 0,
            auto_failover_enabled: false,
            auto_backup_interval_hours: 24,
            backup_retention_count: 10,
        };

        let cache = DesktopSettingsCache::from_app_settings(payload);
        assert_eq!(cache.proxy_host, "127.0.0.1");
        assert_eq!(cache.proxy_port, 6789);
    }

    #[test]
    fn launch_agent_plist_uses_current_executable() {
        let plist = build_launch_agent_plist(Path::new(
            "/Applications/AI Gate.app/Contents/MacOS/aigate-desktop",
        ));

        assert!(plist.contains("<string>com.aigate.desktop</string>"));
        assert!(plist
            .contains("<string>/Applications/AI Gate.app/Contents/MacOS/aigate-desktop</string>"));
        assert!(plist.contains("<key>RunAtLoad</key>"));
    }

    #[test]
    fn timeout_errors_are_formatted_for_humans() {
        assert_eq!(
            format_timeout_error("read"),
            "read backend timed out while waiting for the sidecar"
        );
    }

    #[test]
    fn timeout_io_errors_are_mapped_consistently() {
        let error = std::io::Error::new(std::io::ErrorKind::TimedOut, "test timeout");
        assert_eq!(
            map_backend_io_error("read", error),
            "read backend timed out while waiting for the sidecar"
        );
    }

    #[test]
    fn parse_proxy_status_response_rejects_invalid_json() {
        let response = HttpResponse {
            status: 200,
            body: "{\"enabled\":".to_string(),
        };
        assert!(parse_proxy_status_response(&response).is_err());
    }

    #[test]
    fn parse_accounts_response_rejects_non_array_payload() {
        let response = HttpResponse {
            status: 200,
            body: "{\"id\":1}".to_string(),
        };
        assert!(parse_accounts_response(&response).is_err());
    }
}
