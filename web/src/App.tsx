import { useEffect, useRef, useState } from "react";
import { Alert, Button, Card, Checkbox, ConfigProvider, Input, Progress, Radio, Space, Typography, Upload } from "antd";
import { ApiError, deleteVideoAsset, errorMessage, getAssets, mixAssets, uploadAudio, uploadVideos } from "./api";
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

function VideoPoster({ name, url }: { name: string; url?: string }) {
  const [failed, setFailed] = useState(false);
  return <span style={{ width: 96, height: 72, flex: "0 0 96px", display: "flex", alignItems: "center",
    justifyContent: "center", background: "#252b34", borderRadius: 4, overflow: "hidden" }}>
    {url && !failed
      ? <img alt={`视频封面 ${name}`} src={url} onError={() => setFailed(true)}
          style={{ width: "100%", height: "100%", objectFit: "contain" }} />
      : <span role="img" aria-label={`视频封面占位 ${name}`} style={{ color: "#c5cad2", fontSize: 12 }}>视频</span>}
  </span>;
}

export function App() {
  const [videos, setVideos] = useState<VideoRow[]>([]);
  const [audios, setAudios] = useState<Asset[]>([]);
  const [selectedVideoIds, setSelectedVideoIds] = useState<string[]>([]);
  const [selectedAudioId, setSelectedAudioId] = useState<string | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [videoUploadError, setVideoUploadError] = useState<string | null>(null);
  const [videoDeleteError, setVideoDeleteError] = useState<string | null>(null);
  const [deletingVideoIds, setDeletingVideoIds] = useState<string[]>([]);
  const [clearingVideos, setClearingVideos] = useState(false);
  const [videoUploadProgress, setVideoUploadProgress] = useState<{ key: number; percent: number }[]>([]);
  const [audioUpload, setAudioUpload] = useState<AudioUpload | null>(null);
  const [seed, setSeed] = useState("");
  const [mixing, setMixing] = useState(false);
  const [mixError, setMixError] = useState<string | null>(null);
  const [mixResult, setMixResult] = useState<MixResult | null>(null);
  const nextKey = useRef(0);
  const audioPending = useRef(false);
  const mixPending = useRef(false);
  const clearPending = useRef(false);
  const controllers = useRef(new Set<AbortController>());

  useEffect(() => {
    const controller = new AbortController();
    controllers.current.add(controller);
    getAssets(controller.signal).then((items) => {
      if (controller.signal.aborted) return;
      setVideos((current) => {
        const known = new Set(current.flatMap((row) => row.asset ? [row.asset.id] : []));
        const restored = items.filter((item) => {
          if (item.kind !== "video" || known.has(item.id)) return false;
          known.add(item.id);
          return true;
        }).map((item) => ({ localKey: ++nextKey.current, name: item.name, status: "ready" as const, asset: item }));
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
    if (clearPending.current) return;
    const rows = files.map((file) => ({ localKey: ++nextKey.current, name: file.name, status: "uploading" as const }));
    const keys = new Set(rows.map((row) => row.localKey));
    const batchKey = rows[0].localKey;
    setVideos((current) => [...current, ...rows]);
    setVideoUploadProgress((current) => [...current, { key: batchKey, percent: 0 }]);
    const controller = new AbortController();
    controllers.current.add(controller);
    uploadVideos(files, controller.signal, ({ percent }) => {
      if (!controller.signal.aborted) setVideoUploadProgress((current) => current.map((batch) =>
        batch.key === batchKey ? { ...batch, percent } : batch));
    }).then((items) => {
      if (controller.signal.aborted) return;
      const readyIds = items.flatMap((item) => item.status === "ready" ? [item.asset.id] : []);
      setVideos((current) => {
        const seen = new Set<string>();
        return current.map((row) => {
          const index = rows.findIndex((entry) => entry.localKey === row.localKey);
          if (index < 0) return row;
          const item = items[index];
          return item.status === "ready"
            ? { ...row, status: "ready" as const, asset: item.asset }
            : { ...row, status: "failed" as const, error: errorMessage(item.error.code) };
        }).filter((row) => {
          if (row.status !== "ready" || !row.asset) return true;
          if (seen.has(row.asset.id)) return false;
          seen.add(row.asset.id);
          return true;
        });
      });
      setSelectedVideoIds((current) => [...new Set([...current, ...readyIds])]);
    }).catch((error: unknown) => {
      if (controller.signal.aborted) return;
      setVideos((current) => current.map((row) => keys.has(row.localKey)
        ? { ...row, status: "failed", error: failure(error) } : row));
    }).finally(() => {
      controllers.current.delete(controller);
      if (!controller.signal.aborted) setVideoUploadProgress((current) => current.filter((batch) => batch.key !== batchKey));
    });
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

  async function deleteVideo(id: string) {
    if (mixing || clearingVideos || videos.some((row) => row.status === "uploading") || deletingVideoIds.includes(id)) return;
    setVideoDeleteError(null);
    setDeletingVideoIds((current) => [...current, id]);
    const controller = new AbortController();
    controllers.current.add(controller);
    try {
      await deleteVideoAsset(id, controller.signal);
      if (controller.signal.aborted) return;
      setVideos((current) => current.filter((row) => row.asset?.id !== id));
      setSelectedVideoIds((current) => current.filter((selected) => selected !== id));
      setMixResult(null);
      setMixError(null);
    } catch (error) {
      if (!controller.signal.aborted) setVideoDeleteError(failure(error));
    } finally {
      controllers.current.delete(controller);
      if (!controller.signal.aborted) setDeletingVideoIds((current) => current.filter((pending) => pending !== id));
    }
  }

  async function clearVideos() {
    if (clearPending.current || mixing || deletingVideoIds.length > 0 ||
      videos.length === 0 || videos.some((row) => row.status === "uploading")) return;
    clearPending.current = true;
    setClearingVideos(true);
    setVideoDeleteError(null);
    const ids = [...new Set(videos.flatMap((row) => row.status === "ready" && row.asset ? [row.asset.id] : []))];
    const controller = new AbortController();
    controllers.current.add(controller);
    try {
      for (const id of ids) {
        try {
          await deleteVideoAsset(id, controller.signal);
        } catch (error) {
          if (!(error instanceof ApiError && error.code === "ASSET_NOT_FOUND")) throw error;
        }
        if (controller.signal.aborted) return;
        setVideos((current) => current.filter((row) => row.asset?.id !== id));
        setSelectedVideoIds((current) => current.filter((selected) => selected !== id));
        setMixResult(null);
        setMixError(null);
      }
      setVideos([]);
      setSelectedVideoIds([]);
      setMixResult(null);
      setMixError(null);
      setVideoUploadError(null);
    } catch (error) {
      if (!controller.signal.aborted) setVideoDeleteError(failure(error));
    } finally {
      controllers.current.delete(controller);
      clearPending.current = false;
      if (!controller.signal.aborted) setClearingVideos(false);
    }
  }

  async function submitMix() {
    if (mixPending.current || clearPending.current) return;
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
  const uploadingVideo = videos.some((row) => row.status === "uploading");
  const visibleProgress = videoUploadProgress.at(-1);

  return (
    <ConfigProvider>
      <main style={{ maxWidth: 760, margin: "40px auto", padding: "0 24px" }}>
        <Typography.Title>ClipWeaver</Typography.Title>
        {loadError && <Alert type="error" showIcon message={`素材加载失败：${loadError}`} style={{ marginBottom: 16 }} />}
        <Space direction="vertical" size="large" style={{ width: "100%" }}>
          <Card title="视频素材">
            <Upload.Dragger aria-label="上传视频素材" multiple accept="video/*" showUploadList={false}
              disabled={clearingVideos} beforeUpload={(file, fileList) => {
              if (file === fileList[0]) {
                const accepted = fileList.filter((item) => !item.type || item.type.startsWith("video/"));
                const ignored = fileList.filter((item) => item.type && !item.type.startsWith("video/"));
                setVideoUploadError(ignored.length
                  ? `已忽略 ${ignored.length} 个非视频文件：${ignored.map((item) => item.name).join("、")}`
                  : null);
                if (accepted.length) beginVideoUpload(accepted);
              }
              return false;
            }}>
              <p>点击或拖入多个视频文件</p>
            </Upload.Dragger>
            {visibleProgress && <div role="status" style={{ marginTop: 16 }}>
              <Typography.Text>{visibleProgress.percent === 100 ? "上传完成，处理中" : `上传中 ${visibleProgress.percent}%`}</Typography.Text>
              <Progress percent={visibleProgress.percent} />
            </div>}
            {videoUploadError && <Alert type="error" showIcon message={videoUploadError} style={{ marginTop: 16 }} />}
            {videoDeleteError && <Alert type="error" showIcon message={videoDeleteError} style={{ marginTop: 16 }} />}
            <Space style={{ marginTop: 16, marginBottom: 16 }}>
              <Typography.Text>已选 {selectedVideoIds.length} 个视频</Typography.Text>
              <Button size="small" disabled={selectedVideoIds.length === 0 || mixing} onClick={() => {
                setSelectedVideoIds([]);
                setMixResult(null);
                setMixError(null);
              }}>重置选择</Button>
              <Button size="small" danger loading={clearingVideos}
                disabled={videos.length === 0 || mixing || uploadingVideo || deletingVideoIds.length > 0 || clearingVideos}
                onClick={() => void clearVideos()}>清空视频</Button>
            </Space>
            <ul aria-label="视频素材列表" style={{ listStyle: "none", padding: 0, margin: 0 }}>
              {videos.map((row) => <li key={row.localKey} style={{ display: "flex", alignItems: "center", gap: 12,
                padding: "8px 0", borderBottom: "1px solid #f0f0f0" }}>
                <VideoPoster name={row.name} url={row.asset?.posterUrl} />
                  {row.status === "ready" && row.asset
                    ? <Checkbox aria-label={`选择视频 ${row.name}`} checked={selectedVideoIds.includes(row.asset.id)}
                        onChange={(event) => setSelectedVideoIds((current) => event.target.checked
                          ? [...new Set([...current, row.asset!.id])]
                          : current.filter((id) => id !== row.asset!.id))} />
                    : null}
                  <span style={{ minWidth: 0, overflowWrap: "anywhere", flex: 1 }}>
                    <span>{row.name}</span> · {row.status === "failed"
                      ? <Typography.Text type="danger">{row.status}</Typography.Text>
                      : <span>{row.status}</span>}
                    {row.asset && <> · <span>{formatDuration(row.asset.durationUs)}</span></>}
                    {row.error && <><br /><Typography.Text type="danger" role="alert">{row.error}</Typography.Text></>}
                  </span>
                  {row.status === "ready" && row.asset && <Button size="small" danger
                    aria-label={`删除视频 ${row.name}`} disabled={mixing || clearingVideos || uploadingVideo || deletingVideoIds.includes(row.asset.id)}
                    onClick={() => void deleteVideo(row.asset!.id)}>删除</Button>}
                  {row.status === "failed" && <Button size="small" aria-label={`移除视频 ${row.name}`}
                    disabled={mixing || clearingVideos} onClick={() => setVideos((current) => current.filter((item) => item.localKey !== row.localKey))}>移除</Button>}
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
                <Button type="primary" disabled={!canMix || mixing || clearingVideos || deletingVideoIds.length > 0} onClick={submitMix}>开始混剪</Button>
                {mixResult && <Button disabled={!canMix || mixing || clearingVideos || deletingVideoIds.length > 0} onClick={submitMix}>重新制作</Button>}
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
