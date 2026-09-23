import { useEffect, useRef, useState } from "react";
import { Alert, Button, Card, Checkbox, ConfigProvider, Input, Radio, Space, Typography, Upload } from "antd";
import { errorMessage, getAssets, mixAssets, uploadAudio, uploadVideos } from "./api";
import type { Asset, MixResult } from "./types";
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
  const [seed, setSeed] = useState("");
  const [mixing, setMixing] = useState(false);
  const [mixError, setMixError] = useState<string | null>(null);
  const [mixResult, setMixResult] = useState<MixResult | null>(null);
  const nextKey = useRef(0);
  const audioPending = useRef(false);
  const mixPending = useRef(false);
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

  async function submitMix() {
    if (mixPending.current) return;
    const readyIds = new Set(videos.flatMap((row) => row.status === "ready" && row.asset ? [row.asset.id] : []));
    const videoIds = selectedVideoIds.filter((id) => readyIds.has(id));
    if (videoIds.length === 0 || !selectedAudioId || !audios.some((item) => item.id === selectedAudioId)) return;
    mixPending.current = true;
    setMixing(true);
    setMixError(null);
    const controller = new AbortController();
    controllers.current.add(controller);
    try {
      const result = await mixAssets({ videoIds, audioId: selectedAudioId, ...(seed === "" ? {} : { seed }) }, controller.signal);
      if (!controller.signal.aborted) setMixResult(result);
    } catch (error) {
      if (!controller.signal.aborted) setMixError(failure(error));
    } finally {
      controllers.current.delete(controller);
      mixPending.current = false;
      if (!controller.signal.aborted) setMixing(false);
    }
  }

  const canMix = selectedVideoIds.some((id) => videos.some((row) => row.status === "ready" && row.asset?.id === id)) &&
    selectedAudioId !== null && audios.some((item) => item.id === selectedAudioId);

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
          <Card title="混剪操作">
            <Space direction="vertical" style={{ width: "100%" }}>
              <label htmlFor="mix-seed">随机种子（可选）</label>
              <Input id="mix-seed" value={seed} onChange={(event) => setSeed(event.target.value)}
                placeholder="留空由服务端生成" />
              <Space>
                <Button type="primary" disabled={!canMix || mixing} onClick={submitMix}>开始混剪</Button>
                {mixResult && <Button disabled={!canMix || mixing} onClick={submitMix}>重新制作</Button>}
              </Space>
              {mixing && <Typography.Text role="status">制作中</Typography.Text>}
              {mixError && <Alert type="error" showIcon message={mixError} />}
            </Space>
          </Card>
          {mixResult && <Card title="成片结果">
            <Space direction="vertical" style={{ width: "100%" }}>
              <video aria-label="成片预览" controls src={mixResult.previewUrl} style={{ maxWidth: "100%", maxHeight: 480 }} />
              <Typography.Text>时长：{formatDuration(mixResult.durationUs)}</Typography.Text>
              <Typography.Text>本次种子：{mixResult.seed}</Typography.Text>
              <Button href={mixResult.downloadUrl} download>下载 MP4</Button>
            </Space>
          </Card>}
        </Space>
      </main>
    </ConfigProvider>
  );
}
