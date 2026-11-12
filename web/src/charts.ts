import * as echarts from "echarts/core";
import { LineChart, ScatterChart } from "echarts/charts";
import {
  GridComponent,
  TooltipComponent,
  DataZoomComponent,
  TitleComponent,
  LegendComponent,
} from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";
import type { EChartsOption } from "echarts";
import type { Day, Point } from "./types";

echarts.use([
  LineChart,
  ScatterChart,
  GridComponent,
  TooltipComponent,
  DataZoomComponent,
  TitleComponent,
  LegendComponent,
  CanvasRenderer,
]);
const colors = [
  "#256b50",
  "#c58b48",
  "#3178a5",
  "#8563a0",
  "#b76555",
  "#6e8560",
  "#537a8b",
];
type PlotData = Pick<
  Day,
  | "pressure"
  | "pressure_fields"
  | "channels"
  | "events"
  | "fields"
  | "diagnostics"
>;

export function minuteLabel(minute: number) {
  const m = (Math.round(minute) + 540) % 1440;
  return (
    String(Math.floor(m / 60)).padStart(2, "0") +
    ":" +
    String(m % 60).padStart(2, "0")
  );
}
export function channelData(
  points: Point[],
  breakIndexes: Set<number>,
  scale: number,
) {
  const data: (number | null)[][] = [];
  for (const p of points) {
    // Разрыв принадлежит конкретному каналу и исходному индексу следующей точки.
    if (p.break_before || breakIndexes.has(p.index))
      data.push([p.minute, null, -1]);
    data.push([p.minute, p.raw * scale, p.index]);
  }
  return data;
}
const number = new Intl.NumberFormat("ru-RU", { maximumFractionDigits: 6 });
export function tooltipText(
  day: PlotData,
  minute: number,
  simple = false,
  options: { origin?: string; channels?: boolean } = {},
) {
  const lines: string[] = [];
  const pressure = day.pressure.filter((p) => p.minute === minute);
  for (const p of pressure) {
    lines.push(
      simple
        ? `Давление: ${number.format(p.first_raw * day.pressure_fields[0].scale)} / ${number.format(p.second_raw * day.pressure_fields[1].scale)} ${day.pressure_fields[0].unit}`
        : `${day.fields.pressure_1?.Label || "pressure_1 / pressure_2"} · #${p.source_index ?? p.index}: ${number.format(p.first_raw * day.pressure_fields[0].scale)} / ${number.format(p.second_raw * day.pressure_fields[1].scale)} ${day.pressure_fields[0].unit} (отсчёты ${p.first_raw}, ${p.second_raw}; ${day.pressure_fields[0].meaning_evidence})`,
    );
  }
  for (const ch of options.channels === false ? [] : day.channels) {
    for (const p of ch.points.filter((p) => p.minute === minute)) {
      lines.push(
        simple
          ? `${day.fields[ch.field]?.Label || ch.field}: ${number.format(p.raw * ch.scale)} ${ch.unit}`
          : `${day.fields[ch.field]?.Label || ch.field} · #${p.source_index ?? p.index}: ${number.format(p.raw * ch.scale)} ${ch.unit} (отсчёт ${p.raw}; ${ch.meaning_evidence})`,
      );
    }
  }
  for (const e of day.events.filter((e) => e.minute === minute)) {
    lines.push(
      simple
        ? e.label || "Неизвестное событие"
        : `Событие #${e.source_index ?? e.index} · ${e.code} · ${e.label || "неизвестный код"} (${e.meaning_evidence || "U"})`,
    );
  }
  // Линия соединяет измерения, но не создаёт новых значений между ними.
  // Если в выбранной минуте нечего показать, подсказка остаётся скрытой.
  if (!lines.length) return "";
  const sample =
    pressure[0] ||
    day.channels.flatMap((c) => c.points).find((p) => p.minute === minute) ||
    day.events.find((p) => p.minute === minute);
  const stamp =
    sample?.device_time?.replace("T", " ") ||
    (options.origin
      ? new Date(Date.parse(options.origin + "Z") + minute * 60000)
          .toISOString()
          .slice(0, 19)
          .replace("T", " ")
      : `Минута ${minute} · ${minuteLabel(minute)}`);
  return stamp + "\n" + lines.join("\n");
}
export class DayCharts {
  private chart: echarts.ECharts;
  private resize: ResizeObserver;
  private day: PlotData | undefined;
  private simple = false;
  private origin?: string;
  private overview = false;
  private axisCount = 7;
  private tooltip = document.createElement("div");
  constructor(private element: HTMLElement) {
    this.chart = echarts.init(element, undefined, { renderer: "canvas" });
    this.tooltip.className = "chart-tooltip";
    this.tooltip.setAttribute("role", "tooltip");
    this.tooltip.hidden = true;
    element.append(this.tooltip);
    this.resize = new ResizeObserver(() => {
      this.tooltip.hidden = true;
      this.chart.resize();
    });
    const pointer = (event: { offsetX: number; offsetY: number }) => {
      const point = [event.offsetX, event.offsetY];
      const inGrid = Array.from({ length: this.axisCount }, (_, i) => i).some(
        (gridIndex) => this.chart.containPixel({ gridIndex }, point),
      );
      if (!this.day || !inGrid) {
        this.tooltip.hidden = true;
        return;
      }
      // ECharts привязывает штатную подсказку к ближайшей точке даже при snap:false.
      // Переводим положение курсора сами: в разрыве нельзя показывать чужой отсчёт.
      const minute = Math.round(
        this.chart.convertFromPixel({ xAxisIndex: 0 }, event.offsetX) as number,
      );
      this.tooltip.textContent = tooltipText(this.day, minute, this.simple, {
        origin: this.origin,
        channels: !this.overview,
      });
      this.tooltip.hidden = !this.tooltip.textContent;
      if (this.tooltip.hidden) return;
      this.tooltip.style.left =
        Math.max(
          0,
          Math.min(
            event.offsetX + 14,
            element.clientWidth - this.tooltip.offsetWidth,
          ),
        ) + "px";
      this.tooltip.style.top =
        Math.max(
          0,
          Math.min(
            event.offsetY + 14,
            element.clientHeight - this.tooltip.offsetHeight,
          ),
        ) + "px";
    };
    this.chart.getZr().on("mousemove", pointer);
    this.chart.getZr().on("click", pointer);
    this.chart.getZr().on("globalout", () => {
      this.tooltip.hidden = true;
    });
    this.resize.observe(element);
  }
  show(
    day: PlotData,
    options: {
      overview?: boolean;
      origin?: string;
      span?: number;
      simple?: boolean;
    } = {},
  ) {
    this.day = day;
    this.tooltip.hidden = true;
    this.simple = options.simple || false;
    this.origin = options.origin;
    this.overview = options.overview || false;
    const visible = options.overview ? [0, 6] : [0, 1, 2, 3, 4, 5, 6];
    const allAxes = visible.map((_, i) => i);
    this.axisCount = allAxes.length;
    this.element.style.height = 168 * allAxes.length + 59 + "px";
    this.chart.resize();
    const origin = options.origin
      ? Date.parse(options.origin + "Z")
      : undefined;
    const label = (minute: number) =>
      origin === undefined
        ? minuteLabel(minute)
        : new Date(origin + Math.round(minute) * 60000)
            .toISOString()
            .slice(11, 16);
    const units = [
      day.pressure_fields[0].unit +
        (this.simple ? "" : " · " + day.pressure_fields[0].meaning_evidence),
      ...day.channels.map(
        (c) =>
          (c.unit || "Исходные значения") +
          (this.simple ? "" : " · " + c.meaning_evidence),
      ),
      "",
    ];
    const titles = [
      this.simple
        ? "Давление"
        : day.fields.pressure_1?.Label || "pressure_1 / pressure_2",
      ...day.channels.map((c) => day.fields[c.field]?.Label || c.field),
      "События",
    ];
    const rows = [...new Set(day.events.map((e) => e.code))].sort(
      (a, b) => a - b,
    );
    const codes = rows.length ? rows : [0];
    const series: NonNullable<EChartsOption["series"]> = [
      ...[0, 1].map((axis) => ({
        name: "Ряд " + (axis + 1),
        type: "line" as const,
        xAxisIndex: 0,
        yAxisIndex: 0,
        symbol: axis ? "diamond" : "circle",
        symbolSize: axis ? 6 : 5,
        showSymbol: true,
        connectNulls: false,
        smooth: false,
        lineStyle: { width: 1.4 },
        itemStyle: { color: colors[axis] },
        data: channelData(
          day.pressure.map((p) => ({
            ...p,
            raw: axis ? p.second_raw : p.first_raw,
          })),
          new Set(
            day.diagnostics
              .filter(
                (d) => d.field === "pressure_1" && d.code === "time_reversal",
              )
              .map((d) => d.index!),
          ),
          day.pressure_fields[axis].scale,
        ),
      })),
      ...(options.overview
        ? []
        : day.channels.map((ch, i) => ({
            name: ch.field,
            type: "line" as const,
            xAxisIndex: i + 1,
            yAxisIndex: i + 1,
            data: channelData(
              ch.points,
              new Set(
                day.diagnostics
                  .filter(
                    (d) =>
                      d.field === ch.field &&
                      ["sample_gap", "time_reversal"].includes(d.code),
                  )
                  .map((d) => d.index!),
              ),
              ch.scale,
            ),
            symbol: "circle",
            symbolSize: 4,
            showSymbol: true,
            connectNulls: false,
            smooth: false,
            lineStyle: { width: 1.4 },
            itemStyle: { color: colors[i + 2] },
          }))),
      {
        name: "События",
        type: "scatter",
        xAxisIndex: allAxes.length - 1,
        yAxisIndex: allAxes.length - 1,
        symbol: "rect",
        symbolSize: [4, 15],
        itemStyle: { color: "#a66f4b" },
        data: day.events.map((e) => [
          e.minute,
          this.simple ? 0 : codes.indexOf(e.code),
          e.index,
        ]),
      },
    ];
    const option: EChartsOption = {
      animation: false,
      textStyle: { fontFamily: "Inter Variable, sans-serif", color: "#617168" },
      title: visible.map((original, i) => ({
        text: titles[original],
        subtext:
          original === 6 && this.simple
            ? ""
            : units[original] ||
              (original === 6 ? "Код события" : "Исходные значения"),
        left: 0,
        top: i * 168 + 4,
        textStyle: { fontSize: 12, fontWeight: 500, color: "#25362e" },
        subtextStyle: { fontSize: 10 },
        itemGap: 4,
      })),
      legend: {
        data: ["Ряд 1", "Ряд 2"],
        top: 0,
        right: 0,
        textStyle: { fontSize: 10 },
        itemWidth: 10,
        itemHeight: 8,
      },
      grid: allAxes.map((i) => ({
        top: i * 168 + 46,
        left: 48,
        right: 12,
        height: 91,
      })),
      xAxis: allAxes.map((i) => ({
        gridIndex: i,
        type: "value",
        min: 0,
        max: Math.max(1, options.span ?? 1439),
        minInterval: 1,
        splitNumber: 6,
        axisLabel: {
          formatter: label,
          fontSize: 9,
          color: "#617168",
          hideOverlap: true,
        },
        axisLine: { show: true, lineStyle: { color: "#dce4de" } },
        axisTick: { show: false },
        splitLine: { lineStyle: { color: "#eef1ed" } },
        axisPointer: { show: true, snap: false, label: { show: false } },
      })),
      yAxis: allAxes.map((i) =>
        i === allAxes.length - 1
          ? {
              gridIndex: i,
              type: "category",
              data: this.simple ? [""] : codes.map(String),
              axisTick: { show: false },
              axisLine: { show: false },
              axisLabel: { fontSize: 9 },
              splitLine: { show: true, lineStyle: { color: "#edf1eb" } },
            }
          : {
              gridIndex: i,
              type: "value",
              scale: true,
              splitNumber: 3,
              axisLabel: { fontSize: 9, hideOverlap: true },
              splitLine: { lineStyle: { color: "#edf1eb" } },
            },
      ),
      axisPointer: {
        link: [{ xAxisIndex: allAxes }],
        snap: false,
        lineStyle: { color: "#7e9c87", width: 1 },
      },
      tooltip: { show: false, trigger: "axis" },
      dataZoom: [
        {
          type: "inside",
          xAxisIndex: allAxes,
          filterMode: "none",
          zoomOnMouseWheel: "shift",
          moveOnMouseMove: true,
        },
        {
          type: "slider",
          xAxisIndex: allAxes,
          filterMode: "none",
          bottom: 8,
          height: 22,
          showDetail: false,
          borderColor: "#dce4de",
          fillerColor: "#256b501a",
          handleStyle: { color: "#256b50" },
          labelFormatter: label,
        },
      ],
      series,
    };
    this.chart.setOption(option, true);
  }
  reset() {
    this.chart.dispatchAction({ type: "dataZoom", start: 0, end: 100 });
  }
  clear() {
    this.day = undefined;
    this.tooltip.hidden = true;
    this.tooltip.textContent = "";
    this.chart.clear();
  }
  refresh() {
    this.chart.resize();
  }
}
