import { describe, expect, it } from "vitest";
import { plannedMatchPreviewForActivity } from "./plannedMatchPreview";

describe("planned match preview activity guard", () => {
  it("ignores a preview that resolves after navigating to another activity", () => {
    const preview = { activityId: "activity-a", fingerprint: "preview-a" };

    expect(plannedMatchPreviewForActivity(preview, "activity-b")).toBeUndefined();
    expect(plannedMatchPreviewForActivity(preview, "activity-a")).toBe(preview);
  });
});
