import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { App } from "./App";
import { ApiError, deleteVideoAsset, getAssets, mixAssets, uploadAudio, uploadVideos } from "./api";
import { formatDuration } from "./utils/formatDuration";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

class MockXHR {
  static instances: MockXHR[] = [];
  static autoRespond = true;
  upload = { onprogress: null as ((event: ProgressEvent) => void) | null };
  onload: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onabort: (() => void) | null = null;
  status = 0;
  responseText = "";
  method = "";
  url = "";
  body?: FormData;
  aborted = false;
  private controller = new AbortController();

  constructor() { MockXHR.instances.push(this); }
  open(method: string, url: string) { this.method = method; this.url = url; }
  send(body: FormData) {
    this.body = body;
    if (!MockXHR.autoRespond) return;
    Promise.resolve(fetch(this.url, { method: this.method, body, signal: this.controller.signal }))
      .then(async (response) => {
        const text = await response.text();
        if (!this.aborted) this.respond(response.status, text);
      }).catch(() => { if (!this.aborted) this.onerror?.(); });
  }
  abort() {
    this.aborted = true;
    this.controller.abort();
    this.onabort?.();
  }
  respond(status: number, body: unknown) {
    this.status = status;
    this.responseText = typeof body === "string" ? body : JSON.stringify(body);
    this.onload?.();
  }
  progress(loaded: number, total: number, lengthComputable = true) {
    this.upload.onprogress?.({ loaded, total, lengthComputable } as ProgressEvent);
  }
}

beforeEach(() => {
  MockXHR.instances = [];
  MockXHR.autoRespond = true;
  vi.stubGlobal("XMLHttpRequest", MockXHR);
});

const video = (id: string, name = `${id}.mp4`, durationUs = 9_700_000) =>
  ({ id, name, durationUs, kind: "video", width: 1920, height: 1080 });
const audio = (id: string, name = `${id}.m4a`, durationUs = 4_500_000) =>
  ({ id, name, durationUs, kind: "audio", width: 0, height: 0 });
const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

function fileInput(container: HTMLElement, index: number) {
  const inputs = container.querySelectorAll<HTMLInputElement>('input[type="file"]');
  expect(inputs).toHaveLength(2);
  return inputs[index];
}

test("restores assets and server durations without restoring selections", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ items: [video("v1"), audio("a1"), audio("a2")] })));
  const { container } = render(<App />);
  expect(screen.getByRole("heading", { name: "ClipWeaver" })).toBeTruthy();
  expect(await screen.findByText("v1.mp4")).toBeTruthy();
  expect(screen.getByText("9.7 秒")).toBeTruthy();
  expect(screen.getByRole("img", { name: "视频封面占位 v1.mp4" })).toBeTruthy();
  expect(screen.getByText(/a1.m4a/)).toBeTruthy();
  expect(screen.getByText(/a2.m4a/)).toBeTruthy();
  expect(screen.getByText("已选 0 个视频")).toBeTruthy();
  expect(screen.getByRole<HTMLInputElement>("checkbox", { name: "选择视频 v1.mp4" }).checked).toBe(false);
  expect(screen.getAllByRole<HTMLInputElement>("radio").every((radio) => !radio.checked)).toBe(true);
  expect(fileInput(container, 0).multiple).toBe(true);
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "开始混剪" }).disabled).toBe(true);
  expect(container.querySelector("video")).toBeNull();
  expect(screen.queryByText("下载")).toBeNull();
});

test("restores duplicate video IDs only once", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ items: [video("v1"), video("v1")] })));
  render(<App />);
  await screen.findByText("v1.mp4");
  expect(within(screen.getByRole("list", { name: "视频素材列表" })).getAllByRole("listitem")).toHaveLength(1);
});

test("deleting a selected video clears stale mix result and error", async () => {
  const fetchMock = vi.fn().mockResolvedValueOnce(json({ items: [video("v1"), audio("a1")] }))
    .mockResolvedValueOnce(json(completed("first")))
    .mockResolvedValueOnce(json({ error: { code: "MIX_TIMEOUT" } }, 504))
    .mockResolvedValueOnce(json({ id: "v1", deleted: true }));
  vi.stubGlobal("fetch", fetchMock);
  render(<App />);
  await selectMixAssets();
  expect(screen.getByRole("button", { name: "删除视频 v1.mp4" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "开始混剪" }));
  await screen.findByLabelText("成片预览");
  fireEvent.click(screen.getByRole("button", { name: "重新制作" }));
  await screen.findByText("混剪超时，请重试");
  fireEvent.click(screen.getByRole("button", { name: "删除视频 v1.mp4" }));
  await waitFor(() => expect(screen.queryByRole("checkbox", { name: "选择视频 v1.mp4" })).toBeNull());
  expect(fetchMock.mock.calls[3][0]).toBe("/api/assets/v1");
  expect(fetchMock.mock.calls[3][1].method).toBe("DELETE");
  expect(screen.getByText("已选 0 个视频")).toBeTruthy();
  expect(screen.queryByLabelText("成片预览")).toBeNull();
  expect(screen.queryByText("混剪超时，请重试")).toBeNull();
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "开始混剪" }).disabled).toBe(true);
});

test("failed video rows remove locally without DELETE", async () => {
  const fetchMock = vi.fn().mockResolvedValueOnce(json({ items: [] })).mockResolvedValueOnce(json({ items: [
    { filename: "bad.mp4", status: "failed", error: { code: "INVALID_VIDEO" } },
  ] }));
  vi.stubGlobal("fetch", fetchMock);
  const { container } = render(<App />);
  fireEvent.change(fileInput(container, 0), { target: { files: [new File(["bad"], "bad.mp4")] } });
  fireEvent.click(await screen.findByRole("button", { name: "移除视频 bad.mp4" }));
  expect(screen.queryByText("bad.mp4")).toBeNull();
  expect(fetchMock).toHaveBeenCalledTimes(2);
});

test("failed DELETE retains video and shows a card error", async () => {
  const fetchMock = vi.fn().mockResolvedValueOnce(json({ items: [video("v1")] }))
    .mockResolvedValueOnce(json({ error: { code: "INTERNAL_ERROR", message: "raw /path" } }, 500));
  vi.stubGlobal("fetch", fetchMock);
  render(<App />);
  await screen.findByRole("button", { name: "删除视频 v1.mp4" });
  fireEvent.click(screen.getByRole("checkbox", { name: "选择视频 v1.mp4" }));
  fireEvent.click(screen.getByRole("button", { name: "删除视频 v1.mp4" }));
  const alert = await screen.findByRole("alert");
  expect(alert.classList.contains("ant-alert-error")).toBe(true);
  expect(alert.textContent).toContain("请求失败，请稍后重试");
  expect(alert.textContent).not.toContain("raw /path");
  expect(screen.getByRole("checkbox", { name: "选择视频 v1.mp4" })).toBeTruthy();
  expect(screen.getByText("已选 1 个视频")).toBeTruthy();
});

test("reset selection clears mix state while preserving videos, audio and seed", async () => {
  const fetchMock = vi.fn().mockResolvedValueOnce(json({ items: [video("v1"), video("v2"), audio("a1")] }))
    .mockResolvedValueOnce(json(completed("first")))
    .mockResolvedValueOnce(json({ error: { code: "MIX_TIMEOUT" } }, 504));
  vi.stubGlobal("fetch", fetchMock);
  render(<App />);
  await screen.findByRole("button", { name: "删除视频 v1.mp4" });
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "重置选择" }).disabled).toBe(true);
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "清空视频" }).disabled).toBe(false);
  fireEvent.click(screen.getByRole("checkbox", { name: "选择视频 v1.mp4" }));
  fireEvent.click(screen.getByRole("radio", { name: "选择口播 a1.m4a" }));
  fireEvent.change(screen.getByRole("textbox", { name: "随机种子（可选）" }), { target: { value: "42" } });
  fireEvent.click(screen.getByRole("button", { name: "开始混剪" }));
  await screen.findByLabelText("成片预览");
  fireEvent.click(screen.getByRole("button", { name: "重新制作" }));
  await screen.findByText("混剪超时，请重试");
  fireEvent.click(screen.getByRole("button", { name: "重置选择" }));
  expect(screen.getByText("已选 0 个视频")).toBeTruthy();
  expect(screen.getByRole<HTMLInputElement>("checkbox", { name: "选择视频 v1.mp4" }).checked).toBe(false);
  expect(screen.getByRole("checkbox", { name: "选择视频 v2.mp4" })).toBeTruthy();
  expect(screen.getByRole<HTMLInputElement>("radio", { name: "选择口播 a1.m4a" }).checked).toBe(true);
  expect(screen.getByRole<HTMLInputElement>("textbox", { name: "随机种子（可选）" }).value).toBe("42");
  expect(screen.queryByLabelText("成片预览")).toBeNull();
  expect(screen.queryByText("混剪超时，请重试")).toBeNull();
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "开始混剪" }).disabled).toBe(true);
  expect(fetchMock).toHaveBeenCalledTimes(3);
});

test("clear video deletes every visible ready asset and local failed rows while preserving audio and seed", async () => {
  const fetchMock = vi.fn().mockResolvedValueOnce(json({ items: [video("v1"), video("v2"), audio("a1")] }))
    .mockResolvedValueOnce(json(completed("first")))
    .mockResolvedValueOnce(json({ error: { code: "MIX_TIMEOUT" } }, 504))
    .mockResolvedValueOnce(json({ items: [{ filename: "bad.mp4", status: "failed", error: { code: "INVALID_VIDEO" } }] }))
    .mockResolvedValueOnce(json({ id: "v1", deleted: true }))
    .mockResolvedValueOnce(json({ id: "v2", deleted: true }));
  vi.stubGlobal("fetch", fetchMock);
  const { container } = render(<App />);
  await screen.findByRole("button", { name: "删除视频 v2.mp4" });
  fireEvent.click(screen.getByRole("checkbox", { name: "选择视频 v1.mp4" }));
  fireEvent.click(screen.getByRole("radio", { name: "选择口播 a1.m4a" }));
  fireEvent.change(screen.getByRole("textbox", { name: "随机种子（可选）" }), { target: { value: "42" } });
  fireEvent.click(screen.getByRole("button", { name: "开始混剪" }));
  await screen.findByLabelText("成片预览");
  fireEvent.click(screen.getByRole("button", { name: "重新制作" }));
  await screen.findByText("混剪超时，请重试");
  fireEvent.change(fileInput(container, 0), { target: { files: [new File(["bad"], "bad.mp4")] } });
  await screen.findByRole("button", { name: "移除视频 bad.mp4" });
  fireEvent.click(screen.getByRole("button", { name: "清空视频" }));
  await waitFor(() => expect(within(screen.getByRole("list", { name: "视频素材列表" })).queryAllByRole("listitem")).toHaveLength(0));
  expect(fetchMock.mock.calls.slice(4).map(([url, init]) => [url, init.method])).toEqual([
    ["/api/assets/v1", "DELETE"], ["/api/assets/v2", "DELETE"],
  ]);
  expect(screen.getByText("已选 0 个视频")).toBeTruthy();
  expect(screen.queryByLabelText("成片预览")).toBeNull();
  expect(screen.queryByText("混剪超时，请重试")).toBeNull();
  expect(screen.getByRole<HTMLInputElement>("radio", { name: "选择口播 a1.m4a" }).checked).toBe(true);
  expect(screen.getByRole<HTMLInputElement>("textbox", { name: "随机种子（可选）" }).value).toBe("42");
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "清空视频" }).disabled).toBe(true);
});

test("clear video treats a missing asset as deleted and continues with the next video", async () => {
  const fetchMock = vi.fn().mockResolvedValueOnce(json({ items: [video("v1"), video("v2")] }))
    .mockResolvedValueOnce(json({ error: { code: "ASSET_NOT_FOUND" } }, 404))
    .mockResolvedValueOnce(json({ id: "v2", deleted: true }));
  vi.stubGlobal("fetch", fetchMock);
  render(<App />);
  await screen.findByRole("button", { name: "删除视频 v2.mp4" });
  fireEvent.click(screen.getByRole("checkbox", { name: "选择视频 v1.mp4" }));
  fireEvent.click(screen.getByRole("button", { name: "清空视频" }));
  await waitFor(() => expect(within(screen.getByRole("list", { name: "视频素材列表" })).queryAllByRole("listitem")).toHaveLength(0));
  expect(fetchMock.mock.calls.slice(1).map(([url, init]) => [url, init.method])).toEqual([
    ["/api/assets/v1", "DELETE"], ["/api/assets/v2", "DELETE"],
  ]);
  expect(screen.getByText("已选 0 个视频")).toBeTruthy();
  expect(screen.queryByRole("alert")).toBeNull();
});

test("clear video reports partial failure and retains failed and unprocessed rows", async () => {
  const fetchMock = vi.fn().mockResolvedValueOnce(json({ items: [video("v1"), video("v2"), video("v3")] }))
    .mockResolvedValueOnce(json({ items: [{ filename: "bad.mp4", status: "failed", error: { code: "INVALID_VIDEO" } }] }))
    .mockResolvedValueOnce(json({ id: "v1", deleted: true }))
    .mockResolvedValueOnce(json({ error: { code: "INTERNAL_ERROR" } }, 500));
  vi.stubGlobal("fetch", fetchMock);
  const { container } = render(<App />);
  await screen.findByText("v3.mp4");
  fireEvent.change(fileInput(container, 0), { target: { files: [new File(["bad"], "bad.mp4")] } });
  await screen.findByRole("button", { name: "移除视频 bad.mp4" });
  fireEvent.click(screen.getByRole("checkbox", { name: "选择视频 v1.mp4" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "选择视频 v2.mp4" }));
  fireEvent.click(screen.getByRole("button", { name: "清空视频" }));
  await waitFor(() => expect(container.querySelector(".ant-alert-error")).not.toBeNull());
  await waitFor(() => expect(screen.queryByText("v1.mp4")).toBeNull());
  expect(screen.getByText("v2.mp4")).toBeTruthy();
  expect(screen.getByText("v3.mp4")).toBeTruthy();
  expect(screen.getByText("bad.mp4")).toBeTruthy();
  expect(screen.getByText("已选 1 个视频")).toBeTruthy();
  expect(fetchMock.mock.calls.slice(2).map(([url]) => url)).toEqual(["/api/assets/v1", "/api/assets/v2"]);
});

test("clear loading disables video upload, row deletion and mixing", async () => {
  const pending = deferred<Response>();
  const fetchMock = vi.fn().mockResolvedValueOnce(json({ items: [video("v1"), audio("a1")] }))
    .mockImplementationOnce(() => pending.promise);
  vi.stubGlobal("fetch", fetchMock);
  const { container } = render(<App />);
  await selectMixAssets();
  fireEvent.click(screen.getByRole("button", { name: "清空视频" }));
  const button = screen.getByRole<HTMLButtonElement>("button", { name: /清空视频/ });
  expect(button.disabled).toBe(true);
  expect(button.classList.contains("ant-btn-loading")).toBe(true);
  expect(fileInput(container, 0).disabled).toBe(true);
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "删除视频 v1.mp4" }).disabled).toBe(true);
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "开始混剪" }).disabled).toBe(true);
  pending.resolve(json({ id: "v1", deleted: true }));
  await waitFor(() => expect(screen.getByRole<HTMLButtonElement>("button", { name: /清空视频/ }).classList.contains("ant-btn-loading")).toBe(false));
});

test("single video deletion disables clear until its request completes", async () => {
  const pending = deferred<Response>();
  vi.stubGlobal("fetch", vi.fn().mockResolvedValueOnce(json({ items: [video("v1"), video("v2")] }))
    .mockImplementationOnce(() => pending.promise));
  render(<App />);
  await screen.findByRole("button", { name: "删除视频 v2.mp4" });
  fireEvent.click(screen.getByRole("button", { name: "删除视频 v1.mp4" }));
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "清空视频" }).disabled).toBe(true);
  pending.resolve(json({ id: "v1", deleted: true }));
  await waitFor(() => expect(screen.getByRole<HTMLButtonElement>("button", { name: "清空视频" }).disabled).toBe(false));
});

test("mixing disables video removal and reset", async () => {
  const pending = deferred<Response>();
  const fetchMock = vi.fn().mockResolvedValueOnce(json({ items: [video("v1"), audio("a1")] }))
    .mockImplementationOnce(() => pending.promise);
  vi.stubGlobal("fetch", fetchMock);
  const { container } = render(<App />);
  await selectMixAssets();
  fireEvent.change(fileInput(container, 0), { target: { files: [new File(["bad"], "bad.mp4")] } });
  // The in-flight upload produces a failed row before Mix starts.
  pending.resolve(json({ items: [{ filename: "bad.mp4", status: "failed", error: { code: "INVALID_VIDEO" } }] }));
  await screen.findByRole("button", { name: "移除视频 bad.mp4" });
  const mixPending = deferred<Response>();
  fetchMock.mockImplementationOnce(() => mixPending.promise);
  fireEvent.click(screen.getByRole("button", { name: "开始混剪" }));
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "删除视频 v1.mp4" }).disabled).toBe(true);
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "移除视频 bad.mp4" }).disabled).toBe(true);
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "重置选择" }).disabled).toBe(true);
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "清空视频" }).disabled).toBe(true);
  expect(fetchMock).toHaveBeenCalledTimes(3);
  mixPending.resolve(json(completed()));
  await screen.findByLabelText("成片预览");
});

test("delete API validates success payload", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValueOnce(json({ id: "wrong", deleted: true })));
  await expect(deleteVideoAsset("v1")).rejects.toThrow("请求失败，请稍后重试");
});

test("restores a poster and falls back when its image fails", async () => {
  const posterUrl = "/api/assets/v1/poster";
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ items: [{ ...video("v1"), posterUrl }] })));
  render(<App />);
  const poster = await screen.findByRole<HTMLImageElement>("img", { name: "视频封面 v1.mp4" });
  expect(poster.src.endsWith(posterUrl)).toBe(true);
  expect(poster.style.objectFit).toBe("contain");
  fireEvent.error(poster);
  expect(screen.queryByRole("img", { name: "视频封面 v1.mp4" })).toBeNull();
  expect(screen.getByRole("img", { name: "视频封面占位 v1.mp4" })).toBeTruthy();
  expect(screen.getByText("9.7 秒")).toBeTruthy();
  expect(screen.getByRole<HTMLInputElement>("checkbox", { name: "选择视频 v1.mp4" }).checked).toBe(false);
});

test("uploads two videos in one request, maps results, and toggles selection", async () => {
  const pending = deferred<Response>();
  const fetchMock = vi.fn().mockResolvedValueOnce(json({ items: [] })).mockImplementationOnce(() => pending.promise);
  vi.stubGlobal("fetch", fetchMock);
  const { container } = render(<App />);
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  const files = [new File(["one"], "first.mp4", { type: "video/mp4" }), new File(["two"], "second.mp4", { type: "video/mp4" })];
  fireEvent.change(fileInput(container, 0), { target: { files } });
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
  const [url, init] = fetchMock.mock.calls[1] as [string, RequestInit];
  expect(url).toBe("/api/assets/videos");
  expect(init.method).toBe("POST");
  expect((init.body as FormData).getAll("files")).toEqual(files);
  expect(screen.getByText("first.mp4")).toBeTruthy();
  expect(screen.getByText("second.mp4")).toBeTruthy();
  expect(screen.getAllByText("uploading")).toHaveLength(2);
  expect(screen.getByRole<HTMLButtonElement>("button", { name: "清空视频" }).disabled).toBe(true);
  expect(screen.queryAllByRole("checkbox")).toHaveLength(0);
  pending.resolve(json({ items: [
    { filename: "first.mp4", status: "ready", asset: { ...video("v1", "first.mp4"), posterUrl: "/api/assets/v1/poster" } },
    { filename: "second.mp4", status: "ready", asset: video("v2", "second.mp4", 2_300_000) },
  ] }));
  await waitFor(() => expect(screen.getByText("已选 2 个视频")).toBeTruthy());
  expect(screen.getByText("9.7 秒")).toBeTruthy();
  expect(screen.getByText("2.3 秒")).toBeTruthy();
  expect(screen.getByRole<HTMLImageElement>("img", { name: "视频封面 first.mp4" }).src.endsWith("/api/assets/v1/poster")).toBe(true);
  expect(screen.getByRole("img", { name: "视频封面占位 second.mp4" })).toBeTruthy();
  const first = screen.getByRole<HTMLInputElement>("checkbox", { name: "选择视频 first.mp4" });
  expect(first.checked).toBe(true);
  fireEvent.click(first);
  expect(screen.getByText("已选 1 个视频")).toBeTruthy();
  fireEvent.click(first);
  expect(screen.getByText("已选 2 个视频")).toBeTruthy();
});

test("ignores a MIME identified image without uploading or adding a video row", async () => {
  const fetchMock = vi.fn().mockResolvedValue(json({ items: [] }));
  vi.stubGlobal("fetch", fetchMock);
  const { container } = render(<App />);
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  fireEvent.change(fileInput(container, 0), { target: { files: [new File(["png"], "picture.png", { type: "image/png" })] } });
  expect(fetchMock).toHaveBeenCalledTimes(1);
  expect(within(screen.getByRole("list", { name: "视频素材列表" })).queryAllByRole("listitem")).toHaveLength(0);
  expect(screen.getByRole("alert")).toHaveProperty("textContent", "已忽略 1 个非视频文件：picture.png");
  expect(screen.getByRole("alert").classList.contains("ant-alert-error")).toBe(true);
});

test("uploads only video MIME from a mixed selection and clears the warning on the next valid selection", async () => {
  const fetchMock = vi.fn().mockResolvedValueOnce(json({ items: [] }))
    .mockResolvedValueOnce(json({ items: [{ filename: "clip.mp4", status: "ready", asset: video("v1", "clip.mp4") }] }))
    .mockResolvedValueOnce(json({ items: [{ filename: "next.mp4", status: "ready", asset: video("v2", "next.mp4") }] }));
  vi.stubGlobal("fetch", fetchMock);
  const { container } = render(<App />);
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  const clip = new File(["video"], "clip.mp4", { type: "video/mp4" });
  const image = new File(["png"], "picture.png", { type: "image/png" });
  fireEvent.change(fileInput(container, 0), { target: { files: [clip, image] } });
  await screen.findByRole("checkbox", { name: "选择视频 clip.mp4" });
  expect((fetchMock.mock.calls[1][1].body as FormData).getAll("files")).toEqual([clip]);
  expect(within(screen.getByRole("list", { name: "视频素材列表" })).getAllByRole("listitem")).toHaveLength(1);
  expect(screen.queryByText("picture.png")).toBeNull();
  expect(screen.getByRole("alert")).toHaveProperty("textContent", "已忽略 1 个非视频文件：picture.png");
  const next = new File(["video"], "next.mp4", { type: "video/mp4" });
  fireEvent.change(fileInput(container, 0), { target: { files: [next] } });
  await screen.findByRole("checkbox", { name: "选择视频 next.mp4" });
  expect(screen.queryByRole("alert")).toBeNull();
});

test("uploads a file with an empty MIME for server validation", async () => {
  const fetchMock = vi.fn().mockResolvedValueOnce(json({ items: [] }))
    .mockResolvedValueOnce(json({ items: [{ filename: "unknown", status: "ready", asset: video("v1", "unknown") }] }));
  vi.stubGlobal("fetch", fetchMock);
  const { container } = render(<App />);
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  const unknown = new File(["video"], "unknown");
  fireEvent.change(fileInput(container, 0), { target: { files: [unknown] } });
  await screen.findByRole("checkbox", { name: "选择视频 unknown" });
  expect((fetchMock.mock.calls[1][1].body as FormData).getAll("files")).toEqual([unknown]);
});

test("keeps one ready row when two uploaded filenames resolve to one video", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValueOnce(json({ items: [] })).mockResolvedValueOnce(json({ items: [
    { filename: "first.mp4", status: "ready", asset: video("v1", "first.mp4") },
    { filename: "renamed.mp4", status: "ready", asset: video("v1", "first.mp4") },
  ] })));
  const { container } = render(<App />);
  fireEvent.change(fileInput(container, 0), { target: { files: [
    new File(["same"], "first.mp4"), new File(["same"], "renamed.mp4"),
  ] } });
  await waitFor(() => expect(screen.getByText("已选 1 个视频")).toBeTruthy());
  expect(within(screen.getByRole("list", { name: "视频素材列表" })).getAllByRole("listitem")).toHaveLength(1);
  expect(screen.getAllByRole("checkbox", { name: "选择视频 first.mp4" })).toHaveLength(1);
  expect(screen.queryByText("renamed.mp4")).toBeNull();
});

test("reuploading a restored video selects its existing row", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValueOnce(json({ items: [video("v1", "original.mp4")] }))
    .mockResolvedValueOnce(json({ items: [
      { filename: "renamed.mp4", status: "ready", asset: video("v1", "original.mp4") },
    ] })));
  const { container } = render(<App />);
  await screen.findByText("original.mp4");
  fireEvent.change(fileInput(container, 0), { target: { files: [new File(["same"], "renamed.mp4")] } });
  await waitFor(() => expect(screen.getByText("已选 1 个视频")).toBeTruthy());
  expect(within(screen.getByRole("list", { name: "视频素材列表" })).getAllByRole("listitem")).toHaveLength(1);
  expect(screen.getByRole<HTMLInputElement>("checkbox", { name: "选择视频 original.mp4" }).checked).toBe(true);
  expect(screen.queryByText("renamed.mp4")).toBeNull();
});

test("keeps partial failures separate and selects only ready videos", async () => {
  const fetchMock = vi.fn().mockResolvedValueOnce(json({ items: [] })).mockResolvedValueOnce(json({ items: [
    { filename: "good.mp4", status: "ready", asset: video("good", "good.mp4") },
    { filename: "bad.mp4", status: "failed", error: { code: "INVALID_VIDEO", message: "raw /server/path" } },
  ] }));
  vi.stubGlobal("fetch", fetchMock);
  const { container } = render(<App />);
  fireEvent.change(fileInput(container, 0), { target: { files: [new File(["a"], "good.mp4"), new File(["b"], "bad.mp4")] } });
  await waitFor(() => expect(screen.getByText("已选 1 个视频")).toBeTruthy());
  expect(screen.getByRole("checkbox", { name: "选择视频 good.mp4" })).toBeTruthy();
  expect(screen.queryByRole("checkbox", { name: "选择视频 bad.mp4" })).toBeNull();
  expect(screen.getByText("未检测到有效视频流")).toBeTruthy();
  expect(screen.getByText("failed").classList.contains("ant-typography-danger")).toBe(true);
  expect(screen.getByText("未检测到有效视频流").classList.contains("ant-typography-danger")).toBe(true);
  expect(screen.getByRole("img", { name: "视频封面占位 bad.mp4" })).toBeTruthy();
  expect(screen.queryByText(/server\/path/)).toBeNull();
});

test("maps same named videos by position instead of filename", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValueOnce(json({ items: [] })).mockResolvedValueOnce(json({ items: [
    { filename: "same.mp4", status: "ready", asset: video("v1", "same.mp4", 1_100_000) },
    { filename: "same.mp4", status: "failed", error: { code: "INVALID_VIDEO" } },
  ] })));
  const { container } = render(<App />);
  fireEvent.change(fileInput(container, 0), { target: { files: [new File(["1"], "same.mp4"), new File(["2"], "same.mp4")] } });
  await waitFor(() => expect(screen.getByText("已选 1 个视频")).toBeTruthy());
  const rows = within(screen.getByRole("list", { name: "视频素材列表" })).getAllByRole("listitem");
  expect(rows).toHaveLength(2);
  expect(within(rows[0]).getByText("1.1 秒")).toBeTruthy();
  expect(within(rows[1]).getByText("未检测到有效视频流")).toBeTruthy();
});

test("selects one restored audio, then switches to newly uploaded audio without deletion", async () => {
  const fetchMock = vi.fn().mockResolvedValueOnce(json({ items: [audio("a1"), audio("a2")] }))
    .mockResolvedValueOnce(json({ filename: "new.m4a", status: "ready", asset: audio("a3", "new.m4a") }));
  vi.stubGlobal("fetch", fetchMock);
  const { container } = render(<App />);
  await screen.findByText(/a2.m4a/);
  const first = screen.getByRole<HTMLInputElement>("radio", { name: "选择口播 a1.m4a" });
  const second = screen.getByRole<HTMLInputElement>("radio", { name: "选择口播 a2.m4a" });
  expect(first.checked || second.checked).toBe(false);
  fireEvent.click(first);
  expect(first.checked).toBe(true);
  fireEvent.click(second);
  expect(first.checked).toBe(false);
  expect(second.checked).toBe(true);
  const file = new File(["audio"], "new.m4a", { type: "audio/mp4" });
  fireEvent.change(fileInput(container, 1), { target: { files: [file] } });
  await waitFor(() => expect(screen.getByRole<HTMLInputElement>("radio", { name: "选择口播 new.m4a" }).checked).toBe(true));
  expect(screen.getAllByRole("radio")).toHaveLength(3);
  expect(first.checked || second.checked).toBe(false);
  expect(fetchMock).toHaveBeenCalledTimes(2);
  expect(fetchMock.mock.calls[1][0]).toBe("/api/assets/audio");
  expect((fetchMock.mock.calls[1][1].body as FormData).getAll("file")).toEqual([file]);
  expect(fetchMock.mock.calls[1][1].method).toBe("POST");
});

test("audio failure keeps prior selection and shows readable error", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValueOnce(json({ items: [audio("old")] }))
    .mockResolvedValueOnce(json({ error: { code: "INVALID_AUDIO", message: "raw /private/path" } }, 400)));
  const { container } = render(<App />);
  const old = await screen.findByRole<HTMLInputElement>("radio", { name: "选择口播 old.m4a" });
  fireEvent.click(old);
  fireEvent.change(fileInput(container, 1), { target: { files: [new File(["bad"], "bad.m4a")] } });
  await screen.findByText(/未检测到有效音频流/);
  expect(old.checked).toBe(true);
  expect(screen.queryByText(/private\/path/)).toBeNull();
});

test("audio picker sends only one file when several are dropped", async () => {
  const pending = deferred<Response>();
  const fetchMock = vi.fn().mockResolvedValueOnce(json({ items: [] })).mockImplementationOnce(() => pending.promise);
  vi.stubGlobal("fetch", fetchMock);
  const { container } = render(<App />);
  const files = [new File(["a"], "first.m4a"), new File(["b"], "second.m4a")];
  fireEvent.change(fileInput(container, 1), { target: { files } });
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
  expect((fetchMock.mock.calls[1][1].body as FormData).getAll("file")).toEqual([files[0]]);
  pending.resolve(json({ filename: "first.m4a", status: "ready", asset: audio("a1", "first.m4a") }));
  await screen.findByRole("radio", { name: "选择口播 first.m4a" });
  expect(screen.queryByText(/second.m4a/)).toBeNull();
});

test("late GET merges with uploaded rows without clearing new selection", async () => {
  const pendingGet = deferred<Response>();
  const fetchMock = vi.fn().mockImplementationOnce(() => pendingGet.promise)
    .mockResolvedValueOnce(json({ items: [{ filename: "new.mp4", status: "ready", asset: video("v2", "new.mp4") }] }));
  vi.stubGlobal("fetch", fetchMock);
  const { container } = render(<App />);
  fireEvent.change(fileInput(container, 0), { target: { files: [new File(["v"], "new.mp4")] } });
  await waitFor(() => expect(screen.getByText("已选 1 个视频")).toBeTruthy());
  pendingGet.resolve(json({ items: [video("v1", "old.mp4"), video("v2", "new.mp4")] }));
  await screen.findByText("old.mp4");
  expect(screen.getAllByText("new.mp4")).toHaveLength(1);
  expect(screen.getByText("已选 1 个视频")).toBeTruthy();
  expect(screen.getByRole<HTMLInputElement>("checkbox", { name: "选择视频 old.mp4" }).checked).toBe(false);
});

test("overlapping video batches keep their own results and selections", async () => {
  const first = deferred<Response>();
  const second = deferred<Response>();
  const fetchMock = vi.fn().mockResolvedValueOnce(json({ items: [] }))
    .mockImplementationOnce(() => first.promise).mockImplementationOnce(() => second.promise);
  vi.stubGlobal("fetch", fetchMock);
  const { container } = render(<App />);
  fireEvent.change(fileInput(container, 0), { target: { files: [new File(["1"], "first.mp4")] } });
  fireEvent.change(fileInput(container, 0), { target: { files: [new File(["2"], "second.mp4")] } });
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(3));
  second.resolve(json({ items: [{ filename: "second.mp4", status: "ready", asset: video("v2", "second.mp4") }] }));
  await waitFor(() => expect(screen.getByText("已选 1 个视频")).toBeTruthy());
  first.resolve(json({ items: [{ filename: "first.mp4", status: "ready", asset: video("v1", "first.mp4") }] }));
  await waitFor(() => expect(screen.getByText("已选 2 个视频")).toBeTruthy());
  expect(screen.getAllByRole<HTMLInputElement>("checkbox").every((checkbox) => checkbox.checked)).toBe(true);
});

test("batch network failure marks all pending videos failed", async () => {
  const pending = deferred<Response>();
  vi.stubGlobal("fetch", vi.fn().mockResolvedValueOnce(json({ items: [] })).mockImplementationOnce(() => pending.promise));
  const { container } = render(<App />);
  fireEvent.change(fileInput(container, 0), { target: { files: [new File(["1"], "a.mp4"), new File(["2"], "b.mp4")] } });
  expect(screen.getAllByText("uploading")).toHaveLength(2);
  pending.reject(new Error("raw stack /server/path"));
  await waitFor(() => expect(screen.getAllByText("failed")).toHaveLength(2));
  expect(screen.getAllByText("网络连接失败，请稍后重试")).toHaveLength(2);
  expect(screen.getByText("已选 0 个视频")).toBeTruthy();
});

test("video upload progress follows real events and waits for the server response", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ items: [] })));
  MockXHR.autoRespond = false;
  const { container } = render(<App />);
  fireEvent.change(fileInput(container, 0), { target: { files: [new File(["a"], "a.mp4")] } });
  const xhr = MockXHR.instances[0];
  expect(screen.getByText("上传中 0%")).toBeTruthy();
  xhr.progress(50, 100);
  await screen.findByText("上传中 50%");
  xhr.progress(100, 100);
  await screen.findByText("上传完成，处理中");
  expect(screen.getByText("uploading")).toBeTruthy();
  expect(screen.queryByRole("checkbox", { name: "选择视频 a.mp4" })).toBeNull();
  xhr.respond(200, { items: [{ filename: "a.mp4", status: "ready", asset: video("v1", "a.mp4") }] });
  await screen.findByRole("checkbox", { name: "选择视频 a.mp4" });
  await waitFor(() => expect(screen.queryByText("上传完成，处理中")).toBeNull());
});

test("video upload failure hides progress and keeps a failed row", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ items: [] })));
  MockXHR.autoRespond = false;
  const { container } = render(<App />);
  fireEvent.change(fileInput(container, 0), { target: { files: [new File(["a"], "a.mp4")] } });
  MockXHR.instances[0].progress(50, 100);
  await screen.findByText("上传中 50%");
  MockXHR.instances[0].respond(413, "raw response");
  await screen.findByText("文件超过上传限制");
  await waitFor(() => expect(screen.queryByText("上传中 50%")).toBeNull());
  expect(screen.getByText("failed")).toBeTruthy();
});

test("XHR video upload reports monotonic progress and validates success", async () => {
  MockXHR.autoRespond = false;
  const progress = vi.fn();
  const file = new File(["a"], "a.mp4");
  const promise = uploadVideos([file], undefined, progress);
  const xhr = MockXHR.instances[0];
  expect(xhr.method).toBe("POST");
  expect(xhr.url).toBe("/api/assets/videos");
  expect(xhr.body?.getAll("files")).toEqual([file]);
  xhr.progress(50, 100);
  xhr.progress(40, 100);
  xhr.progress(100, 100);
  expect(progress.mock.calls.map(([value]) => value.percent)).toEqual([50, 50, 100]);
  xhr.respond(200, { items: [{ filename: "a.mp4", status: "ready", asset: video("v1") }] });
  await expect(promise).resolves.toMatchObject([{ status: "ready", asset: { id: "v1" } }]);
  const controller = new AbortController();
  const completed = uploadVideos([file], controller.signal);
  const completedXHR = MockXHR.instances.at(-1)!;
  completedXHR.respond(200, { items: [{ filename: "a.mp4", status: "ready", asset: video("v1") }] });
  await completed;
  controller.abort();
  expect(completedXHR.aborted).toBe(false);
});

test("XHR video upload preserves API errors and rejects malformed responses", async () => {
  MockXHR.autoRespond = false;
  const file = new File(["a"], "a.mp4");
  let promise = uploadVideos([file]);
  MockXHR.instances.at(-1)!.respond(413, "not json");
  await expect(promise).rejects.toThrow("文件超过上传限制");
  promise = uploadVideos([file]);
  MockXHR.instances.at(-1)!.respond(422, { error: { code: "INVALID_VIDEO", message: "raw private text" } });
  await expect(promise).rejects.toMatchObject({ code: "INVALID_VIDEO", message: "未检测到有效视频流" });
  promise = uploadVideos([file]);
  MockXHR.instances.at(-1)!.respond(500, "<html>raw</html>");
  await expect(promise).rejects.toThrow("请求失败，请稍后重试");
  promise = uploadVideos([file]);
  MockXHR.instances.at(-1)!.respond(200, { items: [] });
  await expect(promise).rejects.toThrow("请求失败，请稍后重试");
  promise = uploadVideos([file]);
  MockXHR.instances.at(-1)!.respond(200, { items: [{ filename: "a.mp4", status: "ready", asset: { id: "v1" } }] });
  await expect(promise).rejects.toThrow("请求失败，请稍后重试");
});

test("XHR video upload handles network errors and AbortSignal", async () => {
  MockXHR.autoRespond = false;
  const file = new File(["a"], "a.mp4");
  let promise = uploadVideos([file]);
  MockXHR.instances.at(-1)!.onerror?.();
  await expect(promise).rejects.toThrow("网络连接失败，请稍后重试");
  const alreadyAborted = new AbortController();
  alreadyAborted.abort();
  const count = MockXHR.instances.length;
  await expect(uploadVideos([file], alreadyAborted.signal)).rejects.toMatchObject({ name: "AbortError" });
  expect(MockXHR.instances).toHaveLength(count);
  const controller = new AbortController();
  promise = uploadVideos([file], controller.signal);
  const xhr = MockXHR.instances.at(-1)!;
  controller.abort();
  await expect(promise).rejects.toMatchObject({ name: "AbortError" });
  expect(xhr.aborted).toBe(true);
});

test("413 and non JSON errors remain readable and do not leak response bodies", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValueOnce(json({ items: [] }))
    .mockResolvedValueOnce(json({ error: { code: "UPLOAD_TOO_LARGE", message: "raw stack" } }, 413))
    .mockResolvedValueOnce(new Response("<html>raw backend trace</html>", { status: 500 })));
  const { container } = render(<App />);
  fireEvent.change(fileInput(container, 0), { target: { files: [new File(["1"], "huge.mp4")] } });
  await screen.findByText("文件超过上传限制");
  fireEvent.change(fileInput(container, 0), { target: { files: [new File(["2"], "other.mp4")] } });
  await screen.findByText("请求失败，请稍后重试");
  expect(screen.queryByText(/raw stack|backend trace/)).toBeNull();
});

test("GET failure shows an Alert without raw response", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("<html>internal path</html>", { status: 500 })));
  render(<App />);
  expect(await screen.findByRole("alert")).toHaveProperty("textContent", "素材加载失败：请求失败，请稍后重试");
  expect(screen.queryByText(/internal path/)).toBeNull();
});

test("API uses exact multipart fields and formats fractional duration", async () => {
  const fetchMock = vi.fn().mockResolvedValueOnce(json({ items: [video("v1")] }))
    .mockResolvedValueOnce(json({ items: [{ filename: "one.mp4", status: "ready", asset: video("v2") }] }))
    .mockResolvedValueOnce(json({ filename: "one.m4a", status: "ready", asset: audio("a1") }));
  vi.stubGlobal("fetch", fetchMock);
  expect((await getAssets())[0].durationUs).toBe(9_700_000);
  const v = new File(["v"], "one.mp4");
  const a = new File(["a"], "one.m4a");
  await uploadVideos([v]);
  await uploadAudio(a);
  expect((fetchMock.mock.calls[1][1].body as FormData).getAll("files")).toEqual([v]);
  expect((fetchMock.mock.calls[2][1].body as FormData).getAll("file")).toEqual([a]);
  expect(formatDuration(9_700_000)).toBe("9.7 秒");
});

const completed = (seed = "9007199254740993") => ({
  id: "m1", status: "completed", seed, durationUs: 4_500_000,
  previewUrl: "/api/mixes/m1/file", downloadUrl: "/api/mixes/m1/download",
});

async function selectMixAssets() {
  await screen.findByRole("checkbox", { name: "选择视频 v1.mp4" });
  fireEvent.click(screen.getByRole("checkbox", { name: "选择视频 v1.mp4" }));
  fireEvent.click(screen.getByRole("radio", { name: "选择口播 a1.m4a" }));
}

test("mix requires a selected ready video and audio", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(json({ items: [video("v1"), audio("a1")] })));
  render(<App />);
  const start = screen.getByRole<HTMLButtonElement>("button", { name: "开始混剪" });
  expect(start.disabled).toBe(true);
  await screen.findByRole("checkbox", { name: "选择视频 v1.mp4" });
  fireEvent.click(screen.getByRole("checkbox", { name: "选择视频 v1.mp4" }));
  expect(start.disabled).toBe(true);
  fireEvent.click(screen.getByRole("checkbox", { name: "选择视频 v1.mp4" }));
  fireEvent.click(screen.getByRole("radio", { name: "选择口播 a1.m4a" }));
  expect(start.disabled).toBe(true);
  fireEvent.click(screen.getByRole("checkbox", { name: "选择视频 v1.mp4" }));
  expect(start.disabled).toBe(false);
});

test("pending mix has a synchronous duplicate lock and completes with preview and download", async () => {
  const pending = deferred<Response>();
  const fetchMock = vi.fn().mockResolvedValueOnce(json({ items: [video("v1"), audio("a1")] }))
    .mockImplementationOnce(() => pending.promise);
  vi.stubGlobal("fetch", fetchMock);
  render(<App />);
  await selectMixAssets();
  fireEvent.change(screen.getByRole("textbox", { name: "随机种子（可选）" }), { target: { value: "9007199254740993" } });
  const start = screen.getByRole<HTMLButtonElement>("button", { name: "开始混剪" });
  fireEvent.click(start);
  fireEvent.click(start);
  expect(fetchMock).toHaveBeenCalledTimes(2);
  expect(fetchMock.mock.calls[1][0]).toBe("/api/mixes");
  expect(JSON.parse(fetchMock.mock.calls[1][1].body)).toEqual({ videoIds: ["v1"], audioId: "a1", seed: "9007199254740993" });
  expect(start.disabled).toBe(true);
  expect(screen.getByText("制作中")).toBeTruthy();
  pending.resolve(json(completed()));
  const player = await screen.findByLabelText<HTMLVideoElement>("成片预览");
  expect(player.controls).toBe(true);
  expect(player.getAttribute("src")).toBe("/api/mixes/m1/file");
  expect(screen.getByText("本次种子：9007199254740993")).toBeTruthy();
  expect(screen.getByText("时长：4.5 秒")).toBeTruthy();
  const link = screen.getByRole<HTMLAnchorElement>("link", { name: "下载 MP4" });
  expect(link.getAttribute("href")).toBe("/api/mixes/m1/download");
  expect(link.hasAttribute("download")).toBe(true);
  expect(screen.queryByText(/任务历史|进度百分比|字幕开关/)).toBeNull();
});

test("empty seed is omitted on initial and repeat mixes; selection and assets remain", async () => {
  const fetchMock = vi.fn().mockResolvedValueOnce(json({ items: [video("v1"), audio("a1")] }))
    .mockResolvedValueOnce(json(completed("123"))).mockResolvedValueOnce(json(completed("456")));
  vi.stubGlobal("fetch", fetchMock);
  render(<App />);
  await selectMixAssets();
  fireEvent.click(screen.getByRole("button", { name: "开始混剪" }));
  await screen.findByText("本次种子：123");
  fireEvent.click(screen.getByRole("button", { name: "重新制作" }));
  await screen.findByText("本次种子：456");
  expect(JSON.parse(fetchMock.mock.calls[1][1].body)).toEqual({ videoIds: ["v1"], audioId: "a1" });
  expect(JSON.parse(fetchMock.mock.calls[2][1].body)).toEqual({ videoIds: ["v1"], audioId: "a1" });
  expect(screen.getByRole<HTMLInputElement>("checkbox", { name: "选择视频 v1.mp4" }).checked).toBe(true);
  expect(screen.getByRole<HTMLInputElement>("radio", { name: "选择口播 a1.m4a" }).checked).toBe(true);
});

test("int64 max seed is sent unchanged", async () => {
  const fetchMock = vi.fn().mockResolvedValueOnce(json({ items: [video("v1"), audio("a1")] }))
    .mockResolvedValueOnce(json(completed("9223372036854775807")));
  vi.stubGlobal("fetch", fetchMock);
  render(<App />);
  await selectMixAssets();
  fireEvent.change(screen.getByRole("textbox", { name: "随机种子（可选）" }), { target: { value: "9223372036854775807" } });
  fireEvent.click(screen.getByRole("button", { name: "开始混剪" }));
  await screen.findByText("本次种子：9223372036854775807");
  expect(JSON.parse(fetchMock.mock.calls[1][1].body).seed).toBe("9223372036854775807");
});

test.each([
  ["INSUFFICIENT_VIDEO_DURATION", { missingDurationUs: 4_700_000 }, "还缺少 4.7 秒视频素材"],
  ["MIX_BUSY", {}, "已有混剪正在制作，请稍后重试"],
  ["MIX_TIMEOUT", {}, "混剪超时，请重试"],
  ["INVALID_SEED", {}, "随机种子无效，请输入 int64 十进制整数"],
  ["FFMPEG_FAILED", {}, "视频渲染失败，请重试"],
  ["RENDER_VALIDATION_FAILED", {}, "成片校验失败，请重试"],
])("%s failure is readable and keeps selections for retry", async (code, details, message) => {
  const fetchMock = vi.fn().mockResolvedValueOnce(json({ items: [video("v1"), audio("a1")] }))
    .mockResolvedValueOnce(json({ error: { code, message: "raw /private/path", details } }, 422))
    .mockResolvedValueOnce(json(completed("retry")));
  vi.stubGlobal("fetch", fetchMock);
  render(<App />);
  await selectMixAssets();
  fireEvent.click(screen.getByRole("button", { name: "开始混剪" }));
  expect((await screen.findByRole("alert")).textContent).toContain(message);
  expect(screen.queryByText(/raw|private\/path/)).toBeNull();
  expect(screen.getByRole<HTMLInputElement>("checkbox", { name: "选择视频 v1.mp4" }).checked).toBe(true);
  expect(screen.getByRole<HTMLInputElement>("radio", { name: "选择口播 a1.m4a" }).checked).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "开始混剪" }));
  await screen.findByText("本次种子：retry");
  expect(screen.queryByRole("alert")).toBeNull();
});

test.each(["html", "network"]) ("%s mix failure never shows raw body or stack", async (kind) => {
  const fetchMock = vi.fn().mockResolvedValueOnce(json({ items: [video("v1"), audio("a1")] }))
    .mockImplementationOnce(() => kind === "html"
      ? Promise.resolve(new Response("<html>raw /server/path</html>", { status: 500 }))
      : Promise.reject(new Error("raw stack /server/path")));
  vi.stubGlobal("fetch", fetchMock);
  render(<App />);
  await selectMixAssets();
  fireEvent.click(screen.getByRole("button", { name: "开始混剪" }));
  expect(await screen.findByRole("alert")).toHaveProperty("textContent", kind === "html"
    ? "请求失败，请稍后重试" : "网络连接失败，请稍后重试");
  expect(screen.queryByText(/raw|server\/path|<html>/)).toBeNull();
});

test("mix and upload responses update their own state", async () => {
  const mixPending = deferred<Response>();
  const uploadPending = deferred<Response>();
  const fetchMock = vi.fn().mockResolvedValueOnce(json({ items: [video("v1"), audio("a1")] }))
    .mockImplementationOnce(() => mixPending.promise).mockImplementationOnce(() => uploadPending.promise);
  vi.stubGlobal("fetch", fetchMock);
  const { container } = render(<App />);
  await selectMixAssets();
  fireEvent.click(screen.getByRole("button", { name: "开始混剪" }));
  fireEvent.change(fileInput(container, 0), { target: { files: [new File(["v"], "new.mp4")] } });
  uploadPending.resolve(json({ items: [{ filename: "new.mp4", status: "ready", asset: video("v2", "new.mp4") }] }));
  await screen.findByRole("checkbox", { name: "选择视频 new.mp4" });
  mixPending.resolve(json(completed()));
  await screen.findByLabelText("成片预览");
  expect(screen.getByText("已选 2 个视频")).toBeTruthy();
  expect(screen.getByRole<HTMLInputElement>("checkbox", { name: "选择视频 new.mp4" }).checked).toBe(true);
});

test("a failed remake keeps the previous result and uses changed selections on retry", async () => {
  const fetchMock = vi.fn().mockResolvedValueOnce(json({ items: [video("v1"), video("v2"), audio("a1"), audio("a2")] }))
    .mockResolvedValueOnce(json(completed("first")))
    .mockResolvedValueOnce(json({ error: { code: "MIX_TIMEOUT" } }, 504))
    .mockResolvedValueOnce(json(completed("third")));
  vi.stubGlobal("fetch", fetchMock);
  render(<App />);
  await selectMixAssets();
  fireEvent.click(screen.getByRole("button", { name: "开始混剪" }));
  await screen.findByText("本次种子：first");
  fireEvent.click(screen.getByRole("checkbox", { name: "选择视频 v1.mp4" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "选择视频 v2.mp4" }));
  fireEvent.click(screen.getByRole("radio", { name: "选择口播 a2.m4a" }));
  fireEvent.change(screen.getByRole("textbox", { name: "随机种子（可选）" }), { target: { value: "42" } });
  fireEvent.click(screen.getByRole("button", { name: "重新制作" }));
  expect(await screen.findByRole("alert")).toHaveProperty("textContent", "混剪超时，请重试");
  expect(screen.getByText("本次种子：first")).toBeTruthy();
  expect(screen.getByLabelText("成片预览")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "重新制作" }));
  await screen.findByText("本次种子：third");
  expect(JSON.parse(fetchMock.mock.calls[2][1].body)).toEqual({ videoIds: ["v2"], audioId: "a2", seed: "42" });
  expect(JSON.parse(fetchMock.mock.calls[3][1].body)).toEqual({ videoIds: ["v2"], audioId: "a2", seed: "42" });
  expect(screen.getByRole<HTMLInputElement>("textbox", { name: "随机种子（可选）" }).value).toBe("42");
  expect(screen.queryByRole("alert")).toBeNull();
});

test("late GET during mix does not replace uploaded assets or selections", async () => {
  const pendingGet = deferred<Response>();
  const pendingMix = deferred<Response>();
  const fetchMock = vi.fn().mockImplementationOnce(() => pendingGet.promise)
    .mockResolvedValueOnce(json({ items: [{ filename: "v1.mp4", status: "ready", asset: video("v1") }] }))
    .mockResolvedValueOnce(json({ filename: "a1.m4a", status: "ready", asset: audio("a1") }))
    .mockImplementationOnce(() => pendingMix.promise);
  vi.stubGlobal("fetch", fetchMock);
  const { container } = render(<App />);
  fireEvent.change(fileInput(container, 0), { target: { files: [new File(["v"], "v1.mp4")] } });
  await screen.findByRole("checkbox", { name: "选择视频 v1.mp4" });
  fireEvent.change(fileInput(container, 1), { target: { files: [new File(["a"], "a1.m4a")] } });
  await screen.findByRole("radio", { name: "选择口播 a1.m4a" });
  fireEvent.click(screen.getByRole("button", { name: "开始混剪" }));
  pendingGet.resolve(json({ items: [video("v1"), audio("a1"), video("old", "old.mp4")] }));
  await screen.findByText("old.mp4");
  pendingMix.resolve(json(completed()));
  await screen.findByLabelText("成片预览");
  expect(screen.getAllByText("v1.mp4")).toHaveLength(1);
  expect(screen.getByRole<HTMLInputElement>("checkbox", { name: "选择视频 v1.mp4" }).checked).toBe(true);
  expect(screen.getByRole<HTMLInputElement>("radio", { name: "选择口播 a1.m4a" }).checked).toBe(true);
  expect(JSON.parse(fetchMock.mock.calls[3][1].body)).toEqual({ videoIds: ["v1"], audioId: "a1" });
});

test("unmount aborts pending mix", async () => {
  const pending = deferred<Response>();
  const fetchMock = vi.fn().mockResolvedValueOnce(json({ items: [video("v1"), audio("a1")] }))
    .mockImplementationOnce(() => pending.promise);
  vi.stubGlobal("fetch", fetchMock);
  const { unmount } = render(<App />);
  await selectMixAssets();
  fireEvent.click(screen.getByRole("button", { name: "开始混剪" }));
  const signal = fetchMock.mock.calls[1][1].signal as AbortSignal;
  unmount();
  expect(signal.aborted).toBe(true);
  pending.resolve(json(completed()));
  await Promise.resolve();
});

test("mix API rejects malformed successes and does not turn response URLs into HTML", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValueOnce(json({ ...completed(), previewUrl: "javascript:alert(1)" })));
  await expect(mixAssets({ videoIds: ["v1"], audioId: "a1" })).rejects.toThrow("请求失败，请稍后重试");
});

test("mix API exposes only stable error code, message, and missing duration", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValueOnce(json({ error: {
    code: "INSUFFICIENT_VIDEO_DURATION", message: "raw /server/path",
    details: { missingDurationUs: 4_700_000, stack: "raw stack" },
  } }, 422)));
  const error: unknown = await mixAssets({ videoIds: ["v1"], audioId: "a1" }).catch((failure: unknown) => failure);
  expect(error).toBeInstanceOf(ApiError);
  expect(error).toMatchObject({
    code: "INSUFFICIENT_VIDEO_DURATION",
    message: "所选视频素材时长不足，还缺少 4.7 秒视频素材",
    details: { missingDurationUs: 4_700_000 },
  } satisfies Partial<ApiError>);
  expect((error as ApiError).details).toEqual({ missingDurationUs: 4_700_000 });
});
