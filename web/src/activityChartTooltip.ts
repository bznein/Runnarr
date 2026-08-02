export type ActivityChartTrendKey = "elevationM" | "heartRate" | "paceSPKM" | "power" | "cadence";
export type ActivityChartRecordedKey = "rawElevationM" | "rawHeartRate" | "rawPaceSPKM" | "rawPower" | "rawCadence";

const recordedKeyByTrend: Record<ActivityChartTrendKey, ActivityChartRecordedKey> = {
  elevationM: "rawElevationM",
  heartRate: "rawHeartRate",
  paceSPKM: "rawPaceSPKM",
  power: "rawPower",
  cadence: "rawCadence"
};

export function recordedActivityChartKey(key: ActivityChartTrendKey): ActivityChartRecordedKey {
  return recordedKeyByTrend[key];
}

export function formatActivityChartTooltipValue(
  trend: number,
  recorded: number | undefined,
  format: (value: number) => string
) {
  const trendText = format(trend);
  if (recorded === undefined) {
    return trendText;
  }
  const recordedText = format(recorded);
  return recordedText === trendText ? trendText : `${recordedText} recorded · ${trendText} trend`;
}
