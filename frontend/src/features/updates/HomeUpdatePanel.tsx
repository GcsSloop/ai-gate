import { CloudDownloadOutlined, ReloadOutlined } from "@ant-design/icons";
import { Button, Progress, Typography } from "antd";
import { useEffect, useMemo, useState } from "react";

import type { AppLanguage, Translator } from "../../lib/i18n";
import { createDesktopUpdateService, type DesktopUpdateInfo, type DesktopUpdateService, type DownloadProgress } from "./updateService";

const { Paragraph, Text } = Typography;

type HomeUpdatePanelProps = {
  currentVersion: string;
  initialUpdate: DesktopUpdateInfo | null;
  language: AppLanguage;
  t: Translator;
  service?: DesktopUpdateService;
};

type HomeUpdateViewState =
  | { status: "up-to-date"; message: string }
  | { status: "available"; message: string; update: DesktopUpdateInfo }
  | { status: "downloading"; message: string; update: DesktopUpdateInfo; progress: DownloadProgress }
  | { status: "ready"; message: string; update: DesktopUpdateInfo }
  | { status: "unsupported"; message: string; update?: DesktopUpdateInfo }
  | { status: "error"; message: string };

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

function buildInitialState(update: DesktopUpdateInfo | null, t: Translator): HomeUpdateViewState {
  if (!update) {
    return { status: "up-to-date", message: t("已是最新版本") };
  }
  return {
    status: "available",
    message: `${t("发现新版本")} ${update.version}`,
    update,
  };
}

export function HomeUpdatePanel({ currentVersion, initialUpdate, language, t, service }: HomeUpdatePanelProps) {
  const updateService = useMemo(() => service ?? createDesktopUpdateService(), [service]);
  const [state, setState] = useState<HomeUpdateViewState>(() => buildInitialState(initialUpdate, t));

  useEffect(() => {
    setState((current) => {
      if (current.status === "downloading" || current.status === "ready") {
        return current;
      }
      return buildInitialState(initialUpdate, t);
    });
  }, [initialUpdate, t]);

  useEffect(() => {
    let cancelled = false;

    async function hydrate() {
      try {
        const result = await updateService.check(currentVersion);
        if (cancelled) {
          return;
        }
        if (!result.supported) {
          setState({
            status: "unsupported",
            message: result.update ? t("当前环境不支持自动安装，但已检查到最新版本。") : t("当前仅桌面版支持自动更新。"),
            update: result.update ?? undefined,
          });
          return;
        }
        if (!result.update) {
          setState({ status: "up-to-date", message: t("已是最新版本") });
          return;
        }
        setState({
          status: "available",
          message: `${t("发现新版本")} ${result.update.version}`,
          update: result.update,
        });
      } catch (error) {
        if (cancelled) {
          return;
        }
        setState({ status: "error", message: error instanceof Error ? t(error.message) : t("检查更新失败") });
      }
    }

    void hydrate();
    return () => {
      cancelled = true;
    };
  }, [currentVersion, t, updateService]);

  async function handleDownloadAndInstall(update: DesktopUpdateInfo) {
    setState({
      status: "downloading",
      message: `${t("下载进度")} 0%`,
      update,
      progress: { percent: 0, total: 0, transferred: 0 },
    });
    try {
      await updateService.downloadAndInstall(update, (progress) => {
        setState({
          status: "downloading",
          message: `${t("下载进度")} ${Math.round(progress.percent)}%`,
          update,
          progress,
        });
      });
      setState({
        status: "ready",
        message: t("更新已安装，重启后生效"),
        update,
      });
    } catch (error) {
      setState({ status: "error", message: error instanceof Error ? t(error.message) : t("安装更新失败") });
    }
  }

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
            <strong className={`update-status-value${state.status === "downloading" ? " is-checking" : ""}`}>{state.message}</strong>
          </div>

          {"update" in state && state.update ? (
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

        {state.status === "downloading" ? <Progress percent={Math.round(state.progress.percent)} showInfo={false} /> : null}

        <div className="update-card-actions">
          {state.status === "available" ? (
            <Button
              aria-label={t("下载并安装")}
              type="primary"
              icon={<CloudDownloadOutlined />}
              onClick={() => void handleDownloadAndInstall(state.update)}
            >
              {t("下载并安装")}
            </Button>
          ) : null}
          {state.status === "ready" ? (
            <Button aria-label={t("立即重启")} type="primary" icon={<ReloadOutlined />} onClick={() => void updateService.relaunch()}>
              {t("立即重启")}
            </Button>
          ) : null}
          {state.status === "unsupported" && !state.update ? <Text type="secondary">{state.message}</Text> : null}
        </div>
      </div>
    </div>
  );
}
