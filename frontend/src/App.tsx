import { NavLink, Outlet, Route, Routes } from "react-router-dom";

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
        <Route
          path="/"
          element={<Placeholder title="Accounts" description="Manage official auth and third-party API credentials." />}
        />
        <Route
          path="/accounts"
          element={<Placeholder title="Accounts" description="Manage official auth and third-party API credentials." />}
        />
        <Route
          path="/policies"
          element={<Placeholder title="Policies" description="Adjust routing order, cooldown rules, and safety budgets." />}
        />
        <Route
          path="/monitoring"
          element={<Placeholder title="Monitoring" description="Inspect balance, quota, cooldown, and routing health." />}
        />
        <Route
          path="/conversations"
          element={<Placeholder title="Conversations" description="Review active sessions and account failover chains." />}
        />
      </Route>
    </Routes>
  );
}
