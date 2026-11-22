import { t } from './i18n.svelte';

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
  if (d > 0) return `${d}${t('units.d')} ${h}${t('units.h')}`;
  if (h > 0) return `${h}${t('units.h')} ${m}${t('units.m')}`;
  if (m > 0) return `${m}${t('units.m')} ${s % 60}${t('units.s')}`;
  return `${s}${t('units.s')}`;
}

export function fmtBytes(n: number): string {
  const units = [
    t('units.byte'),
    t('units.kb'),
    t('units.mb'),
    t('units.gb'),
    t('units.tb'),
  ];
  if (n <= 0) return `0 ${units[0]}`;
  const i = Math.min(units.length - 1, Math.floor(Math.log(n) / Math.log(1024)));
  const v = (n / 1024 ** i).toFixed(i === 0 ? 0 : 1).replace(/\.0$/, '');
  return `${v} ${units[i]}`;
}

// Bits per second → human bit rate (сеть принято мерить в битах).
export function fmtRate(bps: number): string {
  if (!(bps > 0)) return `0 ${t('units.bps')}`;
  if (bps < 1e3) return `${Math.round(bps)} ${t('units.bps')}`;
  const units = [t('units.kbps'), t('units.mbps'), t('units.gbps')];
  const i = Math.min(units.length - 1, Math.floor(Math.log(bps) / Math.log(1000)) - 1);
  return `${(bps / 1000 ** (i + 1)).toFixed(1).replace(/\.0$/, '')} ${units[i]}`;
}
