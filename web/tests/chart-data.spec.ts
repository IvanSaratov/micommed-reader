import { test, expect } from "@playwright/test";
import {
  channelData,
  minuteLabel,
  pressureLines,
  tooltipText,
} from "../src/charts";
import type { Day } from "../src/types";

test("pressure keeps both sides of a divergence but never overlays a shared segment or bridges a break", () => {
  const data = {
    pressure: [7, 7, 9, 7, 7, 8, 7].map((second_raw, index) => ({
      index,
      minute: index,
      first_raw: 7,
      second_raw,
      break_before: index === 5,
    })),
    pressure_fields: [{ scale: 1 }, { scale: 1 }],
    diagnostics: [],
  } as unknown as Day;
  expect(pressureLines(data)[1]).toEqual([
    [1, 7, 1],
    [2, 9, 2],
    [3, 7, 3],
    [5, null, -1],
    [5, 8, 5],
    [6, 7, 6],
  ]);
  data.pressure.forEach((p) => (p.second_raw = p.first_raw));
  expect(pressureLines(data)).toHaveLength(1);
});

test("channel-specific gap and reversed time preserve raw order and repeated minutes", () => {
  const points = [
    { index: 0, minute: 7, raw: 4 },
    { index: 1, minute: 60, raw: 0 },
    { index: 2, minute: 58, raw: 9 },
    { index: 3, minute: 58, raw: 10 },
  ];
  expect(channelData(points, new Set([1, 2]), 0.1)).toEqual([
    [7, 0.4, 0],
    [60, null, -1],
    [60, 0, 1],
    [58, null, -1],
    [58, 0.9, 2],
    [58, 1, 3],
  ]);
  expect(channelData(points, new Set(), 1)).toHaveLength(4);
  expect(minuteLabel(0)).toBe("09:00");
  expect(minuteLabel(900)).toBe("00:00");
  expect(minuteLabel(1439)).toBe("08:59");
});

test("tooltip never borrows a neighbouring sample from another channel", () => {
  const day = {
    pressure: [],
    fields: {},
    events: [],
    pressure_fields: [
      { unit: "", scale: 1 },
      { unit: "", scale: 1 },
    ],
    channels: [
      {
        field: "channel_1",
        unit: "",
        scale: 1,
        points: [{ index: 0, minute: 10, raw: 111 }],
      },
      {
        field: "channel_2",
        unit: "",
        scale: 1,
        points: [{ index: 0, minute: 11, raw: 222 }],
      },
    ],
  } as unknown as Day;
  expect(tooltipText(day, 10)).toContain("111");
  expect(tooltipText(day, 10)).not.toContain("222");
  expect(tooltipText(day, 12)).toBe("");
  expect(tooltipText(day, 12, true)).toBe("");
});
