#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use base64::Engine;
use futures_util::StreamExt;
use minisign_verify::{PublicKey, Signature};
use once_cell::sync::Lazy;
use reqwest::header::{HeaderValue, ACCEPT};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use std::collections::VecDeque;
use std::future::Future;
use std::io::{Read, Write};
use std::net::{IpAddr, TcpStream, ToSocketAddrs};
use std::path::{Path, PathBuf};
use std::process::{Child, ChildStdin, Command, Stdio};
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::Arc;
use std::sync::Mutex;
use std::thread::JoinHandle;
use std::time::{Duration, Instant};
use tauri::image::Image;
use tauri::menu::{Menu, MenuBuilder, MenuItemBuilder};
use tauri::tray::TrayIconBuilder;
use tauri::{AppHandle, Emitter, LogicalSize, Manager, Runtime, Size};
use tauri_plugin_updater::{Update, UpdaterExt};

#[cfg(windows)]
use std::os::windows::process::CommandExt;

static SIDECAR_CHILD: Lazy<Mutex<Option<Child>>> = Lazy::new(|| Mutex::new(None));
static SIDECAR_HEARTBEAT: Lazy<Mutex<Option<JoinHandle<()>>>> = Lazy::new(|| Mutex::new(None));
static SIDECAR_EXIT_WATCHER: Lazy<Mutex<Option<JoinHandle<()>>>> = Lazy::new(|| Mutex::new(None));
static SIDECAR_EXIT_REASON: Lazy<Mutex<Option<String>>> = Lazy::new(|| Mutex::new(None));
static SIDECAR_EXIT_WATCHER_STOP: AtomicBool = AtomicBool::new(false);
static RESUME_RECOVERY_WATCHER: Lazy<Mutex<Option<JoinHandle<()>>>> =
    Lazy::new(|| Mutex::new(None));
static RESUME_RECOVERY_WATCHER_STOP: AtomicBool = AtomicBool::new(false);
static RESUME_RECOVERY_IN_FLIGHT: AtomicBool = AtomicBool::new(false);
static RESUME_RECOVERY_LAST_STARTED_MS: AtomicU64 = AtomicU64::new(0);
static APP_EXITING: AtomicBool = AtomicBool::new(false);
static DESKTOP_RUNTIME: Lazy<Mutex<DesktopRuntime>> =
    Lazy::new(|| Mutex::new(DesktopRuntime::default()));
static DESKTOP_RECENT_LOGS: Lazy<Mutex<VecDeque<DesktopLogEntry>>> =
    Lazy::new(|| Mutex::new(VecDeque::new()));
static UPDATE_MANAGER: Lazy<Mutex<UpdateManagerState>> =
    Lazy::new(|| Mutex::new(UpdateManagerState::default()));

const DEFAULT_PROXY_HOST: &str = "127.0.0.1";
const DEFAULT_PROXY_PORT: u16 = 6789;
const SETTINGS_CACHE_FILE: &str = "desktop-settings.json";
const MAIN_WINDOW_LABEL: &str = "main";
const MAIN_WINDOW_MIN_WIDTH: u32 = 1024;
const MAIN_WINDOW_MIN_HEIGHT: u32 = 700;
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
const RESUME_RECOVERY_COOLDOWN: Duration = Duration::from_secs(10);
const DESKTOP_RECENT_LOG_CAPACITY: usize = 200;
const DESKTOP_RECENT_LOG_DEFAULT_LIMIT: usize = 50;
const DESKTOP_RECENT_LOG_MAX_LIMIT: usize = 50;
const UPDATE_POLL_CHUNK_SIZE: usize = 64 * 1024;
const UPDATE_CHECK_TIMEOUT: Duration = Duration::from_secs(15);
const SIDECAR_MACOS_NAME: &str = "routerd-universal-apple-darwin";
const SIDECAR_WINDOWS_NAME: &str = "routerd-x86_64-pc-windows-msvc.exe";
const TRAY_ICON_COLOR_BYTES: &[u8] = include_bytes!("../icons/tray-icon-color.png");
const TRAY_ICON_TEMPLATE_BYTES: &[u8] = include_bytes!("../icons/tray-icon-template.png");
const UPDATER_PUBKEY_BASE64: &str = "dW50cnVzdGVkIGNvbW1lbnQ6IG1pbmlzaWduIHB1YmxpYyBrZXk6IERGRDUyRkY5NzAzRDJGQzYKUldUR0x6MXcrUy9WMzdDd1VacitqN0JHSUc4UlVkSzB5bncwdUVNOTdhNys2aTIrTy85NXFyd2oK";
#[cfg(windows)]
const CREATE_NO_WINDOW: u32 = 0x0800_0000;

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq, Eq)]
struct WindowSizeCache {
    width: u32,
    height: u32,
}

impl WindowSizeCache {
    fn as_tauri_size(self) -> Size {
        Size::Logical(LogicalSize::new(self.width as f64, self.height as f64))
    }
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq, Eq)]
#[serde(default)]
struct DesktopSettingsCache {
    launch_at_login: bool,
    silent_start: bool,
    close_to_tray: bool,
    lan_share_enabled: bool,
    proxy_host: String,
    proxy_port: u16,
    main_window_size: Option<WindowSizeCache>,
}

impl Default for DesktopSettingsCache {
    fn default() -> Self {
        Self {
            launch_at_login: false,
            silent_start: false,
            close_to_tray: true,
            lan_share_enabled: false,
            proxy_host: DEFAULT_PROXY_HOST.to_string(),
            proxy_port: DEFAULT_PROXY_PORT,
            main_window_size: None,
        }
    }
}

impl DesktopSettingsCache {
    fn from_app_settings(value: AppSettingsPayload) -> Self {
        Self::default().updated_from_app_settings(value)
    }

    fn updated_from_app_settings(mut self, value: AppSettingsPayload) -> Self {
        let defaults = Self::default();
        let proxy_host = sanitize_local_proxy_host(value.proxy_host.trim(), &defaults.proxy_host);
        let proxy_port = if value.proxy_port == 0 {
            defaults.proxy_port
        } else {
            value.proxy_port
        };
        self.launch_at_login = value.launch_at_login;
        self.silent_start = value.silent_start;
        self.close_to_tray = value.close_to_tray;
        self.lan_share_enabled = value.lan_share_enabled;
        self.proxy_host = proxy_host;
        self.proxy_port = proxy_port;
        self
    }

    fn backend_addr(&self) -> String {
        format_host_port(&self.proxy_host, self.proxy_port)
    }

    fn listen_addr(&self) -> String {
        if self.lan_share_enabled {
            format_host_port("0.0.0.0", self.proxy_port)
        } else {
            self.backend_addr()
        }
    }

    fn backend_api_base(&self) -> String {
        format!("http://{}/ai-router/api", self.backend_addr())
    }
}

fn sanitize_local_proxy_host(host: &str, fallback: &str) -> String {
    let normalized = host.trim();
    if normalized.is_empty() {
        return fallback.to_string();
    }
    if normalized.eq_ignore_ascii_case("localhost") {
        return normalized.to_string();
    }
    match normalized.parse::<IpAddr>() {
        Ok(ip) if ip.is_loopback() => normalized.to_string(),
        _ => fallback.to_string(),
    }
}

fn format_host_port(host: &str, port: u16) -> String {
    match host.parse::<IpAddr>() {
        Ok(IpAddr::V6(_)) => format!("[{host}]:{port}"),
        _ => format!("{host}:{port}"),
    }
}

#[derive(Clone, Debug, Serialize, Deserialize)]
struct AppSettingsPayload {
    launch_at_login: bool,
    silent_start: bool,
    close_to_tray: bool,
    show_proxy_switch_on_home: bool,
    lan_share_enabled: bool,
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

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
#[serde(rename_all = "camelCase")]
struct UpdateInfoPayload {
    body: Option<String>,
    current_version: String,
    date: Option<String>,
    version: String,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize, PartialEq)]
#[serde(rename_all = "camelCase")]
struct UpdateProgressPayload {
    percent: f64,
    total: u64,
    transferred: u64,
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "kebab-case")]
enum UpdateStatus {
    Idle,
    Checking,
    UpToDate,
    Available,
    Downloading,
    Ready,
    Unsupported,
    Cancelled,
    Error,
}

impl Default for UpdateStatus {
    fn default() -> Self {
        Self::Idle
    }
}

#[derive(Clone, Debug, Default, Serialize, Deserialize, PartialEq)]
#[serde(rename_all = "camelCase")]
struct UpdateStatePayload {
    status: UpdateStatus,
    update: Option<UpdateInfoPayload>,
    progress: Option<UpdateProgressPayload>,
    error: Option<String>,
}

#[derive(Default)]
struct UpdateManagerState {
    snapshot: UpdateStatePayload,
    active_version: Option<String>,
    cancel_flag: Option<Arc<AtomicBool>>,
}

impl UpdateManagerState {
    fn snapshot(&self) -> UpdateStatePayload {
        self.snapshot.clone()
    }

    fn set_snapshot(&mut self, snapshot: UpdateStatePayload) {
        self.snapshot = snapshot;
    }

    fn begin_check(&mut self) -> bool {
        if self.snapshot.status == UpdateStatus::Checking {
            return false;
        }
        self.snapshot = UpdateStatePayload {
            status: UpdateStatus::Checking,
            update: None,
            progress: None,
            error: None,
        };
        true
    }

    fn begin_download(&mut self, update: UpdateInfoPayload) -> Result<Arc<AtomicBool>, String> {
        if self.snapshot.status == UpdateStatus::Downloading {
            if self.active_version.as_deref() == Some(update.version.as_str()) {
                if let Some(flag) = &self.cancel_flag {
                    return Ok(flag.clone());
                }
            }
            return Err("Another update download is already in progress.".to_string());
        }
        let cancel_flag = Arc::new(AtomicBool::new(false));
        self.active_version = Some(update.version.clone());
        self.cancel_flag = Some(cancel_flag.clone());
        self.snapshot = UpdateStatePayload {
            status: UpdateStatus::Downloading,
            update: Some(update),
            progress: Some(UpdateProgressPayload::default()),
            error: None,
        };
        Ok(cancel_flag)
    }

    fn request_cancel(&mut self) {
        if let Some(flag) = &self.cancel_flag {
            flag.store(true, Ordering::SeqCst);
        }
    }

    fn finish_terminal(
        &mut self,
        status: UpdateStatus,
        update: Option<UpdateInfoPayload>,
        error: Option<String>,
    ) {
        self.active_version = None;
        self.cancel_flag = None;
        self.snapshot = UpdateStatePayload {
            status,
            update,
            progress: None,
            error,
        };
    }
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
    APP_EXITING.store(false, Ordering::SeqCst);
    tauri::Builder::default()
        .plugin(tauri_plugin_process::init())
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_updater::Builder::new().build())
        .invoke_handler(tauri::generate_handler![
            force_exit_app,
            refresh_tray_state,
            get_desktop_shell_context,
            apply_app_settings,
            get_app_metadata,
            get_recent_desktop_logs,
            write_clipboard_text,
            get_update_state,
            check_for_app_update,
            start_update_download,
            cancel_update_download
        ])
        .setup(|app| {
            let cache = initialize_runtime(app.handle())?;
            if let Err(err) = sync_launch_agent(cache.launch_at_login) {
                eprintln!("sync launch agent failed: {err}");
            }
            apply_saved_main_window_size(app.handle())?;
            spawn_sidecar()?;
            wait_for_backend_ready(
                &cache.backend_addr(),
                Duration::from_millis(SIDECAR_READY_WAIT_TIMEOUT_MS),
            )?;
            start_sidecar_exit_watcher()?;
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
                if label == MAIN_WINDOW_LABEL {
                    match event {
                        tauri::WindowEvent::CloseRequested { api, .. } => {
                            match window_close_action(current_settings_cache().close_to_tray) {
                                WindowCloseAction::MinimizeWindow => {
                                    api.prevent_close();
                                    if let Some(window) =
                                        app_handle.get_webview_window(MAIN_WINDOW_LABEL)
                                    {
                                        let _ = window.minimize();
                                    }
                                }
                                WindowCloseAction::ExitApp => {
                                    api.prevent_close();
                                    mark_app_exit_started();
                                    stop_sidecar_exit_watcher();
                                    stop_resume_recovery_watcher();
                                    shutdown_sidecar();
                                    app_handle.exit(0);
                                }
                            }
                        }
                        tauri::WindowEvent::Resized(_) => {
                            let _ = persist_main_window_size_from_window(&app_handle);
                        }
                        _ => {}
                    }
                }
            }
            #[cfg(target_os = "macos")]
            tauri::RunEvent::Reopen { .. } => {
                recover_backend_after_reopen(&app_handle);
                show_main_window(app_handle);
            }
            tauri::RunEvent::Exit => {
                mark_app_exit_started();
                stop_sidecar_exit_watcher();
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
    mark_app_exit_started();
    stop_sidecar_exit_watcher();
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

#[tauri::command]
fn write_clipboard_text(text: String) -> Result<(), String> {
    let mut clipboard = arboard::Clipboard::new().map_err(|err| err.to_string())?;
    clipboard.set_text(text).map_err(|err| err.to_string())
}

#[tauri::command]
fn get_update_state<R: Runtime>(app: AppHandle<R>) -> Result<UpdateStatePayload, String> {
    let current_version = app.package_info().version.to_string();
    Ok(current_update_snapshot(&current_version))
}

#[tauri::command]
async fn check_for_app_update<R: Runtime>(app: AppHandle<R>) -> Result<UpdateStatePayload, String> {
    let current_version = app.package_info().version.to_string();
    let checking_snapshot = {
        let mut manager = lock_update_manager()?;
        if !manager.begin_check() {
            Some(normalize_update_snapshot(manager.snapshot(), &current_version))
        } else {
            None
        }
    };
    if let Some(snapshot) = checking_snapshot {
        return Ok(snapshot);
    }

    match resolve_update_future_with_timeout(fetch_update(&app), UPDATE_CHECK_TIMEOUT, "check update").await {
        Ok(Some(update)) => {
            let payload = to_update_info_payload(&current_version, &update)?;
            let snapshot = UpdateStatePayload {
                status: UpdateStatus::Available,
                update: Some(payload),
                progress: None,
                error: None,
            };
            lock_update_manager()?.set_snapshot(snapshot.clone());
            Ok(snapshot)
        }
        Ok(None) => {
            let snapshot = UpdateStatePayload {
                status: UpdateStatus::UpToDate,
                update: None,
                progress: None,
                error: None,
            };
            lock_update_manager()?.set_snapshot(snapshot.clone());
            Ok(snapshot)
        }
        Err(err) => {
            lock_update_manager()?.set_snapshot(UpdateStatePayload {
                status: UpdateStatus::Error,
                update: None,
                progress: None,
                error: Some(err.clone()),
            });
            Err(err)
        }
    }
}

#[tauri::command]
async fn start_update_download<R: Runtime>(
    app: AppHandle<R>,
    version: String,
) -> Result<UpdateStatePayload, String> {
    let current_version = app.package_info().version.to_string();
    {
        let manager = lock_update_manager()?;
        if manager.snapshot.status == UpdateStatus::Downloading
            && manager.active_version.as_deref() == Some(version.as_str())
        {
            return Ok(manager.snapshot());
        }
    }

    let update = resolve_update_future_with_timeout(
        fetch_update(&app),
        UPDATE_CHECK_TIMEOUT,
        "fetch update metadata",
    )
        .await?
        .ok_or_else(|| "Update is no longer available. Check again and retry.".to_string())?;
    let payload = to_update_info_payload(&current_version, &update)?;
    if payload.version != version {
        return Err("Update version mismatch. Check again and retry.".to_string());
    }

    let cancel_flag = {
        let mut manager = lock_update_manager()?;
        manager.begin_download(payload.clone())?
    };

    let app_handle = app.clone();
    tauri::async_runtime::spawn(async move {
        let result = download_and_install_update(update, cancel_flag.clone()).await;
        if cancel_flag.load(Ordering::SeqCst) {
            if let Ok(mut manager) = lock_update_manager() {
                manager.finish_terminal(UpdateStatus::Cancelled, Some(payload), None);
            }
            return;
        }
        match result {
            Ok(()) => {
                if let Ok(mut manager) = lock_update_manager() {
                    manager.finish_terminal(UpdateStatus::Ready, Some(payload), None);
                }
            }
            Err(err) => {
                if let Ok(mut manager) = lock_update_manager() {
                    manager.finish_terminal(UpdateStatus::Error, Some(payload), Some(err));
                }
            }
        }
        let _ = app_handle.emit("aigate-update-state-changed", ());
    });

    Ok(current_update_snapshot(&current_version))
}

#[tauri::command]
fn cancel_update_download<R: Runtime>(app: AppHandle<R>) -> Result<UpdateStatePayload, String> {
    let _ = app;
    let mut manager = lock_update_manager()?;
    manager.request_cancel();
    Ok(manager.snapshot())
}

fn lock_update_manager() -> Result<std::sync::MutexGuard<'static, UpdateManagerState>, String> {
    UPDATE_MANAGER
        .lock()
        .map_err(|_| "update manager lock poisoned".to_string())
}

fn current_update_snapshot(current_version: &str) -> UpdateStatePayload {
    let snapshot = lock_update_manager()
        .map(|manager| manager.snapshot())
        .unwrap_or_default();
    normalize_update_snapshot(snapshot, current_version)
}

fn normalize_update_snapshot(
    mut snapshot: UpdateStatePayload,
    current_version: &str,
) -> UpdateStatePayload {
    if let Some(update) = snapshot.update.as_mut() {
        if update.current_version.is_empty() {
            update.current_version = current_version.to_string();
        }
    }
    snapshot
}

async fn resolve_update_future_with_timeout<T, F>(
    future: F,
    timeout: Duration,
    label: &str,
) -> Result<T, String>
where
    F: Future<Output = Result<T, String>>,
{
    match tokio::time::timeout(timeout, future).await {
        Ok(result) => result,
        Err(_) => Err(format!("{label} timed out after {}ms", timeout.as_millis())),
    }
}

async fn fetch_update<R: Runtime>(app: &AppHandle<R>) -> Result<Option<Update>, String> {
    app.updater()
        .map_err(|err| format!("build updater failed: {err}"))?
        .check()
        .await
        .map_err(|err| format!("check update failed: {err}"))
}

fn to_update_info_payload(
    current_version: &str,
    update: &Update,
) -> Result<UpdateInfoPayload, String> {
    Ok(UpdateInfoPayload {
        body: update.body.clone(),
        current_version: current_version.to_string(),
        date: update.date.map(|value| value.to_string()),
        version: update.version.clone(),
    })
}

async fn download_and_install_update(
    update: Update,
    cancel_flag: Arc<AtomicBool>,
) -> Result<(), String> {
    let bytes = download_update_bytes(&update, cancel_flag.clone()).await?;
    if cancel_flag.load(Ordering::SeqCst) {
        return Err("download canceled".to_string());
    }
    update
        .install(bytes)
        .map_err(|err| format!("install update failed: {err}"))
}

async fn download_update_bytes(
    update: &Update,
    cancel_flag: Arc<AtomicBool>,
) -> Result<Vec<u8>, String> {
    let mut headers = update.headers.clone();
    if !headers.contains_key(ACCEPT) {
        headers.insert(ACCEPT, HeaderValue::from_static("application/octet-stream"));
    }

    let mut request = reqwest::ClientBuilder::new().user_agent("aigate-desktop-updater");
    if let Some(timeout) = update.timeout {
        request = request.timeout(timeout);
    }
    if update.no_proxy {
        request = request.no_proxy();
    } else if let Some(ref proxy) = update.proxy {
        let parsed = reqwest::Proxy::all(proxy.as_str())
            .map_err(|err| format!("configure updater proxy failed: {err}"))?;
        request = request.proxy(parsed);
    }

    let response = request
        .build()
        .map_err(|err| format!("build updater client failed: {err}"))?
        .get(update.download_url.clone())
        .headers(headers)
        .send()
        .await
        .map_err(|err| format!("download update failed: {err}"))?;

    if !response.status().is_success() {
        return Err(format!(
            "download request failed with status: {}",
            response.status()
        ));
    }

    let total = response.content_length().unwrap_or(0);
    let mut transferred = 0_u64;
    let mut buffer = Vec::with_capacity(total.min(UPDATE_POLL_CHUNK_SIZE as u64) as usize);
    let mut stream = response.bytes_stream();

    while let Some(next) = stream.next().await {
        if cancel_flag.load(Ordering::SeqCst) {
            return Err("download canceled".to_string());
        }
        let chunk = next.map_err(|err| format!("read update chunk failed: {err}"))?;
        transferred += chunk.len() as u64;
        buffer.extend_from_slice(&chunk);
        update_download_progress(
            transferred,
            total,
            Some(UpdateInfoPayload {
                body: update.body.clone(),
                current_version: update.current_version.clone(),
                date: update.date.map(|value| value.to_string()),
                version: update.version.clone(),
            }),
        )?;
    }

    verify_update_signature(&buffer, &update.signature)?;
    Ok(buffer)
}

fn update_download_progress(
    transferred: u64,
    total: u64,
    update: Option<UpdateInfoPayload>,
) -> Result<(), String> {
    let percent = if total > 0 {
        ((transferred as f64 / total as f64) * 100.0).clamp(0.0, 100.0)
    } else {
        0.0
    };
    let mut manager = lock_update_manager()?;
    let existing_update = manager.snapshot.update.clone();
    manager.set_snapshot(UpdateStatePayload {
        status: UpdateStatus::Downloading,
        update: update.or(existing_update),
        progress: Some(UpdateProgressPayload {
            percent,
            total,
            transferred,
        }),
        error: None,
    });
    Ok(())
}

fn verify_update_signature(data: &[u8], release_signature: &str) -> Result<(), String> {
    let pub_key = decode_base64_to_string(UPDATER_PUBKEY_BASE64)?;
    let public_key = PublicKey::decode(&pub_key)
        .map_err(|err| format!("decode updater public key failed: {err}"))?;
    let signature_text = decode_base64_to_string(release_signature)?;
    let signature = Signature::decode(&signature_text)
        .map_err(|err| format!("decode update signature failed: {err}"))?;
    public_key
        .verify(data, &signature, true)
        .map_err(|err| format!("verify update signature failed: {err}"))?;
    Ok(())
}

fn decode_base64_to_string(value: &str) -> Result<String, String> {
    let decoded = base64::engine::general_purpose::STANDARD
        .decode(value)
        .map_err(|err| format!("decode base64 failed: {err}"))?;
    std::str::from_utf8(&decoded)
        .map_err(|_| "decode utf-8 failed".to_string())
        .map(|text| text.to_string())
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
    let mut cache = serde_json::from_str::<DesktopSettingsCache>(&raw).unwrap_or_default();
    let fallback = DesktopSettingsCache::default().proxy_host;
    cache.proxy_host = sanitize_local_proxy_host(&cache.proxy_host, &fallback);
    if cache.proxy_port == 0 {
        cache.proxy_port = DEFAULT_PROXY_PORT;
    }
    cache
}

fn persist_runtime_settings(cache: DesktopSettingsCache) -> Result<bool, String> {
    let mut runtime = DESKTOP_RUNTIME
        .lock()
        .map_err(|_| "desktop runtime lock poisoned".to_string())?;
    let previous_addr = runtime.settings_cache.listen_addr();
    persist_settings_cache(&runtime.settings_path, &cache)?;
    runtime.settings_cache = cache.clone();
    Ok(previous_addr != cache.listen_addr())
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

fn sanitize_main_window_size(width: u32, height: u32) -> Option<WindowSizeCache> {
    if width == 0 || height == 0 {
        return None;
    }
    Some(WindowSizeCache {
        width: width.max(MAIN_WINDOW_MIN_WIDTH),
        height: height.max(MAIN_WINDOW_MIN_HEIGHT),
    })
}

fn resolve_main_window_size(size: Option<WindowSizeCache>) -> WindowSizeCache {
    size.unwrap_or(WindowSizeCache {
        width: MAIN_WINDOW_MIN_WIDTH,
        height: MAIN_WINDOW_MIN_HEIGHT,
    })
}

fn apply_saved_main_window_size<R: Runtime>(app: &AppHandle<R>) -> Result<(), String> {
    let size = resolve_main_window_size(current_settings_cache().main_window_size);
    if let Some(window) = app.get_webview_window(MAIN_WINDOW_LABEL) {
        window
            .set_size(size.as_tauri_size())
            .map_err(|e| format!("apply main window size failed: {e}"))?;
    }
    Ok(())
}

fn persist_main_window_size(size: WindowSizeCache) -> Result<(), String> {
    let mut runtime = DESKTOP_RUNTIME
        .lock()
        .map_err(|_| "desktop runtime lock poisoned".to_string())?;
    if runtime.settings_cache.main_window_size == Some(size.clone()) {
        return Ok(());
    }
    runtime.settings_cache.main_window_size = Some(size);
    persist_settings_cache(&runtime.settings_path, &runtime.settings_cache)
}

fn persist_main_window_size_from_window<R: Runtime>(app: &AppHandle<R>) -> Result<(), String> {
    let Some(window) = app.get_webview_window(MAIN_WINDOW_LABEL) else {
        return Ok(());
    };
    let inner_size = window
        .inner_size()
        .map_err(|e| format!("read main window size failed: {e}"))?;
    let scale_factor = window
        .scale_factor()
        .map_err(|e| format!("read main window scale factor failed: {e}"))?;
    let logical_size = inner_size.to_logical::<f64>(scale_factor);
    let width = logical_size.width.round().max(0.0) as u32;
    let height = logical_size.height.round().max(0.0) as u32;
    let Some(size) = sanitize_main_window_size(width, height) else {
        return Ok(());
    };
    persist_main_window_size(size)
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
    {
        let guard = SIDECAR_CHILD
            .lock()
            .map_err(|_| "sidecar child lock poisoned".to_string())?;
        if guard.is_some() {
            return Err("sidecar already running".to_string());
        }
    }
    let runtime = DESKTOP_RUNTIME
        .lock()
        .map_err(|_| "desktop runtime lock poisoned".to_string())?
        .clone();

    log_desktop_event(
        "info",
        "sidecar",
        format!(
            "spawn requested path={} listen_addr={} backend_addr={}",
            runtime.sidecar_path.display(),
            runtime.settings_cache.listen_addr(),
            runtime.settings_cache.backend_addr()
        ),
    );

    let mut command = Command::new(&runtime.sidecar_path);
    command
        .env(
            "CODEX_ROUTER_LISTEN_ADDR",
            runtime.settings_cache.listen_addr(),
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
    set_sidecar_exit_reason(None);
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
    if !should_dispatch_resume_recovery(is_app_exiting()) {
        return;
    }
    request_resume_recovery(app.clone(), "reopen".to_string(), None);
}

fn mark_app_exit_started() {
    APP_EXITING.store(true, Ordering::SeqCst);
}

fn is_app_exiting() -> bool {
    APP_EXITING.load(Ordering::SeqCst)
}

fn should_dispatch_resume_recovery(app_is_exiting: bool) -> bool {
    !app_is_exiting
}

#[cfg(target_os = "macos")]
struct ResumeRecoveryLease<'a> {
    running: &'a AtomicBool,
}

#[cfg(target_os = "macos")]
impl Drop for ResumeRecoveryLease<'_> {
    fn drop(&mut self) {
        self.running.store(false, Ordering::SeqCst);
    }
}

#[cfg(target_os = "macos")]
fn try_acquire_resume_recovery_slot<'a>(
    running: &'a AtomicBool,
    last_started_ms: &AtomicU64,
    now_ms: u64,
    cooldown_ms: u64,
) -> Option<ResumeRecoveryLease<'a>> {
    let last_started = last_started_ms.load(Ordering::SeqCst);
    if running.load(Ordering::SeqCst)
        || (last_started != 0 && now_ms.saturating_sub(last_started) < cooldown_ms)
    {
        return None;
    }
    running
        .compare_exchange(false, true, Ordering::SeqCst, Ordering::SeqCst)
        .ok()?;
    last_started_ms.store(now_ms, Ordering::SeqCst);
    Some(ResumeRecoveryLease { running })
}

#[cfg(target_os = "macos")]
fn current_unix_timestamp_ms() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|duration| duration.as_millis() as u64)
        .unwrap_or_default()
}

#[cfg(target_os = "macos")]
fn probe_backend_after_resume(trigger: &str, gap: Option<Duration>) {
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
}

#[cfg(target_os = "macos")]
fn request_resume_recovery<R: Runtime + 'static>(
    app: AppHandle<R>,
    trigger: String,
    gap: Option<Duration>,
) {
    if !should_dispatch_resume_recovery(is_app_exiting()) {
        return;
    }
    let gap_suffix = gap
        .map(|value| format!(" gap_ms={}", value.as_millis()))
        .unwrap_or_default();
    let cooldown_ms = RESUME_RECOVERY_COOLDOWN.as_millis() as u64;
    let now_ms = current_unix_timestamp_ms();
    let Some(_lease) = try_acquire_resume_recovery_slot(
        &RESUME_RECOVERY_IN_FLIGHT,
        &RESUME_RECOVERY_LAST_STARTED_MS,
        now_ms,
        cooldown_ms,
    ) else {
        log_desktop_event(
            "info",
            "recovery",
            format!("skip coalesced recovery trigger={trigger}{gap_suffix}"),
        );
        return;
    };

    let _ = std::thread::Builder::new()
        .name("resume-recovery-dispatch".to_string())
        .spawn(move || {
            let _lease = _lease;
            probe_backend_after_resume(&trigger, gap);

            if !should_dispatch_resume_recovery(is_app_exiting()) {
                log_desktop_event(
                    "info",
                    "recovery",
                    format!("skip resume recovery dispatch during exit trigger={trigger}"),
                );
                return;
            }

            let app_handle = app.clone();
            let _ = app.run_on_main_thread(move || {
                refresh_tray_state_from_backend(&app_handle);
                emit_backend_state_changed(&app_handle);
            });
        });
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
            mark_app_exit_started();
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

fn should_restart_sidecar_after_exit(reason: Option<&str>, exited: bool) -> bool {
    if !exited {
        return false;
    }
    !matches!(
        reason.map(str::trim).filter(|value| !value.is_empty()),
        Some("shutdown" | "restart" | "quit")
    )
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
    set_sidecar_exit_reason(Some(reason));
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

    stop_sidecar_heartbeat();
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
                    request_resume_recovery(app.clone(), "resume_gap".to_string(), Some(elapsed));
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

fn stop_sidecar_heartbeat() {
    if let Ok(mut heartbeat_guard) = SIDECAR_HEARTBEAT.lock() {
        if let Some(handle) = heartbeat_guard.take() {
            let _ = handle.join();
        }
    }
}

fn set_sidecar_exit_reason(reason: Option<&str>) {
    if let Ok(mut guard) = SIDECAR_EXIT_REASON.lock() {
        *guard = reason.map(str::to_string);
    }
}

fn take_sidecar_exit_reason() -> Option<String> {
    SIDECAR_EXIT_REASON
        .lock()
        .ok()
        .and_then(|mut guard| guard.take())
}

fn start_sidecar_exit_watcher() -> Result<(), String> {
    SIDECAR_EXIT_WATCHER_STOP.store(false, Ordering::SeqCst);
    let handle = std::thread::Builder::new()
        .name("sidecar-exit-watcher".to_string())
        .spawn(move || {
            while !SIDECAR_EXIT_WATCHER_STOP.load(Ordering::SeqCst) {
                let mut exited = false;
                let mut status_text = String::new();
                if let Ok(mut guard) = SIDECAR_CHILD.lock() {
                    if let Some(child) = guard.as_mut() {
                        match child.try_wait() {
                            Ok(Some(status)) => {
                                exited = true;
                                status_text = status.to_string();
                                *guard = None;
                            }
                            Ok(None) => {}
                            Err(err) => {
                                log_desktop_event(
                                    "warn",
                                    "sidecar",
                                    format!("try_wait failed: {err}"),
                                );
                            }
                        }
                    }
                }

                if exited {
                    stop_sidecar_heartbeat();
                    let reason = take_sidecar_exit_reason();
                    log_desktop_event(
                        "warn",
                        "sidecar",
                        format!(
                            "sidecar exited unexpectedly status={} reason={}",
                            status_text,
                            reason.as_deref().unwrap_or("unexpected")
                        ),
                    );
                    if should_restart_sidecar_after_exit(reason.as_deref(), true) {
                        log_desktop_event("warn", "recovery", "auto restarting sidecar after exit");
                        match spawn_sidecar() {
                            Ok(()) => {
                                if let Err(err) = wait_for_backend_ready(
                                    &current_backend_addr(),
                                    Duration::from_millis(SIDECAR_READY_WAIT_TIMEOUT_MS),
                                ) {
                                    log_desktop_event(
                                        "error",
                                        "recovery",
                                        format!("sidecar auto restart readiness failed: {err}"),
                                    );
                                }
                            }
                            Err(err) => log_desktop_event(
                                "error",
                                "recovery",
                                format!("sidecar auto restart failed: {err}"),
                            ),
                        }
                    }
                }

                std::thread::sleep(Duration::from_millis(500));
            }
        })
        .map_err(|e| format!("start sidecar exit watcher failed: {e}"))?;
    let mut guard = SIDECAR_EXIT_WATCHER
        .lock()
        .map_err(|_| "sidecar exit watcher lock poisoned".to_string())?;
    if let Some(previous) = guard.replace(handle) {
        let _ = previous.join();
    }
    Ok(())
}

fn stop_sidecar_exit_watcher() {
    SIDECAR_EXIT_WATCHER_STOP.store(true, Ordering::SeqCst);
    if let Ok(mut guard) = SIDECAR_EXIT_WATCHER.lock() {
        if let Some(handle) = guard.take() {
            let _ = handle.join();
        }
    }
}

#[cfg(test)]
mod tests {
    use super::{
        append_recent_desktop_log, build_launch_agent_plist, clamp_recent_log_limit,
        current_backend_addr, current_settings_cache, decode_chunked_body, format_timeout_error,
        format_tray_title, load_settings_cache, map_backend_io_error, parse_account_menu_id,
        parse_accounts_response, parse_proxy_status_response, persist_runtime_settings,
        proxy_menu_enabled_states, request_backend, resolve_main_window_size,
        restart_sidecar_and_wait_ready, sanitize_main_window_size, should_attempt_sidecar_recovery,
        should_dispatch_resume_recovery, should_refresh_tray_after_action,
        should_restart_sidecar_after_exit, should_retry_sidecar_request,
        should_trigger_resume_recovery, shutdown_sidecar_with_reason,
        sidecar_candidate_paths, sidecar_creation_flags, sidecar_request_with_recovery,
        sidecar_request_with_recovery_hooks, sidecar_resource_name, spawn_sidecar,
        tray_icon_bytes_for_platform, tray_icon_is_template_for_platform, update_download_progress,
        wait_for_backend_ready, wait_for_backend_ready_with_probe, window_close_action,
        AppSettingsPayload, DesktopLogEntry, DesktopRuntime, DesktopSettingsCache, HttpResponse,
        UpdateInfoPayload, UpdateManagerState, UpdateProgressPayload, UpdateStatePayload,
        UpdateStatus, WindowCloseAction, WindowSizeCache, DESKTOP_RUNTIME, MAIN_WINDOW_MIN_HEIGHT,
        MAIN_WINDOW_MIN_WIDTH, SIDECAR_CHILD, SIDECAR_MACOS_NAME, SIDECAR_WINDOWS_NAME,
        TRAY_ICON_COLOR_BYTES, TRAY_ICON_TEMPLATE_BYTES, UPDATE_MANAGER,
        resolve_update_future_with_timeout,
    };
    use std::cell::RefCell;
    use std::collections::VecDeque;
    use std::fs;
    use std::net::TcpListener;
    use std::path::{Path, PathBuf};
    use std::process::Command;
    use std::sync::atomic::Ordering;
    use std::sync::Arc;
    use std::time::Duration;
    use std::time::{SystemTime, UNIX_EPOCH};

    #[cfg(target_os = "macos")]
    use super::try_acquire_resume_recovery_slot;
    #[cfg(target_os = "macos")]
    use std::sync::atomic::{AtomicBool, AtomicU64, AtomicUsize};

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
        assert!(!cache.lan_share_enabled);
        assert_eq!(cache.proxy_host, "127.0.0.1");
        assert_eq!(cache.proxy_port, 6789);
        assert_eq!(cache.main_window_size, None);
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
            lan_share_enabled: true,
            proxy_host: "localhost".to_string(),
            proxy_port: 18080,
            auto_failover_enabled: true,
            auto_backup_interval_hours: 12,
            backup_retention_count: 7,
        };

        let cache = DesktopSettingsCache::default().updated_from_app_settings(payload);
        assert!(cache.launch_at_login);
        assert!(cache.silent_start);
        assert!(!cache.close_to_tray);
        assert!(cache.lan_share_enabled);
        assert_eq!(cache.proxy_host, "localhost");
        assert_eq!(cache.proxy_port, 18080);
        assert_eq!(cache.backend_addr(), "localhost:18080");
        assert_eq!(cache.listen_addr(), "0.0.0.0:18080");
    }

    #[test]
    fn desktop_settings_cache_sanitizes_invalid_proxy_endpoint() {
        let payload = AppSettingsPayload {
            launch_at_login: false,
            silent_start: false,
            close_to_tray: true,
            show_proxy_switch_on_home: true,
            lan_share_enabled: false,
            proxy_host: "   ".to_string(),
            proxy_port: 0,
            auto_failover_enabled: false,
            auto_backup_interval_hours: 24,
            backup_retention_count: 10,
        };

        let cache = DesktopSettingsCache::default().updated_from_app_settings(payload);
        assert_eq!(cache.proxy_host, "127.0.0.1");
        assert_eq!(cache.proxy_port, 6789);
    }

    #[test]
    fn desktop_settings_cache_rejects_non_local_proxy_host() {
        let payload = AppSettingsPayload {
            launch_at_login: false,
            silent_start: false,
            close_to_tray: true,
            show_proxy_switch_on_home: true,
            lan_share_enabled: true,
            proxy_host: "192.168.1.24".to_string(),
            proxy_port: 18080,
            auto_failover_enabled: false,
            auto_backup_interval_hours: 24,
            backup_retention_count: 10,
        };

        let cache = DesktopSettingsCache::default().updated_from_app_settings(payload);
        assert_eq!(cache.proxy_host, "127.0.0.1");
        assert_eq!(cache.proxy_port, 18080);
        assert_eq!(cache.backend_addr(), "127.0.0.1:18080");
        assert_eq!(cache.listen_addr(), "0.0.0.0:18080");
    }

    #[test]
    fn load_settings_cache_sanitizes_non_local_proxy_host_from_disk() {
        let unique = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("clock")
            .as_nanos();
        let dir = std::env::temp_dir().join(format!("aigate-desktop-tests-{unique}"));
        fs::create_dir_all(&dir).expect("create temp dir");
        let path = dir.join("desktop-settings.json");
        fs::write(
            &path,
            r#"{
  "launch_at_login": false,
  "silent_start": false,
  "close_to_tray": true,
  "lan_share_enabled": true,
  "proxy_host": "10.0.0.8",
  "proxy_port": 16789
}"#,
        )
        .expect("write settings cache");

        let cache = load_settings_cache(&path);
        assert!(cache.lan_share_enabled);
        assert_eq!(cache.proxy_host, "127.0.0.1");
        assert_eq!(cache.proxy_port, 16789);
        assert_eq!(cache.backend_addr(), "127.0.0.1:16789");
        assert_eq!(cache.listen_addr(), "0.0.0.0:16789");
        let _ = fs::remove_file(&path);
        let _ = fs::remove_dir(&dir);
    }

    #[test]
    fn resolved_main_window_size_uses_minimum_dimensions_by_default() {
        let size = resolve_main_window_size(None);

        assert_eq!(size.width, MAIN_WINDOW_MIN_WIDTH);
        assert_eq!(size.height, MAIN_WINDOW_MIN_HEIGHT);
    }

    #[test]
    fn desktop_settings_cache_preserves_saved_window_size_when_app_settings_change() {
        let initial = DesktopSettingsCache {
            main_window_size: Some(WindowSizeCache {
                width: 1440,
                height: 900,
            }),
            ..DesktopSettingsCache::default()
        };
        let payload = AppSettingsPayload {
            launch_at_login: true,
            silent_start: false,
            close_to_tray: true,
            show_proxy_switch_on_home: true,
            lan_share_enabled: false,
            proxy_host: "127.0.0.1".to_string(),
            proxy_port: 6789,
            auto_failover_enabled: false,
            auto_backup_interval_hours: 24,
            backup_retention_count: 10,
        };

        let updated = initial.updated_from_app_settings(payload);

        assert_eq!(
            updated.main_window_size,
            Some(WindowSizeCache {
                width: 1440,
                height: 900,
            })
        );
    }

    #[test]
    #[ignore = "local smoke test that builds and launches routerd"]
    fn lan_share_toggle_restarts_sidecar_without_changing_desktop_backend_addr() {
        let unique = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("clock")
            .as_nanos();
        let temp_root = std::env::temp_dir().join(format!("aigate-lan-smoke-{unique}"));
        fs::create_dir_all(&temp_root).expect("create smoke root");

        let backend_root = Path::new(env!("CARGO_MANIFEST_DIR")).join("../../backend");
        let sidecar_path = temp_root.join("routerd-smoke");
        let build_status = Command::new("go")
            .args(["build", "-o"])
            .arg(&sidecar_path)
            .arg("./cmd/routerd")
            .current_dir(&backend_root)
            .status()
            .expect("run go build");
        assert!(build_status.success(), "go build routerd failed");

        let listener = TcpListener::bind("127.0.0.1:0").expect("allocate local port");
        let port = listener.local_addr().expect("listener addr").port();
        drop(listener);

        let database_path = temp_root.join("aigate.sqlite");
        let settings_path = temp_root.join("desktop-settings.json");

        {
            let mut runtime = DESKTOP_RUNTIME.lock().expect("desktop runtime lock");
            *runtime = DesktopRuntime {
                sidecar_path: sidecar_path.clone(),
                database_path,
                settings_path,
                settings_cache: DesktopSettingsCache {
                    proxy_host: "127.0.0.1".to_string(),
                    proxy_port: port,
                    lan_share_enabled: false,
                    ..DesktopSettingsCache::default()
                },
            };
        }

        shutdown_sidecar_with_reason("smoke-cleanup-start");
        spawn_sidecar().expect("spawn initial sidecar");
        wait_for_backend_ready(&current_backend_addr(), Duration::from_secs(5))
            .expect("wait for initial backend");
        let initial_response = request_backend("GET", "/ai-router/api/settings/app", "")
            .expect("request app settings");
        assert_eq!(
            initial_response.status, 200,
            "initial backend request should succeed"
        );
        let initial_pid = SIDECAR_CHILD
            .lock()
            .expect("sidecar child lock")
            .as_ref()
            .map(|child| child.id())
            .expect("initial child pid");
        assert_eq!(current_backend_addr(), format!("127.0.0.1:{port}"));

        let mut shared_cache = current_settings_cache();
        shared_cache.lan_share_enabled = true;
        let restart_required =
            persist_runtime_settings(shared_cache).expect("persist shared runtime settings");
        assert!(
            restart_required,
            "lan toggle should require sidecar restart"
        );
        restart_sidecar_and_wait_ready().expect("restart sidecar after enabling lan");
        let shared_pid = SIDECAR_CHILD
            .lock()
            .expect("sidecar child lock")
            .as_ref()
            .map(|child| child.id())
            .expect("shared child pid");
        assert_ne!(
            shared_pid, initial_pid,
            "sidecar pid should change after restart"
        );
        assert_eq!(current_backend_addr(), format!("127.0.0.1:{port}"));
        assert_eq!(
            current_settings_cache().listen_addr(),
            format!("0.0.0.0:{port}")
        );
        let shared_response = request_backend("GET", "/ai-router/api/settings/app", "")
            .expect("request shared app settings");
        assert_eq!(
            shared_response.status, 200,
            "shared backend request should succeed"
        );

        let mut local_cache = current_settings_cache();
        local_cache.lan_share_enabled = false;
        let restart_required =
            persist_runtime_settings(local_cache).expect("persist local runtime settings");
        assert!(
            restart_required,
            "lan toggle reset should require sidecar restart"
        );
        restart_sidecar_and_wait_ready().expect("restart sidecar after disabling lan");
        let final_pid = SIDECAR_CHILD
            .lock()
            .expect("sidecar child lock")
            .as_ref()
            .map(|child| child.id())
            .expect("final child pid");
        assert_ne!(
            final_pid, shared_pid,
            "sidecar pid should change after second restart"
        );
        assert_eq!(current_backend_addr(), format!("127.0.0.1:{port}"));
        assert_eq!(
            current_settings_cache().listen_addr(),
            format!("127.0.0.1:{port}")
        );
        let final_response = request_backend("GET", "/ai-router/api/settings/app", "")
            .expect("request local app settings");
        assert_eq!(
            final_response.status, 200,
            "final backend request should succeed"
        );

        shutdown_sidecar_with_reason("smoke-cleanup-end");
        let _ = fs::remove_file(&sidecar_path);
        let _ = fs::remove_file(temp_root.join("desktop-settings.json"));
        let _ = fs::remove_file(temp_root.join("aigate.sqlite"));
        let _ = fs::remove_dir_all(&temp_root);
    }

    #[test]
    fn sanitize_main_window_size_clamps_small_dimensions_to_minimum() {
        let size = sanitize_main_window_size(800, 600).expect("size should be accepted");

        assert_eq!(size.width, MAIN_WINDOW_MIN_WIDTH);
        assert_eq!(size.height, MAIN_WINDOW_MIN_HEIGHT);
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
    fn sidecar_exit_restart_policy_skips_intentional_shutdowns() {
        assert!(!should_restart_sidecar_after_exit(Some("shutdown"), true));
        assert!(!should_restart_sidecar_after_exit(Some("restart"), true));
        assert!(!should_restart_sidecar_after_exit(Some("quit"), true));
    }

    #[test]
    fn sidecar_exit_restart_policy_recovers_unexpected_exit() {
        assert!(should_restart_sidecar_after_exit(None, true));
        assert!(should_restart_sidecar_after_exit(Some(""), true));
        assert!(!should_restart_sidecar_after_exit(None, false));
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

    #[cfg(target_os = "macos")]
    #[test]
    fn resume_recovery_slot_coalesces_overlapping_runs() {
        let running = AtomicBool::new(false);
        let last_started_ms = AtomicU64::new(0);
        let active_runs = Arc::new(AtomicUsize::new(0));
        let observed_max = Arc::new(AtomicUsize::new(0));

        std::thread::scope(|scope| {
            for _ in 0..2 {
                let active_runs = Arc::clone(&active_runs);
                let observed_max = Arc::clone(&observed_max);
                let running_ref = &running;
                let last_started_ms_ref = &last_started_ms;
                scope.spawn(move || {
                    let Some(_lease) = try_acquire_resume_recovery_slot(
                        running_ref,
                        last_started_ms_ref,
                        1_000,
                        10_000,
                    ) else {
                        return;
                    };
                    let concurrent = active_runs.fetch_add(1, Ordering::SeqCst) + 1;
                    observed_max.fetch_max(concurrent, Ordering::SeqCst);
                    std::thread::sleep(Duration::from_millis(50));
                    active_runs.fetch_sub(1, Ordering::SeqCst);
                });
            }
        });

        assert_eq!(observed_max.load(Ordering::SeqCst), 1);
        assert!(!running.load(Ordering::SeqCst));
    }

    #[cfg(target_os = "macos")]
    #[test]
    fn resume_recovery_slot_skips_recent_duplicate_triggers() {
        let running = AtomicBool::new(false);
        let last_started_ms = AtomicU64::new(0);

        let lease = try_acquire_resume_recovery_slot(&running, &last_started_ms, 1_000, 10_000)
            .expect("first recovery should acquire the slot");
        drop(lease);

        assert!(
            try_acquire_resume_recovery_slot(&running, &last_started_ms, 5_000, 10_000).is_none(),
            "duplicate trigger inside cooldown should be skipped"
        );
        assert!(
            try_acquire_resume_recovery_slot(&running, &last_started_ms, 11_500, 10_000).is_some(),
            "trigger after cooldown should be allowed"
        );
    }

    #[cfg(target_os = "macos")]
    #[test]
    fn resume_recovery_dispatch_skips_when_app_is_exiting() {
        assert!(
            !should_dispatch_resume_recovery(true),
            "resume recovery should not dispatch after exit begins"
        );
        assert!(
            should_dispatch_resume_recovery(false),
            "resume recovery should still dispatch while app is running"
        );
    }

    #[cfg(target_os = "macos")]
    #[test]
    #[ignore = "local smoke test that stresses duplicate resume recovery triggers"]
    fn resume_recovery_slot_stays_single_flight_under_burst_triggers() {
        let running = AtomicBool::new(false);
        let last_started_ms = AtomicU64::new(0);
        let active_runs = Arc::new(AtomicUsize::new(0));
        let observed_max = Arc::new(AtomicUsize::new(0));
        let completed_runs = Arc::new(AtomicUsize::new(0));

        std::thread::scope(|scope| {
            for _ in 0..32 {
                let active_runs = Arc::clone(&active_runs);
                let observed_max = Arc::clone(&observed_max);
                let completed_runs = Arc::clone(&completed_runs);
                let running_ref = &running;
                let last_started_ms_ref = &last_started_ms;
                scope.spawn(move || {
                    let Some(_lease) = try_acquire_resume_recovery_slot(
                        running_ref,
                        last_started_ms_ref,
                        50_000,
                        10_000,
                    ) else {
                        return;
                    };
                    let concurrent = active_runs.fetch_add(1, Ordering::SeqCst) + 1;
                    observed_max.fetch_max(concurrent, Ordering::SeqCst);
                    std::thread::sleep(Duration::from_millis(25));
                    active_runs.fetch_sub(1, Ordering::SeqCst);
                    completed_runs.fetch_add(1, Ordering::SeqCst);
                });
            }
        });

        assert_eq!(completed_runs.load(Ordering::SeqCst), 1);
        assert_eq!(observed_max.load(Ordering::SeqCst), 1);
        assert!(!running.load(Ordering::SeqCst));
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

    #[test]
    fn update_manager_initial_state_is_idle() {
        let manager = UpdateManagerState::default();
        assert_eq!(manager.snapshot.status, UpdateStatus::Idle);
        assert!(manager.snapshot.update.is_none());
        assert!(manager.snapshot.progress.is_none());
        assert!(manager.active_version.is_none());
    }

    #[test]
    fn update_manager_progress_uses_expected_percentages() {
        UPDATE_MANAGER
            .lock()
            .expect("update manager")
            .set_snapshot(UpdateStatePayload {
                status: UpdateStatus::Downloading,
                update: Some(UpdateInfoPayload {
                    body: None,
                    current_version: "1.1.7".to_string(),
                    date: None,
                    version: "1.1.9".to_string(),
                }),
                progress: Some(UpdateProgressPayload::default()),
                error: None,
            });

        update_download_progress(25, 100, None).expect("progress update");
        let snapshot = UPDATE_MANAGER.lock().expect("update manager").snapshot();
        let progress = snapshot.progress.expect("progress payload");
        assert_eq!(snapshot.status, UpdateStatus::Downloading);
        assert_eq!(progress.transferred, 25);
        assert_eq!(progress.total, 100);
        assert_eq!(progress.percent, 25.0);
    }

    #[test]
    fn update_manager_reuses_existing_task_for_same_version() {
        let mut manager = UpdateManagerState::default();
        let first_flag = manager
            .begin_download(UpdateInfoPayload {
                body: None,
                current_version: "1.1.7".to_string(),
                date: None,
                version: "1.1.9".to_string(),
            })
            .expect("first download");

        let second_flag = manager
            .begin_download(UpdateInfoPayload {
                body: None,
                current_version: "1.1.7".to_string(),
                date: None,
                version: "1.1.9".to_string(),
            })
            .expect("reuse same version");

        assert!(Arc::ptr_eq(&first_flag, &second_flag));
        assert_eq!(manager.snapshot.status, UpdateStatus::Downloading);
    }

    #[test]
    fn update_manager_cancellation_marks_flag_and_finishes_cancelled() {
        let mut manager = UpdateManagerState::default();
        let cancel_flag = manager
            .begin_download(UpdateInfoPayload {
                body: None,
                current_version: "1.1.7".to_string(),
                date: None,
                version: "1.1.9".to_string(),
            })
            .expect("begin download");

        manager.request_cancel();
        assert!(cancel_flag.load(Ordering::SeqCst));

        manager.finish_terminal(
            UpdateStatus::Cancelled,
            Some(UpdateInfoPayload {
                body: None,
                current_version: "1.1.7".to_string(),
                date: None,
                version: "1.1.9".to_string(),
            }),
            None,
        );

        assert_eq!(manager.snapshot.status, UpdateStatus::Cancelled);
        assert!(manager.cancel_flag.is_none());
        assert!(manager.active_version.is_none());
    }

    #[test]
    fn update_manager_deduplicates_checking_state() {
        let mut manager = UpdateManagerState::default();
        manager.set_snapshot(UpdateStatePayload {
            status: UpdateStatus::Available,
            update: Some(UpdateInfoPayload {
                body: None,
                current_version: "1.2.3".to_string(),
                date: None,
                version: "1.2.4".to_string(),
            }),
            progress: None,
            error: None,
        });

        assert!(manager.begin_check());
        assert_eq!(manager.snapshot.status, UpdateStatus::Checking);
        assert!(!manager.begin_check());
        assert_eq!(manager.snapshot.status, UpdateStatus::Checking);
        assert!(manager.snapshot.update.is_none());
    }

    #[test]
    fn update_check_timeout_returns_error_for_stalled_future() {
        let result = tauri::async_runtime::block_on(resolve_update_future_with_timeout(
            async {
                futures_util::future::pending::<Result<Option<UpdateInfoPayload>, String>>().await
            },
            Duration::from_millis(10),
            "check update",
        ));

        assert!(result.is_err());
        assert_eq!(
            result.expect_err("timeout error"),
            "check update timed out after 10ms"
        );
    }
}
