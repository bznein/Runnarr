export function trainingSheetWritebackStatusLabel(status?: string) {
  switch (status) {
    case "running": return "Writing";
    case "completed": return "Complete";
    case "completed_with_conflicts": return "Completed with conflicts";
    case "completed_with_warnings": return "Completed with warnings";
    case "not_applicable": return "Not applicable";
    case "not_provided": return "Awaiting reflection";
    case "skipped": return "Skipped";
    case "failed": return "Failed";
    case "canceled": return "Canceled";
    case "pending":
    case "":
    case undefined:
      return "Pending";
    default:
      return status
        .split("_")
        .filter(Boolean)
        .map((part) => part[0]?.toUpperCase() + part.slice(1))
        .join(" ");
  }
}
