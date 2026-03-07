import { NavLink, Outlet, Route, Routes } from "react-router-dom";

import { AccountsPage } from "./features/accounts/AccountsPage";
import { ConversationsPage } from "./features/conversations/ConversationsPage";
import { MonitoringPage } from "./features/monitoring/MonitoringPage";
import { PolicyPage } from "./features/policy/PolicyPage";
import "./styles.css";

function Shell() {
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <p className="eyebrow">Gateway Control Plane</p>
        <h1>Codex Router</h1>
        <nav className="nav">
          <NavLink to="/accounts">Accounts</NavLink>
          <NavLink to="/policies">Policies</NavLink>
          <NavLink to="/monitoring">Monitoring</NavLink>
          <NavLink to="/conversations">Conversations</NavLink>
        </nav>
      </aside>
      <main className="content">
        <Outlet />
      </main>
    </div>
  );
}

function Placeholder({ title, description }: { title: string; description: string }) {
  return (
    <section className="panel">
      <h2>{title}</h2>
      <p>{description}</p>
    </section>
  );
}

export function App() {
  return (
    <Routes>
      <Route element={<Shell />}>
        <Route path="/" element={<AccountsPage />} />
        <Route path="/accounts" element={<AccountsPage />} />
        <Route path="/policies" element={<PolicyPage />} />
        <Route path="/monitoring" element={<MonitoringPage />} />
        <Route path="/conversations" element={<ConversationsPage />} />
      </Route>
    </Routes>
  );
}
