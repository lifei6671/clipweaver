import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { App } from "./App";
import { getAssets, uploadAudio, uploadVideos } from "./api";
import { formatDuration } from "./utils/formatDuration";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
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
  expect(screen.getByText(/a1.m4a/)).toBeTruthy();
  expect(screen.getByText(/a2.m4a/)).toBeTruthy();
  expect(screen.getByText("已选 0 个视频")).toBeTruthy();
  expect(screen.getByRole<HTMLInputElement>("checkbox", { name: "选择视频 v1.mp4" }).checked).toBe(false);
  expect(screen.getAllByRole<HTMLInputElement>("radio").every((radio) => !radio.checked)).toBe(true);
  expect(fileInput(container, 0).multiple).toBe(true);
  expect(screen.queryByText("开始混剪")).toBeNull();
  expect(container.querySelector("video")).toBeNull();
  expect(screen.queryByText("下载")).toBeNull();
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
  expect(screen.queryAllByRole("checkbox")).toHaveLength(0);
  pending.resolve(json({ items: [
    { filename: "first.mp4", status: "ready", asset: video("v1", "first.mp4") },
    { filename: "second.mp4", status: "ready", asset: video("v2", "second.mp4", 2_300_000) },
  ] }));
  await waitFor(() => expect(screen.getByText("已选 2 个视频")).toBeTruthy());
  expect(screen.getByText("9.7 秒")).toBeTruthy();
  expect(screen.getByText("2.3 秒")).toBeTruthy();
  const first = screen.getByRole<HTMLInputElement>("checkbox", { name: "选择视频 first.mp4" });
  expect(first.checked).toBe(true);
  fireEvent.click(first);
  expect(screen.getByText("已选 1 个视频")).toBeTruthy();
  fireEvent.click(first);
  expect(screen.getByText("已选 2 个视频")).toBeTruthy();
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
