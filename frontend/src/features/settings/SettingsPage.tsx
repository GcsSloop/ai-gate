import { Button, Card, Modal, Space, Table, Tabs, Typography, message } from "antd";
import type { ColumnsType } from "antd/es/table";
import { useEffect, useState } from "react";

import { createCodexBackup, getCodexBackupFiles, listCodexBackups, restoreCodexBackup, type CodexBackupItem } from "../../lib/api";

const { Paragraph, Text, Title } = Typography;

function formatDateTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString("zh-CN", { hour12: false });
}

export function SettingsPage() {
  const [messageApi, contextHolder] = message.useMessage();
  const [items, setItems] = useState<CodexBackupItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [actioningID, setActioningID] = useState<string>("");
  const [viewingBackupID, setViewingBackupID] = useState<string>("");
  const [viewingFiles, setViewingFiles] = useState<Record<string, string>>({});

  useEffect(() => {
    void refreshBackups();
  }, []);

  async function refreshBackups() {
    setLoading(true);
    try {
      const backups = await listCodexBackups();
      setItems(backups);
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : "加载备份列表失败");
    } finally {
      setLoading(false);
    }
  }

  async function handleCreateBackup() {
    setActioningID("create");
    try {
      await createCodexBackup();
      await refreshBackups();
      void messageApi.success("备份已创建");
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : "创建备份失败");
    } finally {
      setActioningID("");
    }
  }

  async function handleRestore(item: CodexBackupItem) {
    setActioningID(item.backup_id);
    try {
      await restoreCodexBackup(item.backup_id);
      void messageApi.success("恢复完成，已自动创建 pre-restore 备份");
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : "恢复失败");
    } finally {
      setActioningID("");
    }
  }

  async function handleView(item: CodexBackupItem) {
    setActioningID(`view:${item.backup_id}`);
    try {
      const payload = await getCodexBackupFiles(item.backup_id);
      setViewingFiles(payload.files);
      setViewingBackupID(item.backup_id);
    } catch (error) {
      void messageApi.error(error instanceof Error ? error.message : "加载备份文件失败");
    } finally {
      setActioningID("");
    }
  }

  const columns: ColumnsType<CodexBackupItem> = [
    {
      title: "备份 ID",
      dataIndex: "backup_id",
    },
    {
      title: "创建时间",
      dataIndex: "created_at",
      render: (value: string) => formatDateTime(value),
    },
    {
      title: "操作",
      key: "action",
      width: 220,
      render: (_, record) => (
        <Space>
          <Button loading={actioningID === `view:${record.backup_id}`} onClick={() => void handleView(record)}>
            查看
          </Button>
          <Button loading={actioningID === record.backup_id} onClick={() => void handleRestore(record)}>
            恢复
          </Button>
        </Space>
      ),
    },
  ];

  return (
    <div className="dashboard-page">
      {contextHolder}
      <div className="dashboard-header">
        <div>
          <Title level={2} style={{ marginBottom: 8 }}>
            设置
          </Title>
          <Text type="secondary">管理 Codex 配置备份与恢复。</Text>
        </div>
      </div>

      <Card className="summary-card" variant="borderless">
        <Space direction="vertical" size={8}>
          <Title level={4} style={{ margin: 0 }}>
            Codex 备份与恢复
          </Title>
          <Paragraph style={{ marginBottom: 0 }}>
            一键备份会复制 <code>~/.codex/config.toml</code> 和 <code>~/.codex/auth.json</code> 到
            <code> ~/.aigate/data/codex/backup</code>。
          </Paragraph>
          <Paragraph type="secondary" style={{ marginBottom: 8 }}>
            一键恢复前会先把当前 <code>~/.codex</code> 下对应文件备份到 <code>~/.aigate/data/codex/pre-restore</code>。
          </Paragraph>
          <Button type="primary" onClick={() => void handleCreateBackup()} loading={actioningID === "create"}>
            一键备份
          </Button>
        </Space>
      </Card>

      <Card title="备份历史" className="summary-card" variant="borderless">
        <Table rowKey="backup_id" loading={loading} columns={columns} dataSource={items} pagination={false} />
      </Card>

      <Modal
        title="备份文件详情"
        open={!!viewingBackupID}
        onCancel={() => {
          setViewingBackupID("");
          setViewingFiles({});
        }}
        footer={null}
        width={900}
        destroyOnHidden
      >
        <Text type="secondary">备份 ID：{viewingBackupID}</Text>
        <Tabs
          style={{ marginTop: 12 }}
          items={["config.toml", "auth.json", "manifest.json"].map((name) => ({
            key: name,
            label: name,
            children: <pre className="test-output">{viewingFiles[name] || ""}</pre>,
          }))}
        />
      </Modal>
    </div>
  );
}
