import type { PlannedActivity } from "./types";

export type PlannedActivityAgendaGroup = {
  plannedDate: string;
  candidates: PlannedActivity[];
};

function plannedDateKey(value: string) {
  return value.match(/^\d{4}-\d{2}-\d{2}/)?.[0] ?? value;
}

export function groupPlannedActivityCandidates(candidates: PlannedActivity[]): PlannedActivityAgendaGroup[] {
  const groups = new Map<string, PlannedActivityAgendaGroup>();
  for (const candidate of candidates) {
    const plannedDate = plannedDateKey(candidate.plannedDate);
    const group = groups.get(plannedDate);
    if (group) {
      group.candidates.push(candidate);
    } else {
      groups.set(plannedDate, { plannedDate, candidates: [candidate] });
    }
  }
  return Array.from(groups.values()).sort((left, right) => left.plannedDate.localeCompare(right.plannedDate));
}

export function formatPlannedActivityAgendaDate(value: string) {
  const date = new Date(`${plannedDateKey(value)}T12:00:00`);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleDateString(undefined, { weekday: "long", month: "long", day: "numeric", year: "numeric" });
}

export function PlannedActivityMatchAgenda({
  candidates,
  suggestedId,
  selectedCandidateId,
  matching,
  onSelectCandidate
}: {
  candidates: PlannedActivity[];
  suggestedId?: string;
  selectedCandidateId?: string;
  matching: boolean;
  onSelectCandidate: (plannedActivityId: string) => void;
}) {
  const groups = groupPlannedActivityCandidates(candidates);

  return (
    <div className="planned-match-agenda" role="group" aria-label="Planned run candidates">
      {groups.map((group) => {
        const headingId = `planned-match-date-${group.plannedDate}`;
        return (
          <section className="planned-match-agenda-day" key={group.plannedDate} aria-labelledby={headingId}>
            <h3 className="planned-match-agenda-date" id={headingId}>{formatPlannedActivityAgendaDate(group.plannedDate)}</h3>
            <div className="planned-match-agenda-candidates">
              {group.candidates.map((candidate) => (
                <label className="planned-match-candidate" key={candidate.id}>
                  <input
                    type="radio"
                    name="planned-activity"
                    checked={candidate.id === selectedCandidateId}
                    disabled={matching}
                    aria-describedby={headingId}
                    onChange={() => onSelectCandidate(candidate.id)}
                  />
                  <div>
                    <div className="planned-match-candidate-title">
                      <strong>{candidate.name}</strong>
                      {candidate.id === suggestedId && <span className="planned-match-badge">Suggested</span>}
                    </div>
                    {candidate.notes && <p className="muted">{candidate.notes}</p>}
                  </div>
                </label>
              ))}
            </div>
          </section>
        );
      })}
    </div>
  );
}
