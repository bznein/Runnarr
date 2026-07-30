import type { CalendarPlanMatch } from "./types";

export function calendarPlanMatchDescription(
  match: CalendarPlanMatch,
  calendarDate: string,
  formatDate: (value: string) => string
): string {
  const summary = `Matched plan: ${match.name}`;
  return match.plannedDate === calendarDate
    ? summary
    : `${summary} · Planned for ${formatDate(match.plannedDate)}`;
}
