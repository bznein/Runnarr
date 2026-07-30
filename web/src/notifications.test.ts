import { describe, expect, it } from "vitest";
import { safeNotificationActionPath } from "./notifications";

describe("notification action links", () => {
  it("keeps safe local paths including query strings and hashes", () => {
    expect(safeNotificationActionPath("/workouts/123?section=garmin#status", "https://runnarr.example"))
      .toBe("/workouts/123?section=garmin#status");
  });

  it.each([
    "https://attacker.example/workouts/123",
    "//attacker.example/workouts/123",
    "javascript:alert(1)",
    "workouts/123"
  ])("falls back for unsafe path %s", (path) => {
    expect(safeNotificationActionPath(path, "https://runnarr.example")).toBe("/notifications");
  });
});
