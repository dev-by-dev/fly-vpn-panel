import { test, expect } from 'vitest';
import { toSeries, fmtUptime } from './traffic';

// Anonymized fixtures: TEST-NET IP, RFC3339 timestamps, fake user.
const buckets = [
  { TS: '2026-11-20T10:00:00Z', Up: 1024, Down: 2048 },
  { TS: '2026-11-20T11:00:00Z', Up: 4096, Down: 8192 },
];

test('toSeries: buckets → [timestamps, up, down] в секундах', () => {
  const [ts, up, down] = toSeries(buckets);
  expect(ts).toEqual([Date.parse('2026-11-20T10:00:00Z') / 1000, Date.parse('2026-11-20T11:00:00Z') / 1000]);
  expect(up).toEqual([1024, 4096]);
  expect(down).toEqual([2048, 8192]);
});

test('toSeries: пустые данные → нули', () => {
  const [ts, up, down] = toSeries([]);
  expect(ts).toEqual([0]);
  expect(up).toEqual([0]);
  expect(down).toEqual([0]);
});

test('fmtUptime: наносекунды из Go time.Duration → человекочитаемо', () => {
  expect(fmtUptime(0)).toBe('0с');
  expect(fmtUptime(90e9)).toBe('1м 30с');
  expect(fmtUptime(26 * 3600e9)).toBe('1д 2ч');
  expect(fmtUptime(3600e9 + 60e9)).toBe('1ч 1м');
});
