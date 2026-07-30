import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { formatPlannedActivityAgendaDate, formatPlannedActivityTimelineDate, groupPlannedActivityCandidates, plannedMatchResponseForDialog, PlannedActivityMatchAgenda } from "./plannedMatchAgenda";
import type { PlannedActivityMatchCandidate } from "./types";

function candidate(id: string, plannedDate: string, name = id, notes?: string, matchScore = 80, matchLevel: PlannedActivityMatchCandidate["matchLevel"] = "strong"): PlannedActivityMatchCandidate {
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
    status: "pending",
    matchScore,
    matchLevel,
    matchReasons: ["Same day", "Both continuous runs"]
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

  it("formats the compact date rail without shifting date-only values", () => {
    expect(formatPlannedActivityTimelineDate("2026-07-01")).toEqual({
      weekday: new Date("2026-07-01T12:00:00").toLocaleDateString(undefined, { weekday: "short" }),
      month: new Date("2026-07-01T12:00:00").toLocaleDateString(undefined, { month: "short" }),
      day: "1"
    });
  });

  it("renders one agenda section per populated date and keeps candidate controls", () => {
    const markup = renderToStaticMarkup(
      <PlannedActivityMatchAgenda
        candidates={[
          candidate("one", "2026-07-01", "Morning run"),
          candidate("two", "2026-07-01", "Intervals", "Intervals note", 65, "possible"),
          candidate("three", "2026-07-05", "Long run", undefined, 59, "weak")
        ]}
        suggestedId="one"
        selectedCandidateId="one"
        targetDate="2026-07-01"
        matching={false}
        onSelectCandidate={vi.fn()}
      />
    );

    expect((markup.match(/class="planned-match-agenda-day(?: |"|$)/g) ?? [])).toHaveLength(2);
    expect(markup).toContain("planned-match-agenda-day--target");
    expect(markup).toContain("Activity date");
    expect((markup.match(/type="radio"/g) ?? [])).toHaveLength(3);
    expect((markup.match(/aria-describedby="planned-match-date-2026-07-01"/g) ?? [])).toHaveLength(2);
    expect(markup).toContain("Suggested");
    expect(markup).toContain("80/100");
    expect(markup).toContain("Match score 80 out of 100");
    expect(markup).toContain("Both continuous runs");
    expect(markup).toContain("planned-match-score--possible");
    expect(markup).toContain("planned-match-score--weak");
    expect(markup).toContain("Intervals note");
    expect(markup.indexOf("Morning run")).toBeLessThan(markup.indexOf("Long run"));
  });

  it("keeps the match dialog renderable while the initial candidate request has no data", () => {
    expect(plannedMatchResponseForDialog()).toEqual({ candidates: [], hasMore: false });
  });
});
