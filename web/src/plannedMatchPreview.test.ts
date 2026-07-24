import { describe, expect, it } from "vitest";
import { plannedMatchActivityIsCurrent, plannedMatchPreviewForActivity, plannedMatchRequestIsCurrent } from "./plannedMatchPreview";

describe("planned match preview activity guard", () => {
  it("only treats mutation callbacks as current for the activity that started them", () => {
    expect(plannedMatchActivityIsCurrent("activity-a", "activity-b")).toBe(false);
    expect(plannedMatchActivityIsCurrent("activity-a", "activity-a")).toBe(true);
    expect(plannedMatchRequestIsCurrent("activity-a", 1, "activity-a", 2)).toBe(false);
    expect(plannedMatchRequestIsCurrent("activity-a", 2, "activity-a", 2)).toBe(true);
  });

  it("ignores a preview that resolves after navigating to another activity", () => {
    const preview = { activityId: "activity-a", fingerprint: "preview-a" };

    expect(plannedMatchPreviewForActivity(preview, "activity-b")).toBeUndefined();
    expect(plannedMatchPreviewForActivity(preview, "activity-a")).toBe(preview);
  });
});
