export const timePresets = [
  { id: "today", label: "今天" },
  { id: "last24Hours", label: "近 24 小时" },
  { id: "thisWeek", label: "本周" },
  { id: "last7Days", label: "近 7 天" },
  { id: "thisMonth", label: "本月" },
  { id: "last30Days", label: "近 30 天" },
] as const;

export function presetTimeRange(id: (typeof timePresets)[number]["id"], end = new Date()) {
  const start = new Date(end);
  if (id === "last24Hours" || id === "last7Days" || id === "last30Days") {
    const days = id === "last24Hours" ? 1 : id === "last7Days" ? 7 : 30;
    start.setTime(end.getTime() - days * 24 * 60 * 60 * 1000);
  } else {
    start.setHours(0, 0, 0, 0);
    if (id === "thisWeek") start.setDate(start.getDate() - (start.getDay() + 6) % 7);
    if (id === "thisMonth") start.setDate(1);
  }
  return [start.toISOString(), end.toISOString()] as const;
}
