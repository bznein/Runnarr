import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { formatPlannedActivityAgendaDate, groupPlannedActivityCandidates, PlannedActivityMatchAgenda } from "./plannedMatchAgenda";
import type { PlannedActivity } from "./types";

function candidate(id: string, plannedDate: string, name = id, notes?: string): PlannedActivity {
  return {
    id,
    source: "training_sheet",
    sourceId: id,
    workbookId: "workbook",
    sheetId: "sheet",
    sheetTitle: "Week",
    planCell: "A1",
    plannedDate,
    name,
    sportType: "Run",
    notes,
    status: "pending"
  };
}

describe("planned activity match agenda", () => {
  it("groups candidates by calendar date in chronological order", () => {
    const candidates = [
      candidate("later", "2026-07-05T00:00:00Z"),
      candidate("same-day", "2026-07-01T00:00:00Z"),
      candidate("earlier", "2026-06-29T00:00:00Z"),
      candidate("same-day-2", "2026-07-01T12:00:00Z")
    ];

    expect(groupPlannedActivityCandidates(candidates)).toEqual([
      { plannedDate: "2026-06-29", candidates: [candidates[2]] },
      { plannedDate: "2026-07-01", candidates: [candidates[1], candidates[3]] },
      { plannedDate: "2026-07-05", candidates: [candidates[0]] }
    ]);
  });

  it("formats date headings without shifting date-only values", () => {
    expect(formatPlannedActivityAgendaDate("2026-07-01")).toBe(
      new Date("2026-07-01T12:00:00").toLocaleDateString(undefined, { weekday: "long", month: "long", day: "numeric", year: "numeric" })
    );
  });

  it("renders one agenda section per populated date and keeps candidate controls", () => {
    const markup = renderToStaticMarkup(
      <PlannedActivityMatchAgenda
        candidates={[
          candidate("one", "2026-07-01", "Morning run"),
          candidate("two", "2026-07-01", "Intervals", "Intervals note"),
          candidate("three", "2026-07-05", "Long run")
        ]}
        suggestedId="two"
        selectedCandidateId="two"
        matching={false}
        onSelectCandidate={vi.fn()}
      />
    );

    expect((markup.match(/class="planned-match-agenda-day"/g) ?? [])).toHaveLength(2);
    expect((markup.match(/type="radio"/g) ?? [])).toHaveLength(3);
    expect(markup).toContain("Suggested");
    expect(markup).toContain("Intervals note");
    expect(markup.indexOf("Morning run")).toBeLessThan(markup.indexOf("Long run"));
  });
});
