import { App as AntApp, ConfigProvider } from "antd";

import { AccountsPage } from "./features/accounts/AccountsPage";
import "./styles.css";

export function App() {
  return (
    <ConfigProvider
      theme={{
        token: {
          colorPrimary: "#1677ff",
          borderRadius: 14,
          colorBgLayout: "#f4f7fb",
          colorBgContainer: "#ffffff",
          colorBorderSecondary: "#e8edf5",
        },
      }}
    >
      <AntApp>
        <AccountsPage />
      </AntApp>
    </ConfigProvider>
  );
}
