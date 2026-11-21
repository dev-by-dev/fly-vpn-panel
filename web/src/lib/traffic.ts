// Bucket mirrors store.Bucket (Go, no json tags → capitalized keys).
export type Bucket = { TS: string; Up: number; Down: number };

// buckets → uPlot data [timestamps(sec), up, down]; empty → zeros.
export function toSeries(buckets: Bucket[]): number[][] {
  if (buckets.length === 0) return [[0], [0], [0]];
  const sorted = [...buckets].sort((a, b) => Date.parse(a.TS) - Date.parse(b.TS));
  return [
    sorted.map((b) => Date.parse(b.TS) / 1000),
    sorted.map((b) => b.Up),
    sorted.map((b) => b.Down),
  ];
}

// xray Status.Uptime arrives as Go time.Duration (integer nanoseconds).
export function fmtUptime(ns: number): string {
  const s = Math.floor(ns / 1e9);
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (d > 0) return `${d}д ${h}ч`;
  if (h > 0) return `${h}ч ${m}м`;
  if (m > 0) return `${m}м ${s % 60}с`;
  return `${s}с`;
}

export function fmtBytes(n: number): string {
  if (n <= 0) return '0 Б';
  const units = ['Б', 'КБ', 'МБ', 'ГБ', 'ТБ'];
  const i = Math.min(units.length - 1, Math.floor(Math.log(n) / Math.log(1024)));
  const v = (n / 1024 ** i).toFixed(i === 0 ? 0 : 1).replace(/\.0$/, '');
  return `${v} ${units[i]}`;
}
