import { useEffect, useState } from "react";

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

function formatRunStatus(status: string) {
  const labels: Record<string, string> = {
    completed: "已完成",
    capacity_failed: "额度不足",
    rate_limited: "被限流",
    hard_failed: "硬失败",
    soft_failed: "软失败",
  };
  return labels[status] ?? status;
}

function formatConversationState(state: string) {
  const labels: Record<string, string> = {
    active: "进行中",
    done: "已完成",
  };
  return labels[state] ?? state;
}

export function ConversationsPage() {
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
        <h2>会话列表</h2>
        <ul className="account-list">
          {conversations.map((conversation) => (
            <li key={conversation.id} className="account-card">
              <strong>{conversation.client_id}</strong>
              <span>{formatConversationState(conversation.state)}</span>
            </li>
          ))}
        </ul>
      </section>
      <section className="panel">
        <h2>切换链路</h2>
        <ul className="account-list">
          {runs.map((run) => (
            <li key={run.id} className="account-card">
              <strong>{formatRunStatus(run.status)}</strong>
              <span>账户 #{run.account_id}</span>
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}
