import type { Asset, AudioUploadItem, MixRequest, MixResult, VideoUploadItem } from "./types";

const genericError = "请求失败，请稍后重试";
const knownErrorCodes = new Set([
  "UPLOAD_TOO_LARGE", "INVALID_VIDEO", "INVALID_AUDIO", "INVALID_MEDIA", "FFPROBE_FAILED",
  "INVALID_REQUEST", "INVALID_ID", "INVALID_SEED", "NO_VIDEO_SELECTED", "AUDIO_REQUIRED",
  "ASSET_NOT_FOUND", "INSUFFICIENT_VIDEO_DURATION", "MIX_BUSY", "MIX_TIMEOUT",
  "FFMPEG_FAILED", "RENDER_VALIDATION_FAILED", "INTERNAL_ERROR",
]);

export function errorMessage(code: string): string {
  switch (code) {
    case "UPLOAD_TOO_LARGE": return "文件超过上传限制";
    case "INVALID_VIDEO": return "未检测到有效视频流";
    case "INVALID_AUDIO": return "未检测到有效音频流";
    case "INVALID_MEDIA": return "媒体文件无效";
    case "FFPROBE_FAILED": return "媒体探测失败，请检查文件";
    case "INVALID_REQUEST": return "上传请求无效";
    case "INVALID_SEED": return "随机种子无效，请输入 int64 十进制整数";
    case "NO_VIDEO_SELECTED": return "请先选择视频素材";
    case "AUDIO_REQUIRED": return "请先选择口播音频";
    case "ASSET_NOT_FOUND": return "所选素材不存在，请刷新后重试";
    case "INSUFFICIENT_VIDEO_DURATION": return "所选视频素材时长不足";
    case "MIX_BUSY": return "已有混剪正在制作，请稍后重试";
    case "MIX_TIMEOUT": return "混剪超时，请重试";
    case "FFMPEG_FAILED": return "视频渲染失败，请重试";
    case "RENDER_VALIDATION_FAILED": return "成片校验失败，请重试";
    default: return genericError;
  }
}

export class ApiError extends Error {
  constructor(readonly code: string, readonly details?: { missingDurationUs: number }) {
    const seconds = details && code === "INSUFFICIENT_VIDEO_DURATION"
      ? `，还缺少 ${Number((details.missingDurationUs / 1_000_000).toFixed(1))} 秒视频素材` : "";
    super(`${errorMessage(code)}${seconds}`);
  }
}

function record(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function asset(value: unknown): value is Asset {
  return record(value) && typeof value.id === "string" &&
    (value.kind === "video" || value.kind === "audio") &&
    typeof value.name === "string" && typeof value.durationUs === "number" &&
    Number.isFinite(value.durationUs) && value.durationUs > 0 &&
    typeof value.width === "number" && typeof value.height === "number" &&
    (value.posterUrl === undefined || (value.kind === "video" &&
      value.posterUrl === `/api/assets/${value.id}/poster`));
}

function parseResponse(status: number, text: string): unknown {
  if (status === 413) throw new Error(errorMessage("UPLOAD_TOO_LARGE"));
  let body: unknown;
  try {
    body = JSON.parse(text);
  } catch {
    throw new Error(genericError);
  }
  if (status < 200 || status >= 300) {
    const error = record(body) && record(body.error) ? body.error : null;
    const rawCode = error && typeof error.code === "string" ? error.code : "";
    const code = knownErrorCodes.has(rawCode) ? rawCode : "UNKNOWN_ERROR";
    const details = error && record(error.details) ? error.details : null;
    const missing = details?.missingDurationUs;
    throw new ApiError(code, code === "INSUFFICIENT_VIDEO_DURATION" &&
      typeof missing === "number" && Number.isSafeInteger(missing) && missing > 0
      ? { missingDurationUs: missing } : undefined);
  }
  return body;
}

async function request(url: string, init?: RequestInit): Promise<unknown> {
  let response: Response;
  try {
    response = await fetch(url, init);
  } catch (error) {
    if (error instanceof DOMException && error.name === "AbortError") throw error;
    throw new Error("网络连接失败，请稍后重试");
  }
  if (response.status === 413) return parseResponse(413, "");
  let body: string;
  try {
    body = await response.text();
  } catch {
    throw new Error(genericError);
  }
  return parseResponse(response.status, body);
}

export async function getAssets(signal?: AbortSignal): Promise<Asset[]> {
  const body = await request("/api/assets", { signal });
  if (!record(body) || !Array.isArray(body.items) || !body.items.every(asset)) {
    throw new Error(genericError);
  }
  return body.items;
}

export async function deleteVideoAsset(id: string, signal?: AbortSignal): Promise<void> {
  const body = await request(`/api/assets/${encodeURIComponent(id)}`, { method: "DELETE", signal });
  if (!record(body) || body.id !== id || body.deleted !== true) {
    throw new Error(genericError);
  }
}

export type UploadProgress = { loaded: number; total: number; percent: number };

export async function uploadVideos(files: File[], signal?: AbortSignal,
  onProgress?: (progress: UploadProgress) => void): Promise<VideoUploadItem[]> {
  if (signal?.aborted) throw new DOMException("The operation was aborted", "AbortError");
  const form = new FormData();
  files.forEach((file) => form.append("files", file));
  const body = await new Promise<unknown>((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    let lastPercent = 0;
    const abort = () => xhr.abort();
    const cleanup = () => signal?.removeEventListener("abort", abort);
    xhr.upload.onprogress = (event) => {
      if (!event.lengthComputable || event.total <= 0) return;
      const percent = Math.max(lastPercent, Math.min(100, Math.round(event.loaded / event.total * 100)));
      lastPercent = percent;
      onProgress?.({ loaded: event.loaded, total: event.total, percent });
    };
    xhr.onload = () => {
      cleanup();
      try { resolve(parseResponse(xhr.status, xhr.responseText)); }
      catch (error) { reject(error); }
    };
    xhr.onerror = () => { cleanup(); reject(new Error("网络连接失败，请稍后重试")); };
    xhr.onabort = () => { cleanup(); reject(new DOMException("The operation was aborted", "AbortError")); };
    signal?.addEventListener("abort", abort, { once: true });
    xhr.open("POST", "/api/assets/videos");
    xhr.send(form);
    if (signal?.aborted) abort();
  });
  if (!record(body) || !Array.isArray(body.items) || body.items.length !== files.length) {
    throw new Error(genericError);
  }
  return body.items.map((item: unknown): VideoUploadItem => {
    if (!record(item) || typeof item.filename !== "string") throw new Error(genericError);
    if (item.status === "ready" && asset(item.asset) && item.asset.kind === "video") {
      return { filename: item.filename, status: "ready", asset: item.asset };
    }
    if (item.status === "failed" && record(item.error) && typeof item.error.code === "string") {
      return { filename: item.filename, status: "failed", error: { code: item.error.code } };
    }
    throw new Error(genericError);
  });
}

export async function uploadAudio(file: File, signal?: AbortSignal): Promise<AudioUploadItem> {
  const form = new FormData();
  form.append("file", file);
  const body = await request("/api/assets/audio", { method: "POST", body: form, signal });
  if (!record(body) || body.status !== "ready" || typeof body.filename !== "string" ||
      !asset(body.asset) || body.asset.kind !== "audio") {
    throw new Error(genericError);
  }
  return { filename: body.filename, status: "ready", asset: body.asset };
}

export async function mixAssets(input: MixRequest, signal?: AbortSignal): Promise<MixResult> {
  const body = await request("/api/mixes", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
    signal,
  });
  if (!record(body) || typeof body.id !== "string" || body.id.length === 0 ||
      body.status !== "completed" || typeof body.seed !== "string" ||
      typeof body.durationUs !== "number" || !Number.isSafeInteger(body.durationUs) || body.durationUs <= 0 ||
      body.previewUrl !== `/api/mixes/${body.id}/file` ||
      body.downloadUrl !== `/api/mixes/${body.id}/download`) {
    throw new Error(genericError);
  }
  return body as MixResult;
}
