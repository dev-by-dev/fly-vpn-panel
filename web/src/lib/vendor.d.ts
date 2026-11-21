// Minimal ambient types for untyped browser deps (uplot ships no .d.ts,
// qrcode used only via toDataURL).
declare module 'uplot' {
  export default class uPlot {
    constructor(opts: Record<string, unknown>, data?: number[][], target?: HTMLElement);
    setData(data: number[][]): void;
    destroy(): void;
  }
}

declare module 'qrcode' {
  export function toDataURL(text: string, opts?: Record<string, unknown>): Promise<string>;
}
