import * as echarts from "echarts/core";
import { LineChart, ScatterChart } from "echarts/charts";
import {
  GridComponent,
  TooltipComponent,
  DataZoomComponent,
  TitleComponent,
} from "echarts/components";
import { CanvasRenderer } from "echarts/renderers";
import type { EChartsOption } from "echarts";
import type { Day, Point } from "./types";
import { createMetricHover, type MetricReading } from "./metric-hover";
import { EventTimeline } from "./event-timeline";

echarts.use([
  LineChart,
  ScatterChart,
  GridComponent,
  TooltipComponent,
  DataZoomComponent,
  TitleComponent,
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
// Описания объясняют общий смысл показателей, но не подтверждают трактовку
// каналов RC-C60: степень уверенности остаётся в подписях из Go-библиотеки.
const descriptions: Record<string, string> = {
  pressure: "Давление воздуха помогает держать дыхательные пути открытыми.",
  channel_1:
    "Показывает, сколько воздуха уходит из маски и дыхательного контура.",
  channel_2: "Объём воздуха за один вдох — показатель глубины дыхания.",
  channel_3: "Объём воздуха за минуту: зависит от глубины и частоты дыхания.",
  channel_4: "Число дыханий в минуту показывает, как меняется темп дыхания.",
  channel_5:
    "Дополнительный сигнал аппарата. Его назначение пока не установлено.",
};
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

export function pressureLines(
  day: Pick<PlotData, "pressure" | "pressure_fields" | "diagnostics">,
) {
  const primary: (number | null)[][] = [];
  const secondary: (number | null)[][] = [];
  const breaks = new Set(
    day.diagnostics
      .filter(
        (d) =>
          d.field === "pressure_1" &&
          ["time_reversal", "sample_gap"].includes(d.code),
      )
      .map((d) => d.index),
  );
  let previous: number[] | undefined;
  let previousDifferent = false;
  let lastSecondary: number[] | undefined;
  let hasDifference = false;
  for (const p of day.pressure) {
    const first = p.first_raw * day.pressure_fields[0].scale;
    const second = p.second_raw * day.pressure_fields[1].scale;
    const sample = [p.minute, second, p.index];
    const different = first !== second;
    if (p.break_before || breaks.has(p.index)) {
      primary.push([p.minute, null, -1]);
      previous = undefined;
      lastSecondary = undefined;
      previousDifferent = false;
    }
    primary.push([p.minute, first, p.index]);
    // Участок расхождения включает общие точки на обоих концах. Между
    // участками ставим разрыв, чтобы пунктир не перекрашивал совпадающую линию.
    if (
      different &&
      !previousDifferent &&
      (!previous || previous !== lastSecondary)
    ) {
      if (secondary.length) secondary.push([p.minute, null, -1]);
      if (previous) secondary.push(previous);
    }
    if (different || previousDifferent) {
      secondary.push(sample);
      lastSecondary = sample;
    }
    hasDifference ||= different;
    previous = sample;
    previousDifferent = different;
  }
  return hasDifference ? [primary, secondary] : [primary];
}

export function tooltipText(
  day: PlotData,
  minute: number,
  simple = false,
  options: { origin?: string } = {},
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
  for (const ch of day.channels) {
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
  private axisCount = 7;
  private eventAxis = 6;
  private timelineTop = 0;
  private fifth = document.createElement("details");
  private lastReading?: MetricReading;
  private metricHover?: ReturnType<typeof createMetricHover>;
  private rowHeight = 168;
  private plotTop = 46;
  private plotHeight = 91;
  private tooltip = document.createElement("div");
  private cursor = document.createElement("div");
  private descriptionArea = document.createElement("div");
  private timeline = new EventTimeline(
    (text, x, y) => {
      this.lastReading = undefined;
      this.cursor.hidden = true;
      this.tooltip.textContent = text;
      this.tooltip.hidden = false;
      this.positionReadout(x, y);
    },
    () => this.hideTooltip(),
  );
  constructor(private element: HTMLElement) {
    this.chart = echarts.init(element, undefined, { renderer: "canvas" });
    this.tooltip.className = "chart-tooltip";
    this.tooltip.setAttribute("role", "tooltip");
    this.tooltip.hidden = true;
    this.cursor.className = "chart-cursor";
    this.cursor.setAttribute("aria-hidden", "true");
    this.cursor.hidden = true;
    this.descriptionArea.className = "chart-descriptions";
    this.fifth.className = "fifth-channel";
    const summary = document.createElement("summary");
    summary.textContent = "Пятый канал";
    this.fifth.append(summary);
    this.fifth.hidden = true;
    element.append(
      this.descriptionArea,
      this.fifth,
      this.timeline.element,
      this.tooltip,
      this.cursor,
    );
    this.resize = new ResizeObserver(() => {
      this.hideTooltip();
      if (this.simple && this.day) this.fitTimeline();
      this.chart.resize();
      this.layoutMarks();
    });
    const pointer = (event: { offsetX: number; offsetY: number }) => {
      const point = [event.offsetX, event.offsetY];
      const grid = Array.from(
        { length: this.axisCount },
        (_, i) => i,
      ).findIndex((gridIndex) => this.chart.containPixel({ gridIndex }, point));
      if (!this.day || grid < 0 || (this.simple && grid === this.eventAxis)) {
        this.hideTooltip();
        return;
      }
      const position = this.chart.convertFromPixel(
        { xAxisIndex: grid },
        event.offsetX,
      ) as number;
      if (this.simple) {
        const radius = Math.abs(
          (this.chart.convertFromPixel(
            { xAxisIndex: grid },
            event.offsetX + 8,
          ) as number) - position,
        );
        const reading = this.metricHover?.(grid, position, radius);
        if (!reading) {
          this.hideTooltip();
          return;
        }
        if (reading !== this.lastReading) {
          this.showReading(reading);
          this.lastReading = reading;
        }
        this.tooltip.hidden = false;
        const rect = element.getBoundingClientRect();
        this.positionReadout(
          rect.left + event.offsetX,
          rect.top + event.offsetY,
        );
        const x = this.chart.convertToPixel(
          { xAxisIndex: grid },
          reading.minute,
        ) as number;
        this.cursor.style.left = x + "px";
        this.cursor.style.top = grid * this.rowHeight + this.plotTop + "px";
        this.cursor.style.height = this.plotHeight + "px";
        this.cursor.hidden = x < 48 || x > element.clientWidth - 12;
        return;
      }
      // ECharts привязывает штатную подсказку к ближайшей точке даже при snap:false.
      // Переводим положение курсора сами: в разрыве нельзя показывать чужой отсчёт.
      const minute = Math.round(position);
      this.tooltip.textContent = tooltipText(this.day, minute, this.simple, {
        origin: this.origin,
      });
      this.tooltip.hidden = !this.tooltip.textContent;
      if (this.tooltip.hidden) return;
      this.tooltip.style.right = "auto";
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
    this.chart.getZr().on("globalout", () => this.hideTooltip());
    this.chart.on("datazoom", () => {
      this.hideTooltip();
      this.layoutMarks();
    });
    // Внутренний контроллер ECharts отменяет wheel раньше проверки Shift.
    // Обычное колесо не передаём ему и не отменяем: страницу прокручивает браузер.
    element.addEventListener(
      "wheel",
      (event) => {
        if (!event.shiftKey) {
          event.stopPropagation();
          this.hideTooltip();
        }
      },
      { capture: true, passive: true },
    );
    window.addEventListener("scroll", () => this.hideTooltip(), {
      capture: true,
      passive: true,
    });
    this.resize.observe(element);
    this.resize.observe(this.timeline.element);
    window.addEventListener("pointerdown", () => this.hideTooltip(), {
      capture: true,
      passive: true,
    });
    // Safari может оставить фокус вне кнопки после нажатия мышью.
    window.addEventListener("keydown", (event) => {
      if (event.key === "Escape") this.hideTooltip();
    });
  }
  show(
    day: PlotData,
    options: {
      origin?: string;
      span?: number;
      simple?: boolean;
    } = {},
    zoom?: { start: number; end: number },
  ) {
    this.day = day;
    this.hideTooltip();
    this.simple = options.simple || false;
    this.origin = options.origin;
    this.metricHover = this.simple
      ? createMetricHover(day, options.origin)
      : undefined;
    this.tooltip.classList.toggle("chart-readout", this.simple);
    this.rowHeight = this.simple ? 360 : 168;
    this.plotTop = this.simple ? 112 : 46;
    this.plotHeight = this.simple ? 216 : 91;
    const channels = day.channels.filter(
      (ch) => !this.simple || ch.field !== "channel_5" || this.fifth.open,
    );
    const metrics = [0, ...channels.map((_, i) => i + 1)];
    this.eventAxis = metrics.length;
    const allAxes =
      this.simple && !day.events.length
        ? metrics
        : [...metrics, this.eventAxis];
    this.axisCount = allAxes.length;
    const origin = options.origin
      ? Date.parse(options.origin + "Z")
      : undefined;
    const label = (minute: number) =>
      origin === undefined
        ? minuteLabel(minute)
        : new Date(origin + Math.round(minute) * 60000)
            .toISOString()
            .slice(11, 16);
    if (this.simple) {
      this.fifth.hidden = false;
      this.fifth.style.top = 5 * this.rowHeight + "px";
      this.fifth.ontoggle = () => {
        if (
          !this.day ||
          channels.some((ch) => ch.field === "channel_5") === this.fifth.open
        )
          return;
        // Раскрытие меняет состав осей. Возвращаем тот же интервал времени.
        const dataZoom = (this.chart.getOption() as EChartsOption).dataZoom;
        const range = Array.isArray(dataZoom) ? dataZoom[0] : dataZoom;
        this.show(day, options, {
          start: range?.start ?? 0,
          end: range?.end ?? 100,
        });
      };
      this.timelineTop =
        metrics.length * this.rowHeight + (this.fifth.open ? 0 : 48);
      this.timeline.element.style.top = this.timelineTop + "px";
      if (!zoom) this.timeline.show(day.events, label);
      this.fitTimeline();
    } else {
      this.fifth.hidden = true;
      this.timeline.clear();
      this.element.style.height = this.rowHeight * allAxes.length + "px";
    }
    this.chart.resize();
    const subtitles = this.simple
      ? []
      : [
          ...[day.pressure_fields[0], ...channels].map(
            (field) =>
              `${field.unit || "Исходные значения"} · ${field.meaning_evidence}`,
          ),
          "Код события",
        ];
    const titles = [
      this.simple
        ? "Давление"
        : day.fields.pressure_1?.Label || "pressure_1 / pressure_2",
      ...channels.map((c) => day.fields[c.field]?.Label || c.field),
      "События",
    ];
    this.descriptionArea.replaceChildren();
    const pressure = pressureLines(day);
    if (this.simple) {
      ["pressure", ...channels.map((ch) => ch.field)].forEach((field, i) => {
        const paragraph = document.createElement("p");
        paragraph.className = "chart-description";
        paragraph.style.top =
          i * this.rowHeight + (field === "channel_5" ? 42 : 30) + "px";
        paragraph.textContent =
          descriptions[field] ||
          "Назначение этого сигнала пока не установлено.";
        if (field === "pressure" && pressure.length > 1)
          paragraph.append(
            " Пунктир показывает второе записанное значение там, где оно отличается.",
          );
        this.descriptionArea.append(paragraph);
      });
    }
    const rows = this.simple
      ? []
      : [...new Set(day.events.map((e) => e.code))].sort((a, b) => a - b);
    const codes = rows.length ? rows : [0];
    const series: NonNullable<EChartsOption["series"]> = [
      ...pressure.map((data, axis) => ({
        name: axis === 0 ? "Давление" : "Второе значение давления",
        type: "line" as const,
        xAxisIndex: 0,
        yAxisIndex: 0,
        // Плотные измерения читаются как линия; маркеры не перекрывают соседние значения.
        symbol: "none",
        showSymbol: false,
        connectNulls: false,
        smooth: false,
        lineStyle: {
          width: this.simple ? 1.8 : 1.4,
          type: axis === 0 ? ("solid" as const) : ("dashed" as const),
        },
        itemStyle: { color: colors[axis] },
        data,
      })),
      ...channels.map((ch, i) => ({
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
        symbol: "none",
        showSymbol: false,
        connectNulls: false,
        smooth: false,
        lineStyle: { width: this.simple ? 1.8 : 1.4 },
        itemStyle: { color: colors[i + 2] },
      })),
    ];
    if (!this.simple) {
      series.push({
        name: "События",
        type: "scatter",
        xAxisIndex: this.eventAxis,
        yAxisIndex: this.eventAxis,
        symbol: "rect",
        symbolSize: [4, 15],
        itemStyle: { color: "#a66f4b" },
        data: day.events.map((e) => [e.minute, codes.indexOf(e.code), e.index]),
      });
    }
    const option: EChartsOption = {
      animation: false,
      textStyle: { fontFamily: "Inter Variable, sans-serif", color: "#617168" },
      title: (this.simple
        ? metrics.filter((i) => channels[i - 1]?.field !== "channel_5")
        : allAxes
      ).map((i) => ({
        text: titles[i],
        subtext: subtitles[i],
        left: 0,
        top: i * this.rowHeight + 4,
        textStyle: {
          fontSize: this.simple ? 14 : 12,
          fontWeight: 500,
          color: "#25362e",
        },
        subtextStyle: { fontSize: 10 },
        itemGap: 4,
      })),
      grid: allAxes.map((i) => ({
        top:
          this.simple && i === this.eventAxis
            ? this.timelineTop + 60
            : i * this.rowHeight + this.plotTop,
        left: 48,
        right: 12,
        ...(this.simple && i === this.eventAxis
          ? { bottom: 26 }
          : { height: this.plotHeight }),
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
          fontSize: this.simple ? 11 : 9,
          color: "#617168",
          hideOverlap: true,
        },
        axisLine: { show: true, lineStyle: { color: "#dce4de" } },
        axisTick: { show: false },
        splitLine: { lineStyle: { color: "#eef1ed" } },
        axisPointer: {
          show: !this.simple,
          snap: false,
          label: { show: false },
        },
      })),
      yAxis: allAxes.map((i) =>
        i === this.eventAxis
          ? {
              gridIndex: i,
              type: "category",
              data: this.simple ? [""] : codes.map(String),
              axisTick: { show: false },
              axisLine: { show: false },
              axisLabel: { fontSize: this.simple ? 11 : 9 },
              splitLine: {
                show: !this.simple,
                lineStyle: { color: "#edf1eb" },
              },
            }
          : {
              gridIndex: i,
              type: "value",
              scale: true,
              splitNumber: 3,
              axisLabel: { fontSize: this.simple ? 11 : 9, hideOverlap: true },
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
          ...zoom,
          xAxisIndex: allAxes,
          filterMode: "none",
          zoomOnMouseWheel: "shift",
          moveOnMouseMove: true,
        },
      ],
      series,
    };
    this.chart.setOption(option, true);
    this.layoutMarks();
  }
  private fitTimeline() {
    const height =
      this.timelineTop +
      this.timeline.element.offsetHeight +
      (this.day?.events.length ? 34 : 0);
    this.element.style.height = height + "px";
  }
  private layoutMarks() {
    if (!this.simple || !this.day?.events.length) return;
    // Координаты берём у общей оси после масштабирования и изменения ширины.
    const axis = { xAxisIndex: this.eventAxis };
    const right = this.element.clientWidth - 12;
    this.timeline.layout(
      (minute) => this.chart.convertToPixel(axis, minute),
      (x) => x >= 48 - 0.01 && x <= right + 0.01,
    );
  }
  reset() {
    this.chart.dispatchAction({ type: "dataZoom", start: 0, end: 100 });
  }
  clear() {
    this.day = undefined;
    this.metricHover = undefined;
    this.hideTooltip();
    this.tooltip.textContent = "";
    this.fifth.ontoggle = null;
    this.fifth.open = false;
    this.fifth.hidden = true;
    this.descriptionArea.replaceChildren();
    this.timeline.clear();
    this.chart.clear();
  }
  refresh() {
    this.chart.resize();
  }
  private hideTooltip() {
    this.lastReading = undefined;
    this.tooltip.hidden = true;
    this.cursor.hidden = true;
  }
  private showReading(reading: MetricReading) {
    const time = document.createElement("time");
    time.className = "reading-time";
    time.textContent = reading.time;
    const unit = document.createElement("small");
    unit.className = "reading-unit";
    unit.textContent = reading.unit;
    const rows = reading.values.map((values) => {
      const row = document.createElement("div");
      row.className = "reading-values";
      values.forEach((value, i) => {
        const number = document.createElement("strong");
        number.textContent = value;
        if (values.length > 1)
          number.className = i === 0 ? "pressure-first" : "pressure-second";
        row.append(number);
      });
      return row;
    });
    rows.at(-1)?.append(unit);
    this.tooltip.replaceChildren(time, ...rows);
  }
  private positionReadout(x: number, y: number) {
    const width = this.tooltip.offsetWidth,
      height = this.tooltip.offsetHeight;
    // У края экрана переносим подсказку на другую сторону курсора.
    // Она не перехватывает мышь и остаётся целиком внутри видимой области.
    const left = x + width + 16 <= innerWidth - 8 ? x + 16 : x - width - 16;
    const top = y + height + 16 <= innerHeight - 8 ? y + 16 : y - height - 16;
    this.tooltip.style.right = "auto";
    this.tooltip.style.left =
      Math.max(8, Math.min(left, innerWidth - width - 8)) + "px";
    this.tooltip.style.top =
      Math.max(8, Math.min(top, innerHeight - height - 8)) + "px";
  }
}
