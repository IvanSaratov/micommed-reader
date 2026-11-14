import { test, expect } from "@playwright/test";
import { createMetricHover } from "../src/metric-hover";
import type { Day } from "../src/types";

const data = () =>
  ({
    pressure: [
      { index: 0, minute: 0, first_raw: 70, second_raw: 100 },
      { index: 1, minute: 10, first_raw: 80, second_raw: 110 },
      { index: 2, minute: 10, first_raw: 85, second_raw: 115 },
    ],
    pressure_fields: [
      { scale: 0.1, unit: "см вод. ст." },
      { scale: 0.1, unit: "см вод. ст." },
    ],
    channels: [
      {
        field: "channel_1",
        scale: 1,
        unit: "л/мин",
        points: [
          { index: 0, minute: 2, raw: 12 },
          { index: 1, minute: 8, raw: 18 },
          { index: 2, minute: 30, raw: 44, break_before: true },
        ],
      },
      {
        field: "channel_2",
        scale: 1,
        unit: "мл",
        points: [
          { index: 0, minute: 2, raw: 400 },
          { index: 1, minute: 8, raw: 500 },
        ],
      },
    ],
    diagnostics: [],
  }) as unknown as Day;

test("hover follows the nearest actual sample of only the active metric", () => {
  const at = createMetricHover(data(), "2001-02-25T23:55:00");
  expect(at(0, 4)).toMatchObject({
    minute: 0,
    time: "23:55",
    values: [["7", "10"]],
    unit: "см вод. ст.",
  });
  expect(at(0, 6)).toMatchObject({
    minute: 10,
    time: "00:05",
    values: [
      ["8", "11"],
      ["8,5", "11,5"],
    ],
    unit: "см вод. ст.",
  });
  expect(at(1, 4)).toMatchObject({ values: [["12"]], unit: "л/мин" });
  expect(at(1, 6)).toMatchObject({ values: [["18"]], unit: "л/мин" });
  expect(at(2, 6)).toMatchObject({ values: [["500"]], unit: "мл" });
  expect(at(3, 6)).toBeUndefined();
});

test("hover respects gaps and does not extend a series across empty time", () => {
  const at = createMetricHover(data());
  expect(at(1, 17)).toBeUndefined();
  expect(at(1, 30)).toMatchObject({ values: [["44"]], unit: "л/мин" });
  expect(at(0, -10)).toBeUndefined();
  expect(at(0, 20)).toBeUndefined();
  const day = data();
  day.diagnostics.push({
    kind: "warning",
    code: "sample_gap",
    field: "channel_2",
    index: 1,
  });
  expect(createMetricHover(day)(2, 5)).toBeUndefined();
  expect(createMetricHover(day)(1, 5)).toMatchObject({
    values: [["12"]],
    unit: "л/мин",
  });
});
