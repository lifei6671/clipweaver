import { render, screen, cleanup } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { App } from "./App";

afterEach(cleanup);

test("renders the shell", () => {
  render(<App />);
  expect(screen.getByRole("heading", { name: "ClipWeaver" })).toBeTruthy();
  expect(screen.getByText("工程骨架已就绪。")).toBeTruthy();
});
