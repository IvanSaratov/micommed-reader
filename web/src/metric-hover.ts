import type { Day, Sample } from "./types";

type HoverData = Pick<
  Day,
  "pressure" | "pressure_fields" | "channels" | "diagnostics"
>;
export interface MetricReading {
  minute: number;
  time: string;
  values: string[][];
  unit: string;
}
interface Group extends MetricReading {
  breakBefore: boolean;
}
const number = new Intl.NumberFormat("ru-RU", { maximumFractionDigits: 6 });

export function createMetricHover(day: HoverData, origin?: string) {
  const start = origin ? Date.parse(origin + "Z") : Date.UTC(2000, 0, 1, 9);
  const group = <T extends Sample>(
    points: T[],
    field: string,
    format: (p: T) => string[],
    unit = "",
  ) => {
    const breaks = new Set(
      day.diagnostics
        .filter(
          (d) =>
            d.field === field &&
            ["sample_gap", "time_reversal"].includes(d.code),
        )
        .map((d) => d.index),
    );
    const groups = new Map<
      number,
      { first: T; values: Map<string, string[]>; breakBefore: boolean }
    >();
    for (const p of points) {
      let entry = groups.get(p.minute);
      if (!entry) {
        entry = { first: p, values: new Map(), breakBefore: false };
        groups.set(p.minute, entry);
      }
      const values = format(p);
      entry.values.set(values.join("|"), values);
      entry.breakBefore ||= !!p.break_before || breaks.has(p.index);
    }
    return [...groups]
      .sort(([a], [b]) => a - b)
      .map(([minute, entry]): Group => ({
        minute,
        time:
          entry.first.device_time?.slice(11, 16) ||
          new Date(start + minute * 60000).toISOString().slice(11, 16),
        values: [...entry.values.values()],
        unit,
        breakBefore: entry.breakBefore,
      }));
  };
  // Индекс готовится при открытии графиков, а не на каждом движении мыши.
  const series: Group[][] = [];
  series[0] = group(
    day.pressure,
    "pressure_1",
    (p) => {
      const first = number.format(p.first_raw * day.pressure_fields[0].scale);
      const second = number.format(p.second_raw * day.pressure_fields[1].scale);
      return first === second ? [first] : [first, second];
    },
    day.pressure_fields[0].unit,
  );
  day.channels.forEach((ch, i) => {
    series[i + 1] = group(
      ch.points,
      ch.field,
      (p) => [number.format(p.raw * ch.scale)],
      ch.field === "channel_4" && ch.unit === "1/мин"
        ? "дыханий в минуту"
        : ch.unit,
    );
  });

  return (
    axis: number,
    minute: number,
    edgeRadius = 1,
  ): MetricReading | undefined => {
    const points = series[axis];
    if (!points?.length || !Number.isFinite(minute)) return;
    let low = 0,
      high = points.length;
    while (low < high) {
      const mid = (low + high) >>> 1;
      if (points[mid].minute < minute) low = mid + 1;
      else high = mid;
    }
    const before = points[low - 1],
      after = points[low];
    const nearest = !before
      ? after
      : !after
        ? before
        : minute - before.minute <= after.minute - minute
          ? before
          : after;
    const distance = Math.abs(nearest.minute - minute);
    if (distance !== 0) {
      if (before && after) {
        if (after.breakBefore) return;
      } else if (distance > Math.min(1, edgeRadius)) return;
    }
    // Показываем время выбранного отсчёта: ближайшее значение не является
    // измерением в произвольной позиции курсора и не переносится через разрыв.
    return nearest;
  };
}
