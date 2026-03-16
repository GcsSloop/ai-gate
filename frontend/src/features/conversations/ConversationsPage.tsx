import { useEffect, useState } from "react";

import type { AppLanguage, Translator } from "../../lib/i18n";
import { apiPath } from "../../lib/paths";

type Conversation = {
  id: number;
  client_id: string;
  state: string;
};

type Run = {
  id: number;
  account_id: number;
  status: string;
  stream_offset: number;
};

function formatRunStatus(status: string, t: Translator) {
  const labels: Record<string, string> = {
    completed: t("已完成"),
    capacity_failed: t("额度不足"),
    rate_limited: t("被限流"),
    usage_limited: t("用量受限"),
    hard_failed: t("硬失败"),
    soft_failed: t("软失败"),
  };
  return labels[status] ?? status;
}

function formatConversationState(state: string, t: Translator) {
  const labels: Record<string, string> = {
    active: t("进行中"),
    done: t("已完成"),
  };
  return labels[state] ?? state;
}

type ConversationsPageProps = {
  language?: AppLanguage;
  t?: Translator;
};

export function ConversationsPage({ t = (value) => value }: ConversationsPageProps) {
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [runs, setRuns] = useState<Run[]>([]);

  useEffect(() => {
    void fetch(apiPath("/conversations?page=1&page_size=20"))
      .then((response) => response.json() as Promise<Conversation[]>)
      .then((items) => {
        setConversations(items);
        if (items[0]) {
          void fetch(apiPath(`/conversations/${items[0].id}/runs`))
            .then((response) => response.json() as Promise<Run[]>)
            .then(setRuns);
        }
      });
  }, []);

  return (
    <div className="page-grid">
      <section className="panel">
        <h2>{t("会话列表")}</h2>
        <ul className="account-list">
          {conversations.map((conversation) => (
            <li key={conversation.id} className="account-card">
              <strong>{conversation.client_id}</strong>
              <span>{formatConversationState(conversation.state, t)}</span>
            </li>
          ))}
        </ul>
      </section>
      <section className="panel">
        <h2>{t("切换链路")}</h2>
        <ul className="account-list">
          {runs.map((run) => (
            <li key={run.id} className="account-card">
              <strong>{formatRunStatus(run.status, t)}</strong>
              <span>{`${t("账户 #")}${run.account_id}`}</span>
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}
