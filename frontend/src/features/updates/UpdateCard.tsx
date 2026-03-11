import { CloudDownloadOutlined, ReloadOutlined, SyncOutlined } from "@ant-design/icons";
import { Button, Card, Progress, Typography } from "antd";
import { useEffect, useMemo, useState } from "react";

import type { AppLanguage, Translator } from "../../lib/i18n";
import { createDesktopUpdateService, type DesktopUpdateInfo, type DesktopUpdateService, type DownloadProgress } from "./updateService";

const { Paragraph, Text } = Typography;

type UpdateCardProps = {
  autoCheckOnMount?: boolean;
  currentVersion: string;
  language: AppLanguage;
  t: Translator;
  service?: DesktopUpdateService;
};

type UpdateViewState =
  | { status: "idle"; message: string }
  | { status: "checking"; message: string }
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

export function UpdateCard({ autoCheckOnMount = true, currentVersion, language, t, service }: UpdateCardProps) {
  const updateService = useMemo(() => service ?? createDesktopUpdateService(), [service]);
  const [state, setState] = useState<UpdateViewState>({ status: "idle", message: t("检查 GitHub Release 中的最新版本。") });

  async function runCheck(silent = false) {
    setState({ status: "checking", message: t("正在检查更新…") });
    try {
      const result = await updateService.check(currentVersion);
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
      if (!silent) {
        return;
      }
    } catch (error) {
      setState({ status: "error", message: error instanceof Error ? error.message : t("检查更新失败") });
    }
  }

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
      setState({ status: "error", message: error instanceof Error ? error.message : t("安装更新失败") });
    }
  }

  useEffect(() => {
    if (!autoCheckOnMount) {
      return;
    }
    void runCheck(true);
  }, [autoCheckOnMount]);

  return (
    <Card className="settings-card update-card" variant="borderless">
      <div className="update-card-header">
        <div>
          <div className="settings-section-title">{t("应用更新")}</div>
          <div className="settings-section-description">{t("从 GitHub Release 检查、下载并安装最新桌面版本。")}</div>
        </div>
        <Button
          aria-label={t("检查更新")}
          icon={<SyncOutlined spin={state.status === "checking"} />}
          onClick={() => void runCheck()}
          disabled={state.status === "checking"}
        >
          {t("检查更新")}
        </Button>
      </div>

      <div className="update-card-body">
        <div className="about-meta-row">
          <span>{language === "en-US" ? "Current version" : "当前版本"}</span>
          <strong>{currentVersion}</strong>
        </div>
        <div className="about-meta-row">
          <span>{language === "en-US" ? "Status" : "状态"}</span>
          <strong className={`update-status-value${state.status === "checking" ? " is-checking" : ""}`}>{state.message}</strong>
        </div>

        {"update" in state && state.update ? (
          <>
            <div className="about-meta-row">
              <span>{state.status === "unsupported" ? (language === "en-US" ? "Latest version" : "最新版本") : language === "en-US" ? "Target version" : "目标版本"}</span>
              <strong>{state.update.version}</strong>
            </div>
            {state.update.date ? (
              <div className="about-meta-row">
                <span>{language === "en-US" ? "Published at" : "发布时间"}</span>
                <strong>{formatDate(state.update.date, language)}</strong>
              </div>
            ) : null}
            {state.update.body ? <Paragraph className="update-release-notes">{state.update.body}</Paragraph> : null}
          </>
        ) : null}

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
    </Card>
  );
}
