import { describe, expect, it } from "vitest";
import { presetTimeRange } from "./timePresets";

describe("presetTimeRange", () => {
  const end = new Date(2026, 10, 1, 15, 23, 42); // Sunday and the first day of the month.

  it("uses local midnight for calendar presets and Monday for the start of the week", () => {
    expect(presetTimeRange("today", end)).toEqual([new Date(2026, 10, 1).toISOString(), end.toISOString()]);
    expect(presetTimeRange("thisWeek", end)[0]).toBe(new Date(2026, 9, 26).toISOString());
    expect(presetTimeRange("thisMonth", new Date(2026, 10, 20, 11))[0]).toBe(new Date(2026, 10, 1).toISOString());
  });

  it.each([["last24Hours", 24], ["last7Days", 168], ["last30Days", 720]] as const)("calculates %s as a rolling range", (preset, hours) => {
    const [start, stop] = presetTimeRange(preset, end);
    expect(Date.parse(stop) - Date.parse(start)).toBe(hours * 60 * 60 * 1000);
  });
});
