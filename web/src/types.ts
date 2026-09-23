export type Asset = {
  id: string;
  kind: "video" | "audio";
  name: string;
  durationUs: number;
  width: number;
  height: number;
};

export type VideoUploadItem =
  | { filename: string; status: "ready"; asset: Asset }
  | { filename: string; status: "failed"; error: { code: string } };

export type AudioUploadItem = { filename: string; status: "ready"; asset: Asset };
