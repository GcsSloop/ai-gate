import { useEffect, useState } from "react";

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

export function ConversationsPage() {
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [runs, setRuns] = useState<Run[]>([]);

  useEffect(() => {
    void fetch("/conversations?page=1&page_size=20")
      .then((response) => response.json() as Promise<Conversation[]>)
      .then((items) => {
        setConversations(items);
        if (items[0]) {
          void fetch(`/conversations/${items[0].id}/runs`)
            .then((response) => response.json() as Promise<Run[]>)
            .then(setRuns);
        }
      });
  }, []);

  return (
    <div className="page-grid">
      <section className="panel">
        <h2>Conversations</h2>
        <ul className="account-list">
          {conversations.map((conversation) => (
            <li key={conversation.id} className="account-card">
              <strong>{conversation.client_id}</strong>
              <span>{conversation.state}</span>
            </li>
          ))}
        </ul>
      </section>
      <section className="panel">
        <h2>Run chain</h2>
        <ul className="account-list">
          {runs.map((run) => (
            <li key={run.id} className="account-card">
              <strong>{run.status}</strong>
              <span>account #{run.account_id}</span>
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}
