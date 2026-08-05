import { describe, expect, it } from "vitest";
import { trainingSheetWritebackStatusLabel } from "./trainingSheetWriteback";

describe("training-sheet writeback statuses", () => {
  it("formats stored status values for people", () => {
    expect(trainingSheetWritebackStatusLabel()).toBe("Pending");
    expect(trainingSheetWritebackStatusLabel("completed")).toBe("Complete");
    expect(trainingSheetWritebackStatusLabel("completed_with_conflicts")).toBe("Completed with conflicts");
    expect(trainingSheetWritebackStatusLabel("not_provided")).toBe("Awaiting reflection");
    expect(trainingSheetWritebackStatusLabel("future_status")).toBe("Future Status");
  });
});
