import { useEffect, useRef, useState } from "react";
import { Alert, Button, Card, Checkbox, ConfigProvider, Radio, Space, Typography, Upload } from "antd";
import { errorMessage, getAssets, uploadAudio, uploadVideos } from "./api";
import type { Asset } from "./types";
import { formatDuration } from "./utils/formatDuration";

type VideoRow = {
  localKey: number;
  name: string;
  status: "uploading" | "ready" | "failed";
  asset?: Asset;
  error?: string;
};

type AudioUpload = { name: string; status: "uploading" | "failed"; error?: string };

function failure(error: unknown): string {
  return error instanceof Error ? error.message : "请求失败，请稍后重试";
}

export function App() {
  const [videos, setVideos] = useState<VideoRow[]>([]);
  const [audios, setAudios] = useState<Asset[]>([]);
  const [selectedVideoIds, setSelectedVideoIds] = useState<string[]>([]);
  const [selectedAudioId, setSelectedAudioId] = useState<string | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [audioUpload, setAudioUpload] = useState<AudioUpload | null>(null);
  const nextKey = useRef(0);
  const audioPending = useRef(false);
  const controllers = useRef(new Set<AbortController>());

  useEffect(() => {
    const controller = new AbortController();
    controllers.current.add(controller);
    getAssets(controller.signal).then((items) => {
      if (controller.signal.aborted) return;
      setVideos((current) => {
        const known = new Set(current.flatMap((row) => row.asset ? [row.asset.id] : []));
        const restored = items.filter((item) => item.kind === "video" && !known.has(item.id))
          .map((item) => ({ localKey: ++nextKey.current, name: item.name, status: "ready" as const, asset: item }));
        return [...restored, ...current];
      });
      setAudios((current) => {
        const known = new Set(current.map((item) => item.id));
        return [...items.filter((item) => item.kind === "audio" && !known.has(item.id)), ...current];
      });
    }).catch((error: unknown) => {
      if (!controller.signal.aborted) setLoadError(failure(error));
    }).finally(() => controllers.current.delete(controller));
    return () => {
      for (const pending of controllers.current) pending.abort();
      controllers.current.clear();
      audioPending.current = false;
    };
  }, []);

  function beginVideoUpload(files: File[]) {
    const rows = files.map((file) => ({ localKey: ++nextKey.current, name: file.name, status: "uploading" as const }));
    const keys = new Set(rows.map((row) => row.localKey));
    setVideos((current) => [...current, ...rows]);
    const controller = new AbortController();
    controllers.current.add(controller);
    uploadVideos(files, controller.signal).then((items) => {
      if (controller.signal.aborted) return;
      const readyIds = items.flatMap((item) => item.status === "ready" ? [item.asset.id] : []);
      setVideos((current) => current
        .filter((row) => keys.has(row.localKey) || !row.asset || !readyIds.includes(row.asset.id))
        .map((row) => {
          const index = rows.findIndex((entry) => entry.localKey === row.localKey);
          if (index < 0) return row;
          const item = items[index];
          return item.status === "ready"
            ? { ...row, status: "ready" as const, asset: item.asset }
            : { ...row, status: "failed" as const, error: errorMessage(item.error.code) };
        }));
      setSelectedVideoIds((current) => [...new Set([...current, ...readyIds])]);
    }).catch((error: unknown) => {
      if (controller.signal.aborted) return;
      setVideos((current) => current.map((row) => keys.has(row.localKey)
        ? { ...row, status: "failed", error: failure(error) } : row));
    }).finally(() => controllers.current.delete(controller));
  }

  function beginAudioUpload(file: File) {
    setAudioUpload({ name: file.name, status: "uploading" });
    const controller = new AbortController();
    controllers.current.add(controller);
    uploadAudio(file, controller.signal).then(({ asset }) => {
      if (controller.signal.aborted) return;
      setAudios((current) => [...current.filter((item) => item.id !== asset.id), asset]);
      setSelectedAudioId(asset.id);
      setAudioUpload(null);
    }).catch((error: unknown) => {
      if (!controller.signal.aborted) setAudioUpload({ name: file.name, status: "failed", error: failure(error) });
    }).finally(() => {
      controllers.current.delete(controller);
      audioPending.current = false;
    });
  }

  return (
    <ConfigProvider>
      <main style={{ maxWidth: 760, margin: "40px auto", padding: "0 24px" }}>
        <Typography.Title>ClipWeaver</Typography.Title>
        {loadError && <Alert type="error" showIcon message={`素材加载失败：${loadError}`} style={{ marginBottom: 16 }} />}
        <Space direction="vertical" size="large" style={{ width: "100%" }}>
          <Card title="视频素材">
            <Upload.Dragger aria-label="上传视频素材" multiple accept="video/*" showUploadList={false} beforeUpload={(file, fileList) => {
              if (file === fileList[0]) beginVideoUpload(fileList);
              return false;
            }}>
              <p>点击或拖入多个视频文件</p>
            </Upload.Dragger>
            <Typography.Paragraph style={{ marginTop: 16 }}>已选 {selectedVideoIds.length} 个视频</Typography.Paragraph>
            <ul aria-label="视频素材列表">
              {videos.map((row) => <li key={row.localKey}>
                <Space wrap>
                  {row.status === "ready" && row.asset
                    ? <Checkbox aria-label={`选择视频 ${row.name}`} checked={selectedVideoIds.includes(row.asset.id)}
                        onChange={(event) => setSelectedVideoIds((current) => event.target.checked
                          ? [...new Set([...current, row.asset!.id])]
                          : current.filter((id) => id !== row.asset!.id))} />
                    : null}
                  <span>{row.name}</span>
                  <span>{row.status}</span>
                  {row.asset && <span>{formatDuration(row.asset.durationUs)}</span>}
                  {row.error && <span role="alert">{row.error}</span>}
                </Space>
              </li>)}
            </ul>
          </Card>
          <Card title="口播音频">
            <Upload accept="audio/*" showUploadList={false} disabled={audioUpload?.status === "uploading"}
              beforeUpload={(file, fileList) => {
                if (file === fileList[0] && !audioPending.current) {
                  audioPending.current = true;
                  beginAudioUpload(file);
                }
                return false;
              }}>
              <Button disabled={audioUpload?.status === "uploading"}>选择口播音频文件</Button>
            </Upload>
            {audioUpload && <p>{audioUpload.name} · {audioUpload.status}
              {audioUpload.error && <span role="alert">：{audioUpload.error}</span>}</p>}
            <ul aria-label="口播音频列表">
              {audios.map((item) => <li key={item.id}>
                <Radio aria-label={`选择口播 ${item.name}`} checked={selectedAudioId === item.id}
                  onChange={() => setSelectedAudioId(item.id)}>
                  {item.name} · ready · {formatDuration(item.durationUs)}
                </Radio>
              </li>)}
            </ul>
          </Card>
        </Space>
      </main>
    </ConfigProvider>
  );
}
