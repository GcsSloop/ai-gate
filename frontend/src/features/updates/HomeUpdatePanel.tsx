import { CloudDownloadOutlined, CloseOutlined, ReloadOutlined } from "@ant-design/icons";
import { Button, Progress, Typography } from "antd";
import { useEffect, useMemo, useState } from "react";

import type { AppLanguage, Translator } from "../../lib/i18n";
import {
  createDesktopUpdateService,
  emptyUpdateState,
  type DesktopUpdateInfo,
  type DesktopUpdateService,
  type DesktopUpdateState,
} from "./updateService";

const { Paragraph, Text } = Typography;
const ACTIVE_POLL_MS = 1000;

type HomeUpdatePanelProps = {
  currentVersion: string;
  initialUpdate: DesktopUpdateInfo | null;
  language: AppLanguage;
  t: Translator;
  service?: DesktopUpdateService;
};

function formatDate(value: string | null | undefined, language: AppLanguage) {
  if (!value) {
    return null;
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString(language, { hour12: false });
}

function buildInitialState(update: DesktopUpdateInfo | null): DesktopUpdateState {
  if (!update) {
    return { ...emptyUpdateState("up-to-date") };
  }
  return {
    status: "available",
    update,
    progress: null,
    error: null,
  };
}

function describeState(state: DesktopUpdateState, t: Translator) {
  switch (state.status) {
    case "up-to-date":
      return t("已是最新版本");
    case "available":
      return state.update ? `${t("发现新版本")} ${state.update.version}` : t("发现新版本");
    case "downloading":
      return `${t("下载进度")} ${Math.round(state.progress?.percent ?? 0)}%`;
    case "ready":
      return t("更新已安装，重启后生效");
    case "unsupported":
      return state.update ? t("当前环境不支持自动安装，但已检查到最新版本。") : t("当前仅桌面版支持自动更新。");
    case "cancelled":
      return t("下载已取消");
    case "error":
      return state.error || t("安装更新失败");
    case "checking":
      return t("正在检查更新…");
    case "idle":
    default:
      return t("检查 GitHub Release 中的最新版本。");
  }
}

function shouldPoll(state: DesktopUpdateState) {
  return state.status === "checking" || state.status === "downloading";
}

export function HomeUpdatePanel({ currentVersion, initialUpdate, language, t, service }: HomeUpdatePanelProps) {
  const updateService = useMemo(() => service ?? createDesktopUpdateService(), [service]);
  const [state, setState] = useState<DesktopUpdateState>(() => buildInitialState(initialUpdate));

  async function hydrate() {
    const snapshot = await updateService.getState();
    setState(snapshot);
    return snapshot;
  }

  async function handleDownloadAndInstall(update: DesktopUpdateInfo) {
    try {
      await updateService.downloadAndInstall(update);
      await hydrate();
    } catch (error) {
      setState({
        status: "error",
        update,
        progress: null,
        error: error instanceof Error ? t(error.message) : t("安装更新失败"),
      });
    }
  }

  async function handleCancel() {
    try {
      await updateService.cancelDownload();
      await hydrate();
    } catch (error) {
      setState((current) => ({
        ...current,
        status: "error",
        error: error instanceof Error ? t(error.message) : t("安装更新失败"),
      }));
    }
  }

  useEffect(() => {
    let cancelled = false;

    async function bootstrap() {
      const snapshot = await updateService.getState();
      if (cancelled) {
        return;
      }
      if (snapshot.status !== "idle") {
        setState(snapshot);
        return;
      }
      const result = await updateService.check(currentVersion);
      if (cancelled) {
        return;
      }
      if (!result.supported) {
        setState({
          status: "unsupported",
          update: result.update,
          progress: null,
          error: null,
        });
        return;
      }
      setState(buildInitialState(result.update ?? initialUpdate));
    }

    void bootstrap();
    return () => {
      cancelled = true;
    };
  }, [currentVersion, initialUpdate, updateService]);

  useEffect(() => {
    if (!shouldPoll(state)) {
      return;
    }
    const timer = window.setInterval(() => {
      void hydrate();
    }, ACTIVE_POLL_MS);
    return () => {
      window.clearInterval(timer);
    };
  }, [state.status, state.progress?.percent]);

  const message = describeState(state, t);

  return (
    <div className="home-update-panel">
      <div className="home-update-panel-body">
        <div className="home-update-panel-meta">
          <div className="about-meta-row">
            <span>{t("当前版本")}</span>
            <strong>{currentVersion}</strong>
          </div>
          <div className="about-meta-row">
            <span>{t("状态")}</span>
            <strong className={`update-status-value${state.status === "checking" || state.status === "downloading" ? " is-checking" : ""}`}>{message}</strong>
          </div>

          {state.update ? (
            <>
              <div className="about-meta-row">
                <span>{state.status === "unsupported" ? t("最新版本") : t("目标版本")}</span>
                <strong>{state.update.version}</strong>
              </div>
              {state.update.date ? (
                <div className="about-meta-row">
                  <span>{t("发布时间")}</span>
                  <strong>{formatDate(state.update.date, language)}</strong>
                </div>
              ) : null}
              {state.update.body ? <Paragraph className="update-release-notes">{state.update.body}</Paragraph> : null}
            </>
          ) : null}
        </div>

        {state.status === "downloading" && state.progress ? <Progress percent={Math.round(state.progress.percent)} showInfo={false} /> : null}

        <div className="update-card-actions">
          {state.status === "available" && state.update ? (
            <Button
              aria-label={t("下载并安装")}
              type="primary"
              icon={<CloudDownloadOutlined />}
              onClick={() => void handleDownloadAndInstall(state.update!)}
            >
              {t("下载并安装")}
            </Button>
          ) : null}
          {state.status === "downloading" ? (
            <Button aria-label={t("终止下载")} icon={<CloseOutlined />} onClick={() => void handleCancel()}>
              {t("终止下载")}
            </Button>
          ) : null}
          {state.status === "ready" ? (
            <Button aria-label={t("立即重启")} type="primary" icon={<ReloadOutlined />} onClick={() => void updateService.relaunch()}>
              {t("立即重启")}
            </Button>
          ) : null}
          {state.status === "unsupported" && !state.update ? <Text type="secondary">{message}</Text> : null}
        </div>
      </div>
    </div>
  );
}
