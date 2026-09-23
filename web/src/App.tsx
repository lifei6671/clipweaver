import { Card, ConfigProvider, Typography } from "antd";

export function App() {
  return (
    <ConfigProvider>
      <main style={{ maxWidth: 720, margin: "64px auto", padding: "0 24px" }}>
        <Card>
          <Typography.Title>ClipWeaver</Typography.Title>
          <Typography.Paragraph>工程骨架已就绪。</Typography.Paragraph>
        </Card>
      </main>
    </ConfigProvider>
  );
}
