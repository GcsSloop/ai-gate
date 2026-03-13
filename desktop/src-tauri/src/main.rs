#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use once_cell::sync::Lazy;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::VecDeque;
use std::io::{Read, Write};
use std::net::{TcpStream, ToSocketAddrs};
use std::path::{Path, PathBuf};
use std::process::{Child, ChildStdin, Command, Stdio};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Mutex;
use std::thread::JoinHandle;
use std::time::{Duration, Instant};
use tauri::image::Image;
use tauri::menu::{Menu, MenuBuilder, MenuItemBuilder};
use tauri::tray::TrayIconBuilder;
use tauri::{AppHandle, Emitter, Manager, Runtime};

#[cfg(windows)]
use std::os::windows::process::CommandExt;

static SIDECAR_CHILD: Lazy<Mutex<Option<Child>>> = Lazy::new(|| Mutex::new(None));
static SIDECAR_HEARTBEAT: Lazy<Mutex<Option<JoinHandle<()>>>> = Lazy::new(|| Mutex::new(None));
static RESUME_RECOVERY_WATCHER: Lazy<Mutex<Option<JoinHandle<()>>>> =
    Lazy::new(|| Mutex::new(None));
static RESUME_RECOVERY_WATCHER_STOP: AtomicBool = AtomicBool::new(false);
static DESKTOP_RUNTIME: Lazy<Mutex<DesktopRuntime>> =
    Lazy::new(|| Mutex::new(DesktopRuntime::default()));
static DESKTOP_RECENT_LOGS: Lazy<Mutex<VecDeque<DesktopLogEntry>>> =
    Lazy::new(|| Mutex::new(VecDeque::new()));

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
const SIDECAR_READY_WAIT_TIMEOUT_MS: u64 = 5000;
const SIDECAR_READY_POLL_INTERVAL_MS: u64 = 100;
const SIDECAR_HEARTBEAT_INTERVAL: Duration = Duration::from_secs(1);
const RESUME_RECOVERY_WATCH_INTERVAL: Duration = Duration::from_secs(5);
const RESUME_RECOVERY_GAP_THRESHOLD: Duration = Duration::from_secs(15);
const DESKTOP_RECENT_LOG_CAPACITY: usize = 200;
const DESKTOP_RECENT_LOG_DEFAULT_LIMIT: usize = 50;
const DESKTOP_RECENT_LOG_MAX_LIMIT: usize = 50;
const SIDECAR_MACOS_NAME: &str = "routerd-universal-apple-darwin";
const SIDECAR_WINDOWS_NAME: &str = "routerd-x86_64-pc-windows-msvc.exe";
const TRAY_ICON_COLOR_BYTES: &[u8] = include_bytes!("../icons/tray-icon-color.png");
const TRAY_ICON_TEMPLATE_BYTES: &[u8] = include_bytes!("../icons/tray-icon-template.png");
#[cfg(windows)]
const CREATE_NO_WINDOW: u32 = 0x0800_0000;

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

#[derive(Clone, Debug, Serialize, PartialEq, Eq)]
struct DesktopLogEntry {
    timestamp_ms: u64,
    level: String,
    category: String,
    message: String,
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
        .plugin(tauri_plugin_process::init())
        .plugin(tauri_plugin_updater::Builder::new().build())
        .invoke_handler(tauri::generate_handler![
            force_exit_app,
            refresh_tray_state,
            get_desktop_shell_context,
            apply_app_settings,
            get_app_metadata,
            get_recent_desktop_logs
        ])
        .setup(|app| {
            let cache = initialize_runtime(app.handle())?;
            if let Err(err) = sync_launch_agent(cache.launch_at_login) {
                eprintln!("sync launch agent failed: {err}");
            }
            spawn_sidecar()?;
            wait_for_backend_ready(
                &cache.backend_addr(),
                Duration::from_millis(SIDECAR_READY_WAIT_TIMEOUT_MS),
            )?;
            start_resume_recovery_watcher(app.handle().clone())?;
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
                                stop_resume_recovery_watcher();
                                shutdown_sidecar();
                                app_handle.exit(0);
                            }
                        }
                    }
                }
            }
            #[cfg(target_os = "macos")]
            tauri::RunEvent::Reopen { .. } => {
                recover_backend_after_reopen(&app_handle);
                show_main_window(app_handle);
            }
            tauri::RunEvent::Exit => {
                stop_resume_recovery_watcher();
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
    stop_resume_recovery_watcher();
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
        restart_sidecar_and_wait_ready()?;
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

#[tauri::command]
fn get_recent_desktop_logs(limit: Option<usize>) -> Vec<DesktopLogEntry> {
    let count = clamp_recent_log_limit(limit);
    DESKTOP_RECENT_LOGS
        .lock()
        .map(|entries| entries.iter().rev().take(count).cloned().collect())
        .unwrap_or_default()
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

fn clamp_recent_log_limit(limit: Option<usize>) -> usize {
    match limit.unwrap_or(DESKTOP_RECENT_LOG_DEFAULT_LIMIT) {
        0 => 1,
        value if value > DESKTOP_RECENT_LOG_MAX_LIMIT => DESKTOP_RECENT_LOG_MAX_LIMIT,
        value => value,
    }
}

fn append_recent_desktop_log(
    entries: &mut VecDeque<DesktopLogEntry>,
    entry: DesktopLogEntry,
    capacity: usize,
) {
    entries.push_back(entry);
    while entries.len() > capacity {
        entries.pop_front();
    }
}

fn log_desktop_event(level: &str, category: &str, message: impl Into<String>) {
    let message = message.into();
    eprintln!("desktop-{category} [{level}] {message}");
    let timestamp_ms = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|duration| duration.as_millis() as u64)
        .unwrap_or_default();
    let entry = DesktopLogEntry {
        timestamp_ms,
        level: level.to_string(),
        category: category.to_string(),
        message,
    };
    if let Ok(mut entries) = DESKTOP_RECENT_LOGS.lock() {
        append_recent_desktop_log(&mut entries, entry, DESKTOP_RECENT_LOG_CAPACITY);
    }
}

#[cfg(windows)]
fn sidecar_creation_flags() -> u32 {
    CREATE_NO_WINDOW
}

#[cfg(not(windows))]
#[allow(dead_code)]
fn sidecar_creation_flags() -> u32 {
    0
}

#[cfg(windows)]
fn configure_sidecar_command(command: &mut Command) {
    command.creation_flags(sidecar_creation_flags());
}

#[cfg(not(windows))]
fn configure_sidecar_command(_command: &mut Command) {}

fn spawn_sidecar() -> Result<(), String> {
    let runtime = DESKTOP_RUNTIME
        .lock()
        .map_err(|_| "desktop runtime lock poisoned".to_string())?
        .clone();

    log_desktop_event(
        "info",
        "sidecar",
        format!(
            "spawn requested path={} addr={}",
            runtime.sidecar_path.display(),
            runtime.settings_cache.backend_addr()
        ),
    );

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
    configure_sidecar_command(&mut command);

    let mut child = command.spawn().map_err(|e| {
        let message = format!(
            "spawn sidecar {} failed: {e}",
            runtime.sidecar_path.display()
        );
        log_desktop_event("error", "sidecar", &message);
        message
    })?;
    let stdin = child
        .stdin
        .take()
        .ok_or_else(|| "capture sidecar stdin failed".to_string())?;
    if let Err(err) = start_sidecar_heartbeat(stdin) {
        let _ = child.kill();
        let _ = child.wait();
        log_desktop_event("error", "sidecar", format!("start heartbeat failed: {err}"));
        return Err(err);
    }

    let mut guard = SIDECAR_CHILD
        .lock()
        .map_err(|_| "sidecar child lock poisoned".to_string())?;
    *guard = Some(child);
    log_desktop_event("info", "sidecar", "spawn success");
    Ok(())
}

fn restart_sidecar() -> Result<(), String> {
    log_desktop_event("warn", "recovery", "restart requested");
    shutdown_sidecar_with_reason("restart");
    let result = spawn_sidecar();
    match &result {
        Ok(_) => log_desktop_event("info", "recovery", "restart success"),
        Err(err) => log_desktop_event("error", "recovery", format!("restart failed: {err}")),
    }
    result
}

fn restart_sidecar_and_wait_ready() -> Result<(), String> {
    restart_sidecar()?;
    wait_for_backend_ready(
        &current_backend_addr(),
        Duration::from_millis(SIDECAR_READY_WAIT_TIMEOUT_MS),
    )
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

fn current_tray_platform() -> &'static str {
    if cfg!(target_os = "macos") {
        "macos"
    } else if cfg!(target_os = "windows") {
        "windows"
    } else {
        "other"
    }
}

fn tray_icon_bytes_for_platform(target_os: &str) -> &'static [u8] {
    if target_os == "macos" {
        TRAY_ICON_TEMPLATE_BYTES
    } else {
        TRAY_ICON_COLOR_BYTES
    }
}

fn tray_icon_is_template_for_platform(target_os: &str) -> bool {
    target_os == "macos"
}

fn setup_tray<R: Runtime>(app: &AppHandle<R>) -> Result<(), String> {
    let tray_state = build_tray_state().unwrap_or_default();
    let tray_menu = build_tray_menu(app, &tray_state)?;
    let tray_title = format_tray_title(
        tray_state.proxy.enabled,
        tray_state.active_account_name.as_deref(),
    );
    let tray_platform = current_tray_platform();
    let mut builder = TrayIconBuilder::with_id(TRAY_ID)
        .menu(&tray_menu)
        .title(tray_title)
        .tooltip("AI Gate")
        .show_menu_on_left_click(true)
        .on_menu_event(|app, event| {
            let id = event.id().as_ref().to_string();
            handle_tray_menu_action(app, &id);
        });

    if let Ok(icon) = Image::from_bytes(tray_icon_bytes_for_platform(tray_platform)) {
        builder = builder.icon(icon);
    } else if let Some(icon) = app.default_window_icon().cloned() {
        builder = builder.icon(icon);
    }
    if tray_icon_is_template_for_platform(tray_platform) {
        builder = builder.icon_as_template(true);
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
    let (enable_proxy_enabled, disable_proxy_enabled) =
        proxy_menu_enabled_states(tray_state.proxy.enabled);
    let enable_proxy_item = MenuItemBuilder::with_id(MENU_PROXY_ENABLE, "开启代理")
        .enabled(enable_proxy_enabled)
        .build(app)
        .map_err(|e| format!("build tray proxy enable item failed: {e}"))?;
    let disable_proxy_item = MenuItemBuilder::with_id(MENU_PROXY_DISABLE, "关闭代理")
        .enabled(disable_proxy_enabled)
        .build(app)
        .map_err(|e| format!("build tray proxy disable item failed: {e}"))?;

    let mut builder = MenuBuilder::new(app)
        .text(MENU_OPEN_MAIN, "打开主界面")
        .separator()
        .text(MENU_PROXY_STATUS, proxy_status_text)
        .item(&enable_proxy_item)
        .item(&disable_proxy_item)
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

#[cfg(target_os = "macos")]
fn recover_backend_after_reopen<R: Runtime>(app: &AppHandle<R>) {
    recover_backend_after_resume(app, "reopen", None);
}

#[cfg(target_os = "macos")]
fn recover_backend_after_resume<R: Runtime>(
    app: &AppHandle<R>,
    trigger: &str,
    gap: Option<Duration>,
) {
    let gap_suffix = gap
        .map(|value| format!(" gap_ms={}", value.as_millis()))
        .unwrap_or_default();
    log_desktop_event(
        "warn",
        "recovery",
        format!("backend probe trigger={trigger}{gap_suffix}"),
    );
    match request_backend("GET", "/ai-router/api/settings/proxy/status", "") {
        Ok(_) => log_desktop_event(
            "info",
            "recovery",
            format!("backend probe recovered trigger={trigger}{gap_suffix}"),
        ),
        Err(err) => log_desktop_event(
            "error",
            "recovery",
            format!("backend probe failed trigger={trigger}{gap_suffix}: {err}"),
        ),
    }
    refresh_tray_state_from_backend(app);
    emit_backend_state_changed(app);
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
            stop_resume_recovery_watcher();
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

fn proxy_menu_enabled_states(proxy_enabled: bool) -> (bool, bool) {
    if proxy_enabled {
        (false, true)
    } else {
        (true, false)
    }
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
    let value = serde_json::from_str::<Value>(&resp.body)
        .map_err(|e| format!("parse proxy status: {e}"))?;
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

fn should_attempt_sidecar_recovery(error: &str) -> bool {
    let normalized = error.to_ascii_lowercase();
    normalized.contains("timed out while waiting for the sidecar")
        || normalized.contains("connection refused")
        || normalized.contains("broken pipe")
        || normalized.contains("not connected")
}

fn should_retry_sidecar_request(restart_worthy: bool, attempted_recovery: bool) -> bool {
    restart_worthy && !attempted_recovery
}

fn sidecar_request_with_recovery<FRequest, FRestart>(
    mut request: FRequest,
    mut restart: FRestart,
) -> Result<HttpResponse, String>
where
    FRequest: FnMut() -> Result<HttpResponse, String>,
    FRestart: FnMut() -> Result<(), String>,
{
    sidecar_request_with_recovery_hooks(&mut request, &mut restart, || Ok(()))
}

fn sidecar_request_with_recovery_hooks<FRequest, FRestart, FWait>(
    mut request: FRequest,
    mut restart: FRestart,
    mut wait_until_ready: FWait,
) -> Result<HttpResponse, String>
where
    FRequest: FnMut() -> Result<HttpResponse, String>,
    FRestart: FnMut() -> Result<(), String>,
    FWait: FnMut() -> Result<(), String>,
{
    let mut attempted_recovery = false;

    loop {
        match request() {
            Ok(response) => return Ok(response),
            Err(error) => {
                let restart_worthy = should_attempt_sidecar_recovery(&error);
                if !should_retry_sidecar_request(restart_worthy, attempted_recovery) {
                    if restart_worthy {
                        log_desktop_event(
                            "error",
                            "recovery",
                            format!("request failed after retry: {error}"),
                        );
                    }
                    return Err(error);
                }
                log_desktop_event(
                    "warn",
                    "recovery",
                    format!("request failed, attempting sidecar restart: {error}"),
                );
                restart()?;
                wait_until_ready()?;
                attempted_recovery = true;
                log_desktop_event("info", "recovery", "retrying backend request after restart");
            }
        }
    }
}

fn request_backend(method: &str, path: &str, body: &str) -> Result<HttpResponse, String> {
    let backend_addr = current_backend_addr();
    sidecar_request_with_recovery_hooks(
        || {
            request_backend_with_timeout(
                &backend_addr,
                method,
                path,
                body,
                Duration::from_millis(BACKEND_REQUEST_TIMEOUT_MS),
            )
        },
        restart_sidecar,
        || {
            wait_for_backend_ready(
                &backend_addr,
                Duration::from_millis(SIDECAR_READY_WAIT_TIMEOUT_MS),
            )
        },
    )
}

fn wait_for_backend_ready(backend_addr: &str, timeout: Duration) -> Result<(), String> {
    let addr = backend_addr.to_string();
    log_desktop_event(
        "info",
        "recovery",
        format!(
            "waiting for backend readiness addr={} timeout_ms={}",
            addr,
            timeout.as_millis()
        ),
    );
    let result = wait_for_backend_ready_with_probe(
        || probe_backend_ready(&addr),
        timeout,
        Duration::from_millis(SIDECAR_READY_POLL_INTERVAL_MS),
        std::thread::sleep,
    );
    match &result {
        Ok(_) => log_desktop_event("info", "recovery", format!("backend ready addr={addr}")),
        Err(err) => log_desktop_event(
            "error",
            "recovery",
            format!("backend readiness wait failed addr={addr}: {err}"),
        ),
    }
    result
}

fn wait_for_backend_ready_with_probe<FProbe, FSleep>(
    mut probe: FProbe,
    timeout: Duration,
    poll_interval: Duration,
    mut sleep_fn: FSleep,
) -> Result<(), String>
where
    FProbe: FnMut() -> Result<(), String>,
    FSleep: FnMut(Duration),
{
    let started_at = Instant::now();

    loop {
        match probe() {
            Ok(()) => return Ok(()),
            Err(err) => {
                let elapsed = started_at.elapsed();
                if elapsed >= timeout {
                    return Err(format!("timed out after {} ms: {err}", timeout.as_millis()));
                }

                let remaining = timeout.saturating_sub(elapsed);
                sleep_fn(remaining.min(poll_interval));
            }
        }
    }
}

fn probe_backend_ready(backend_addr: &str) -> Result<(), String> {
    let mut addrs = backend_addr
        .to_socket_addrs()
        .map_err(|e| format!("resolve backend address failed: {e}"))?;
    let socket_addr = addrs
        .next()
        .ok_or_else(|| format!("resolve backend address failed: {backend_addr}"))?;
    TcpStream::connect_timeout(
        &socket_addr,
        Duration::from_millis(SIDECAR_READY_POLL_INTERVAL_MS),
    )
    .map(|_| ())
    .map_err(|e| format!("connect backend failed: {e}"))
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
    let (headers, raw_body) = response
        .split_once("\r\n\r\n")
        .map(|(head, body)| (head, body.to_string()))
        .unwrap_or(("", String::new()));
    let is_chunked = headers.lines().any(|line| {
        let mut parts = line.splitn(2, ':');
        let Some(name) = parts.next() else {
            return false;
        };
        let Some(value) = parts.next() else {
            return false;
        };
        name.eq_ignore_ascii_case("Transfer-Encoding")
            && value.to_ascii_lowercase().contains("chunked")
    });
    let body = if is_chunked {
        decode_chunked_body(&raw_body).unwrap_or(raw_body)
    } else {
        raw_body
    };

    Ok(HttpResponse { status, body })
}

fn decode_chunked_body(raw: &str) -> Result<String, String> {
    let mut remaining = raw;
    let mut output = String::new();

    loop {
        let Some((size_line, rest)) = remaining.split_once("\r\n") else {
            return Err("invalid chunked body: missing chunk size line".to_string());
        };
        let size_token = size_line
            .split(';')
            .next()
            .map(str::trim)
            .unwrap_or_default();
        let size = usize::from_str_radix(size_token, 16)
            .map_err(|_| "invalid chunked body: bad chunk size".to_string())?;
        if size == 0 {
            break;
        }
        if rest.len() < size + 2 {
            return Err("invalid chunked body: truncated chunk".to_string());
        }
        output.push_str(&rest[..size]);
        if &rest[size..size + 2] != "\r\n" {
            return Err("invalid chunked body: missing chunk terminator".to_string());
        }
        remaining = &rest[size + 2..];
    }

    Ok(output)
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

fn sidecar_resource_name(os: &str) -> Option<&'static str> {
    match os {
        "macos" => Some(SIDECAR_MACOS_NAME),
        "windows" => Some(SIDECAR_WINDOWS_NAME),
        _ => None,
    }
}

fn sidecar_candidate_paths(
    os: &str,
    manifest_dir: Option<&Path>,
    resources_dir: Option<&Path>,
    current_exe: Option<&Path>,
) -> Vec<PathBuf> {
    let Some(sidecar_name) = sidecar_resource_name(os) else {
        return Vec::new();
    };

    let mut candidates: Vec<PathBuf> = Vec::new();

    if let Some(manifest_dir) = manifest_dir {
        candidates.push(manifest_dir.join("bin").join(sidecar_name));
    }

    if let Some(resources_dir) = resources_dir {
        candidates.push(resources_dir.join("bin").join(sidecar_name));
        if os == "windows" {
            candidates.push(resources_dir.join(sidecar_name));
        }
    }

    if let Some(exe) = current_exe {
        if let Some(exe_dir) = exe.parent() {
            candidates.push(exe_dir.join("bin").join(sidecar_name));
            if os == "macos" {
                candidates.push(exe_dir.join("../Resources/bin").join(sidecar_name));
            } else if os == "windows" {
                candidates.push(exe_dir.join("resources").join("bin").join(sidecar_name));
            }
        }
    }

    candidates
}

fn resolve_sidecar_path(app: &tauri::AppHandle) -> Result<PathBuf, String> {
    let os = std::env::consts::OS;
    let sidecar_name = sidecar_resource_name(os)
        .ok_or_else(|| format!("routerd sidecar is not configured for platform {os}"))?;
    let manifest_dir = if cfg!(debug_assertions) {
        std::env::var("CARGO_MANIFEST_DIR").ok().map(PathBuf::from)
    } else {
        None
    };
    let resources_dir = app.path().resource_dir().ok();
    let current_exe = std::env::current_exe().ok();
    let candidates = sidecar_candidate_paths(
        os,
        manifest_dir.as_deref(),
        resources_dir.as_deref(),
        current_exe.as_deref(),
    );

    for candidate in candidates {
        if candidate.exists() {
            return Ok(candidate);
        }
    }

    Err(format!(
        "routerd sidecar not found, expected bin/{sidecar_name}"
    ))
}

fn shutdown_sidecar() {
    shutdown_sidecar_with_reason("shutdown");
}

fn shutdown_sidecar_with_reason(reason: &str) {
    let mut guard = match SIDECAR_CHILD.lock() {
        Ok(guard) => guard,
        Err(_) => return,
    };

    if let Some(child) = guard.as_mut() {
        log_desktop_event(
            "info",
            "sidecar",
            format!("shutdown requested reason={reason}"),
        );
        match child.kill() {
            Ok(()) => log_desktop_event("info", "sidecar", "kill signal sent"),
            Err(err) => log_desktop_event("warn", "sidecar", format!("kill failed: {err}")),
        }
        match child.wait() {
            Ok(status) => log_desktop_event(
                "info",
                "sidecar",
                format!("sidecar exited reason={reason} status={status}"),
            ),
            Err(err) => log_desktop_event("warn", "sidecar", format!("wait failed: {err}")),
        }
    }
    *guard = None;

    if let Ok(mut heartbeat_guard) = SIDECAR_HEARTBEAT.lock() {
        if let Some(handle) = heartbeat_guard.take() {
            let _ = handle.join();
        }
    }
}

fn should_trigger_resume_recovery(elapsed: Duration, threshold: Duration) -> bool {
    elapsed >= threshold
}

#[cfg(target_os = "macos")]
fn start_resume_recovery_watcher<R: Runtime + 'static>(app: AppHandle<R>) -> Result<(), String> {
    RESUME_RECOVERY_WATCHER_STOP.store(false, Ordering::SeqCst);
    let handle = std::thread::Builder::new()
        .name("resume-recovery".to_string())
        .spawn(move || {
            let mut last_tick = Instant::now();
            while !RESUME_RECOVERY_WATCHER_STOP.load(Ordering::SeqCst) {
                std::thread::sleep(RESUME_RECOVERY_WATCH_INTERVAL);
                let elapsed = last_tick.elapsed();
                last_tick = Instant::now();
                if should_trigger_resume_recovery(elapsed, RESUME_RECOVERY_GAP_THRESHOLD) {
                    recover_backend_after_resume(&app, "resume_gap", Some(elapsed));
                }
            }
        })
        .map_err(|e| format!("start resume recovery watcher failed: {e}"))?;
    let mut guard = RESUME_RECOVERY_WATCHER
        .lock()
        .map_err(|_| "resume recovery watcher lock poisoned".to_string())?;
    *guard = Some(handle);
    Ok(())
}

#[cfg(not(target_os = "macos"))]
fn start_resume_recovery_watcher<R: Runtime + 'static>(_app: AppHandle<R>) -> Result<(), String> {
    Ok(())
}

fn stop_resume_recovery_watcher() {
    RESUME_RECOVERY_WATCHER_STOP.store(true, Ordering::SeqCst);
    if let Ok(mut guard) = RESUME_RECOVERY_WATCHER.lock() {
        if let Some(handle) = guard.take() {
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
        append_recent_desktop_log, build_launch_agent_plist, clamp_recent_log_limit,
        decode_chunked_body, format_timeout_error, format_tray_title, map_backend_io_error,
        parse_account_menu_id, parse_accounts_response, parse_proxy_status_response,
        proxy_menu_enabled_states,
        should_attempt_sidecar_recovery, should_refresh_tray_after_action,
        should_retry_sidecar_request, should_trigger_resume_recovery, sidecar_candidate_paths,
        sidecar_creation_flags, sidecar_request_with_recovery, sidecar_request_with_recovery_hooks,
        sidecar_resource_name, tray_icon_bytes_for_platform, tray_icon_is_template_for_platform,
        wait_for_backend_ready_with_probe, window_close_action, AppSettingsPayload,
        DesktopLogEntry, DesktopSettingsCache, HttpResponse, WindowCloseAction, SIDECAR_MACOS_NAME,
        SIDECAR_WINDOWS_NAME, TRAY_ICON_COLOR_BYTES, TRAY_ICON_TEMPLATE_BYTES,
    };
    use std::cell::RefCell;
    use std::collections::VecDeque;
    use std::path::{Path, PathBuf};
    use std::time::Duration;

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
    fn proxy_menu_states_disable_enable_when_proxy_is_active() {
        assert_eq!(proxy_menu_enabled_states(true), (false, true));
    }

    #[test]
    fn proxy_menu_states_disable_disable_when_proxy_is_inactive() {
        assert_eq!(proxy_menu_enabled_states(false), (true, false));
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
    fn tray_icon_platform_selection_matches_expected_assets() {
        assert_eq!(
            tray_icon_bytes_for_platform("macos"),
            TRAY_ICON_TEMPLATE_BYTES
        );
        assert_eq!(
            tray_icon_bytes_for_platform("windows"),
            TRAY_ICON_COLOR_BYTES
        );
        assert_eq!(tray_icon_bytes_for_platform("linux"), TRAY_ICON_COLOR_BYTES);
    }

    #[test]
    fn tray_icon_template_mode_only_applies_to_macos() {
        assert!(tray_icon_is_template_for_platform("macos"));
        assert!(!tray_icon_is_template_for_platform("windows"));
        assert!(!tray_icon_is_template_for_platform("linux"));
    }

    #[test]
    fn sidecar_resource_name_matches_supported_platforms() {
        assert_eq!(sidecar_resource_name("macos"), Some(SIDECAR_MACOS_NAME));
        assert_eq!(sidecar_resource_name("windows"), Some(SIDECAR_WINDOWS_NAME));
        assert_eq!(sidecar_resource_name("linux"), None);
    }

    #[test]
    fn sidecar_candidates_include_macos_debug_and_bundle_locations() {
        let manifest_dir = Path::new("/repo/desktop/src-tauri");
        let resources_dir = Path::new("/Applications/AI Gate.app/Contents/Resources");
        let current_exe = Path::new("/Applications/AI Gate.app/Contents/MacOS/aigate-desktop");

        let candidates = sidecar_candidate_paths(
            "macos",
            Some(manifest_dir),
            Some(resources_dir),
            Some(current_exe),
        );

        assert!(candidates.contains(&manifest_dir.join("bin").join(SIDECAR_MACOS_NAME)));
        assert!(candidates.contains(&resources_dir.join("bin").join(SIDECAR_MACOS_NAME)));
        assert!(candidates.contains(&PathBuf::from(
            "/Applications/AI Gate.app/Contents/MacOS/../Resources/bin/routerd-universal-apple-darwin"
        )));
    }

    #[test]
    fn sidecar_candidates_include_windows_resource_and_portable_locations() {
        let manifest_dir = Path::new("C:/repo/desktop/src-tauri");
        let resources_dir = Path::new("C:/Program Files/AI Gate/resources");
        let current_exe = Path::new("C:/Program Files/AI Gate/aigate-desktop.exe");

        let candidates = sidecar_candidate_paths(
            "windows",
            Some(manifest_dir),
            Some(resources_dir),
            Some(current_exe),
        );

        assert!(candidates.contains(&manifest_dir.join("bin").join(SIDECAR_WINDOWS_NAME)));
        assert!(candidates.contains(&resources_dir.join("bin").join(SIDECAR_WINDOWS_NAME)));
        assert!(candidates.contains(&resources_dir.join(SIDECAR_WINDOWS_NAME)));
        assert!(candidates.contains(&PathBuf::from(
            "C:/Program Files/AI Gate/bin/routerd-x86_64-pc-windows-msvc.exe"
        )));
        assert!(candidates.contains(&PathBuf::from(
            "C:/Program Files/AI Gate/resources/bin/routerd-x86_64-pc-windows-msvc.exe"
        )));
    }

    #[cfg(not(windows))]
    #[test]
    fn hidden_sidecar_flags_are_zero_on_non_windows() {
        assert_eq!(sidecar_creation_flags(), 0);
    }

    #[cfg(windows)]
    #[test]
    fn hidden_sidecar_flags_include_create_no_window() {
        assert_eq!(sidecar_creation_flags(), 0x0800_0000);
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
    fn sidecar_recovery_detects_timeout_errors() {
        assert!(should_attempt_sidecar_recovery(
            "read backend timed out while waiting for the sidecar"
        ));
    }

    #[test]
    fn sidecar_recovery_detects_connection_refused_errors() {
        assert!(should_attempt_sidecar_recovery(
            "connect backend failed: Connection refused (os error 61)"
        ));
    }

    #[test]
    fn sidecar_recovery_ignores_unrelated_errors() {
        assert!(!should_attempt_sidecar_recovery(
            "resolve backend address failed: invalid socket address"
        ));
    }

    #[test]
    fn sidecar_recovery_retries_only_once() {
        assert!(should_retry_sidecar_request(true, false));
        assert!(!should_retry_sidecar_request(true, true));
        assert!(!should_retry_sidecar_request(false, false));
    }

    #[test]
    fn sidecar_request_restarts_then_retries_once() {
        let mut request_calls = 0;
        let mut restart_calls = 0;

        let result = sidecar_request_with_recovery(
            || {
                request_calls += 1;
                if request_calls == 1 {
                    Err("connect backend failed: Connection refused (os error 61)".to_string())
                } else {
                    Ok(HttpResponse {
                        status: 200,
                        body: "{}".to_string(),
                    })
                }
            },
            || {
                restart_calls += 1;
                Ok(())
            },
        )
        .expect("request should recover");

        assert_eq!(result.status, 200);
        assert_eq!(request_calls, 2);
        assert_eq!(restart_calls, 1);
    }

    #[test]
    fn sidecar_request_returns_original_error_after_single_retry() {
        let mut request_calls = 0;
        let mut restart_calls = 0;

        let result = sidecar_request_with_recovery(
            || {
                request_calls += 1;
                Err("read backend timed out while waiting for the sidecar".to_string())
            },
            || {
                restart_calls += 1;
                Ok(())
            },
        );

        let err = match result {
            Ok(_) => panic!("request should fail after one retry"),
            Err(err) => err,
        };

        assert_eq!(err, "read backend timed out while waiting for the sidecar");
        assert_eq!(request_calls, 2);
        assert_eq!(restart_calls, 1);
    }

    #[test]
    fn sidecar_request_waits_for_backend_ready_before_retry() {
        let mut request_calls = 0;
        let mut restart_calls = 0;
        let mut wait_calls = 0;
        let events = RefCell::new(Vec::new());

        let result = sidecar_request_with_recovery_hooks(
            || {
                request_calls += 1;
                events.borrow_mut().push(format!("request-{request_calls}"));
                if request_calls == 1 {
                    Err("connect backend failed: Connection refused (os error 61)".to_string())
                } else {
                    Ok(HttpResponse {
                        status: 200,
                        body: "{}".to_string(),
                    })
                }
            },
            || {
                restart_calls += 1;
                events.borrow_mut().push("restart".to_string());
                Ok(())
            },
            || {
                wait_calls += 1;
                events.borrow_mut().push("wait-ready".to_string());
                Ok(())
            },
        )
        .expect("request should recover after waiting");

        assert_eq!(result.status, 200);
        assert_eq!(restart_calls, 1);
        assert_eq!(wait_calls, 1);
        assert_eq!(
            events.into_inner(),
            vec!["request-1", "restart", "wait-ready", "request-2"]
        );
    }

    #[test]
    fn backend_ready_wait_retries_until_probe_succeeds() {
        let mut probe_calls = 0;
        let mut sleeps = Vec::new();

        wait_for_backend_ready_with_probe(
            || {
                probe_calls += 1;
                if probe_calls < 3 {
                    Err("not ready".to_string())
                } else {
                    Ok(())
                }
            },
            Duration::from_millis(600),
            Duration::from_millis(100),
            |duration| sleeps.push(duration),
        )
        .expect("backend should become ready");

        assert_eq!(probe_calls, 3);
        assert_eq!(
            sleeps,
            vec![Duration::from_millis(100), Duration::from_millis(100)]
        );
    }

    #[test]
    fn backend_ready_wait_times_out_when_probe_never_succeeds() {
        let mut probe_calls = 0;

        let err = wait_for_backend_ready_with_probe(
            || {
                probe_calls += 1;
                Err("still booting".to_string())
            },
            Duration::from_millis(250),
            Duration::from_millis(100),
            |_| {},
        )
        .expect_err("backend readiness wait should time out");

        assert!(err.contains("still booting"));
        assert!(probe_calls >= 2);
    }

    #[test]
    fn resume_recovery_triggers_only_after_large_gap() {
        assert!(!should_trigger_resume_recovery(
            Duration::from_secs(2),
            Duration::from_secs(10)
        ));
        assert!(should_trigger_resume_recovery(
            Duration::from_secs(15),
            Duration::from_secs(10)
        ));
    }

    #[test]
    fn recent_desktop_logs_drop_oldest_entries_when_capacity_is_exceeded() {
        let mut entries = VecDeque::new();
        for index in 0..4 {
            append_recent_desktop_log(
                &mut entries,
                DesktopLogEntry {
                    timestamp_ms: index,
                    level: "info".to_string(),
                    category: "sidecar".to_string(),
                    message: format!("entry-{index}"),
                },
                3,
            );
        }

        let messages = entries
            .iter()
            .map(|entry| entry.message.as_str())
            .collect::<Vec<_>>();
        assert_eq!(messages, vec!["entry-1", "entry-2", "entry-3"]);
    }

    #[test]
    fn recent_desktop_log_limit_is_clamped_to_safe_range() {
        assert_eq!(clamp_recent_log_limit(None), 50);
        assert_eq!(clamp_recent_log_limit(Some(0)), 1);
        assert_eq!(clamp_recent_log_limit(Some(12)), 12);
        assert_eq!(clamp_recent_log_limit(Some(999)), 50);
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

    #[test]
    fn decode_chunked_body_decodes_json_payload() {
        let raw = "1d\r\n[{\"id\":1,\"account_name\":\"a\"}]\r\n0\r\n\r\n";
        let decoded = decode_chunked_body(raw).expect("decode chunked body");
        assert_eq!(decoded, "[{\"id\":1,\"account_name\":\"a\"}]");
    }
}
