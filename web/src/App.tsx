import { useEffect, useRef, useState } from "react";
import { Alert, Button, Card, Checkbox, ConfigProvider, Input, Progress, Radio, Space, Tag, Typography, Upload } from "antd";
import { ApiError, deleteAsset, errorMessage, getAssets, mixAssets, uploadAudio, uploadVideos } from "./api";
import type { Asset, MixResult } from "./types";
import { formatDuration } from "./utils/formatDuration";
import "./App.css";

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
  return <span className="media-poster">
    {url && !failed
      ? <img alt={`视频封面 ${name}`} src={url} onError={() => setFailed(true)}
          style={{ width: "100%", height: "100%", objectFit: "contain" }} />
      : <span role="img" aria-label={`视频封面占位 ${name}`} className="media-poster-placeholder">视频</span>}
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
  const [audioUploadProgress, setAudioUploadProgress] = useState<number | null>(null);
  const [audioDeleteError, setAudioDeleteError] = useState<string | null>(null);
  const [deletingAudioIds, setDeletingAudioIds] = useState<string[]>([]);
  const [clearingAudios, setClearingAudios] = useState(false);
  const [seed, setSeed] = useState("");
  const [mixing, setMixing] = useState(false);
  const [mixError, setMixError] = useState<string | null>(null);
  const [mixResult, setMixResult] = useState<MixResult | null>(null);
  const nextKey = useRef(0);
  const audioPending = useRef(false);
  const mixPending = useRef(false);
  const clearPending = useRef(false);
  const clearAudioPending = useRef(false);
  const deletingAudioPending = useRef(new Set<string>());
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
    setAudioUploadProgress(0);
    const controller = new AbortController();
    controllers.current.add(controller);
    uploadAudio(file, controller.signal, ({ percent }) => {
      if (!controller.signal.aborted) setAudioUploadProgress(percent);
    }).then(({ asset }) => {
      if (controller.signal.aborted) return;
      setAudios((current) => [...current.filter((item) => item.id !== asset.id), asset]);
      setSelectedAudioId(asset.id);
      setAudioUpload(null);
    }).catch((error: unknown) => {
      if (!controller.signal.aborted) setAudioUpload({ name: file.name, status: "failed", error: failure(error) });
    }).finally(() => {
      controllers.current.delete(controller);
      if (!controller.signal.aborted) setAudioUploadProgress(null);
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
      await deleteAsset(id, controller.signal);
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
          await deleteAsset(id, controller.signal);
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

  async function deleteAudio(id: string) {
    if (mixing || audioPending.current || clearAudioPending.current || deletingAudioPending.current.has(id)) return;
    deletingAudioPending.current.add(id);
    setDeletingAudioIds((current) => [...current, id]);
    setAudioDeleteError(null);
    const controller = new AbortController();
    controllers.current.add(controller);
    try {
      await deleteAsset(id, controller.signal);
      if (controller.signal.aborted) return;
      setAudios((current) => current.filter((item) => item.id !== id));
      setSelectedAudioId((current) => current === id ? null : current);
      setMixResult(null);
      setMixError(null);
    } catch (error) {
      if (!controller.signal.aborted) setAudioDeleteError(failure(error));
    } finally {
      controllers.current.delete(controller);
      deletingAudioPending.current.delete(id);
      if (!controller.signal.aborted) setDeletingAudioIds((current) => current.filter((pending) => pending !== id));
    }
  }

  async function clearAudios() {
    if (clearAudioPending.current || mixing || audioPending.current || deletingAudioPending.current.size > 0 ||
      (audios.length === 0 && audioUpload?.status !== "failed")) return;
    clearAudioPending.current = true;
    setClearingAudios(true);
    setAudioDeleteError(null);
    const ids = [...new Set(audios.map((item) => item.id))];
    const controller = new AbortController();
    controllers.current.add(controller);
    try {
      for (const id of ids) {
        try {
          await deleteAsset(id, controller.signal);
        } catch (error) {
          if (!(error instanceof ApiError && error.code === "ASSET_NOT_FOUND")) throw error;
        }
        if (controller.signal.aborted) return;
        setAudios((current) => current.filter((item) => item.id !== id));
        setSelectedAudioId((current) => current === id ? null : current);
        setMixResult(null);
        setMixError(null);
      }
      setAudios([]);
      setSelectedAudioId(null);
      setAudioUpload(null);
      setMixResult(null);
      setMixError(null);
      setAudioDeleteError(null);
    } catch (error) {
      if (!controller.signal.aborted) setAudioDeleteError(failure(error));
    } finally {
      controllers.current.delete(controller);
      clearAudioPending.current = false;
      if (!controller.signal.aborted) setClearingAudios(false);
    }
  }

  async function submitMix() {
    if (mixPending.current || clearPending.current || clearAudioPending.current ||
      deletingAudioPending.current.size > 0 || audioPending.current) return;
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
  const uploadingAudio = audioUpload?.status === "uploading";
  const mixDisabled = !canMix || mixing || clearingVideos || deletingVideoIds.length > 0 ||
    clearingAudios || deletingAudioIds.length > 0 || uploadingAudio;
  const visibleProgress = videoUploadProgress.at(-1);

  return (
    <ConfigProvider theme={{ token: { borderRadius: 10, colorPrimary: "#315f9c" } }}>
      <main className="app-shell">
        <header className="app-header">
          <Typography.Title level={1}>ClipWeaver</Typography.Title>
          <Typography.Text type="secondary">本地视频混剪工具</Typography.Text>
        </header>
        {loadError && <Alert type="error" showIcon message={`素材加载失败：${loadError}`} className="section-alert" />}
        <div className="app-sections">
          <Card title="视频素材" className="section-card">
            <Upload.Dragger aria-label="上传视频素材" className="video-dropzone" multiple accept="video/*" showUploadList={false}
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
              <Typography.Text strong>点击或拖入多个视频文件</Typography.Text>
              <Typography.Text type="secondary" className="upload-hint">支持批量上传，服务端会校验媒体并生成封面</Typography.Text>
            </Upload.Dragger>
            {visibleProgress && <div role="status" className="upload-progress">
              <Typography.Text>{visibleProgress.percent === 100 ? "上传完成，处理中" : `上传中 ${visibleProgress.percent}%`}</Typography.Text>
              <Progress percent={visibleProgress.percent} size="small" />
            </div>}
            {videoUploadError && <Alert type="error" showIcon message={videoUploadError} className="section-alert" />}
            {videoDeleteError && <Alert type="error" showIcon message={videoDeleteError} className="section-alert" />}
            <div className="section-toolbar video-toolbar">
              <Typography.Text className="selection-count">已选 {selectedVideoIds.length} 个视频</Typography.Text>
              <div className="toolbar-actions">
                <Button size="small" disabled={selectedVideoIds.length === 0 || mixing} onClick={() => {
                  setSelectedVideoIds([]);
                  setMixResult(null);
                  setMixError(null);
                }}>重置选择</Button>
                <Button size="small" danger loading={clearingVideos}
                  disabled={videos.length === 0 || mixing || uploadingVideo || deletingVideoIds.length > 0 || clearingVideos}
                  onClick={() => void clearVideos()}>清空视频</Button>
              </div>
            </div>
            <ul aria-label="视频素材列表" className="media-list">
              {videos.map((row) => <li key={row.localKey} className="media-row">
                <VideoPoster name={row.name} url={row.asset?.posterUrl} />
                <div className="media-content">
                  <div className="media-primary">
                    {row.status === "ready" && row.asset
                      ? <Checkbox aria-label={`选择视频 ${row.name}`} checked={selectedVideoIds.includes(row.asset.id)}
                        onChange={(event) => setSelectedVideoIds((current) => event.target.checked
                          ? [...new Set([...current, row.asset!.id])]
                          : current.filter((id) => id !== row.asset!.id))} />
                      : null}
                    <Typography.Text strong className="media-name">{row.name}</Typography.Text>
                  </div>
                  <div className="media-meta">
                    <Tag className="status-tag" color={row.status === "ready" ? "success" : row.status === "failed" ? "error" : "processing"}>{row.status}</Tag>
                    {row.asset && <Typography.Text type="secondary">{formatDuration(row.asset.durationUs)}</Typography.Text>}
                  </div>
                  {row.error && <Typography.Text type="danger" role="alert" className="media-error">{row.error}</Typography.Text>}
                </div>
                {row.status === "ready" && row.asset && <Button type="text" size="small" danger className="media-action"
                  aria-label={`删除视频 ${row.name}`} disabled={mixing || clearingVideos || uploadingVideo || deletingVideoIds.includes(row.asset.id)}
                  onClick={() => void deleteVideo(row.asset!.id)}>删除</Button>}
                {row.status === "failed" && <Button type="text" size="small" danger className="media-action" aria-label={`移除视频 ${row.name}`}
                  disabled={mixing || clearingVideos} onClick={() => setVideos((current) => current.filter((item) => item.localKey !== row.localKey))}>移除</Button>}
              </li>)}
            </ul>
          </Card>
          <Card title="口播音频" className="section-card">
            <div className="section-toolbar audio-toolbar">
              <Upload accept="audio/*" showUploadList={false} disabled={uploadingAudio || clearingAudios}
                beforeUpload={(file, fileList) => {
                  if (file === fileList[0] && !audioPending.current) {
                    audioPending.current = true;
                    beginAudioUpload(file);
                  }
                  return false;
                }}>
                <Button disabled={uploadingAudio || clearingAudios}>选择口播音频文件</Button>
              </Upload>
              <Button size="small" danger loading={clearingAudios}
                disabled={(audios.length === 0 && audioUpload?.status !== "failed") || mixing || uploadingAudio ||
                  deletingAudioIds.length > 0 || clearingAudios}
                onClick={() => void clearAudios()}>清空口播</Button>
            </div>
            {audioUploadProgress !== null && <div role="status" className="upload-progress">
              <Typography.Text>{audioUploadProgress === 100 ? "上传完成，处理中" : `上传中 ${audioUploadProgress}%`}</Typography.Text>
              <Progress percent={audioUploadProgress} size="small" />
            </div>}
            {audioUpload && <div className="upload-message">
              <Typography.Text className="media-name">{audioUpload.name}</Typography.Text>
              <Tag className="status-tag" color={audioUpload.status === "failed" ? "error" : "processing"}>{audioUpload.status}</Tag>
              {audioUpload.error && <Typography.Text type="danger" role="alert">：{audioUpload.error}</Typography.Text>}
            </div>}
            {audioDeleteError && <Alert type="error" showIcon message={audioDeleteError} className="section-alert" />}
            <ul aria-label="口播音频列表" className="media-list">
              {audios.map((item) => <li key={item.id} className="media-row audio-row">
                <Radio aria-label={`选择口播 ${item.name}`} checked={selectedAudioId === item.id}
                  onChange={() => setSelectedAudioId(item.id)} />
                <div className="media-content">
                  <Typography.Text strong className="media-name">{item.name}</Typography.Text>
                  <div className="media-meta">
                    <Tag className="status-tag" color="success">ready</Tag>
                    <Typography.Text type="secondary">{formatDuration(item.durationUs)}</Typography.Text>
                  </div>
                </div>
                <Button type="text" size="small" danger className="media-action" aria-label={`删除口播 ${item.name}`}
                  disabled={mixing || uploadingAudio || clearingAudios || deletingAudioIds.includes(item.id)}
                  onClick={() => void deleteAudio(item.id)}>删除</Button>
              </li>)}
            </ul>
          </Card>
          <Card title="混剪操作" className="section-card">
            <div className="mix-controls">
              <label htmlFor="mix-seed">随机种子（可选）</label>
              <Input id="mix-seed" value={seed} onChange={(event) => setSeed(event.target.value)}
                placeholder="留空由服务端生成" />
              <Space wrap>
                <Button type="primary" size="large" disabled={mixDisabled} onClick={submitMix}>开始混剪</Button>
                {mixResult && <Button disabled={mixDisabled} onClick={submitMix}>重新制作</Button>}
              </Space>
              {mixing && <Typography.Text role="status">制作中</Typography.Text>}
              {mixError && <Alert type="error" showIcon message={mixError} />}
            </div>
          </Card>
          {mixResult && <Card title="成片结果" className="section-card">
            <div className="result-content">
              <video aria-label="成片预览" controls src={mixResult.previewUrl} className="result-preview" />
              <div className="result-meta">
                <Typography.Text type="secondary">时长：{formatDuration(mixResult.durationUs)}</Typography.Text>
                <Typography.Text type="secondary">本次种子：{mixResult.seed}</Typography.Text>
              </div>
              <Button type="primary" href={mixResult.downloadUrl} download>下载 MP4</Button>
            </div>
          </Card>}
        </div>
      </main>
    </ConfigProvider>
  );
}
