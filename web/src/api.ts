import type { Asset, AudioUploadItem, VideoUploadItem } from "./types";

const genericError = "请求失败，请稍后重试";

export function errorMessage(code: string): string {
  switch (code) {
    case "UPLOAD_TOO_LARGE": return "文件超过上传限制";
    case "INVALID_VIDEO": return "未检测到有效视频流";
    case "INVALID_AUDIO": return "未检测到有效音频流";
    case "INVALID_MEDIA": return "媒体文件无效";
    case "FFPROBE_FAILED": return "媒体探测失败，请检查文件";
    case "INVALID_REQUEST": return "上传请求无效";
    default: return genericError;
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
    typeof value.width === "number" && typeof value.height === "number";
}

async function request(url: string, init?: RequestInit): Promise<unknown> {
  let response: Response;
  try {
    response = await fetch(url, init);
  } catch (error) {
    if (error instanceof DOMException && error.name === "AbortError") throw error;
    throw new Error("网络连接失败，请稍后重试");
  }
  if (response.status === 413) throw new Error(errorMessage("UPLOAD_TOO_LARGE"));
  let body: unknown;
  try {
    body = await response.json();
  } catch {
    throw new Error(genericError);
  }
  if (!response.ok) {
    const code = record(body) && record(body.error) ? body.error.code : null;
    throw new Error(errorMessage(typeof code === "string" ? code : ""));
  }
  return body;
}

export async function getAssets(signal?: AbortSignal): Promise<Asset[]> {
  const body = await request("/api/assets", { signal });
  if (!record(body) || !Array.isArray(body.items) || !body.items.every(asset)) {
    throw new Error(genericError);
  }
  return body.items;
}

export async function uploadVideos(files: File[], signal?: AbortSignal): Promise<VideoUploadItem[]> {
  const form = new FormData();
  files.forEach((file) => form.append("files", file));
  const body = await request("/api/assets/videos", { method: "POST", body: form, signal });
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
