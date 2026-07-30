import { describe, expect, it } from "vitest";
import { calendarPlanMatchDescription } from "./calendarPlanMatch";

const formatDate = (value: string) => ({
  "2026-07-28": "Tue, Jul 28",
  "2026-07-30": "Thu, Jul 30"
}[value] ?? value);

describe("calendar matched-plan provenance", () => {
  it("shows the original planned date when completion moved", () => {
    expect(calendarPlanMatchDescription({ id: "plan-1", name: "Long run", plannedDate: "2026-07-28" }, "2026-07-30", formatDate))
      .toBe("Matched plan: Long run · Planned for Tue, Jul 28");
  });

  it("does not repeat the date for a same-day match", () => {
    expect(calendarPlanMatchDescription({ id: "plan-1", name: "Long run", plannedDate: "2026-07-30" }, "2026-07-30", formatDate))
      .toBe("Matched plan: Long run");
  });
});
