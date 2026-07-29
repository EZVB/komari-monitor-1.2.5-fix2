export type TrafficLimitType = "sum" | "max" | "min" | "up" | "down";

export function normalizeTrafficMultiplier(value: unknown): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : 0;
}

export function getTrafficMultiplierFactor(value: unknown): number {
  return 1 + normalizeTrafficMultiplier(value);
}

export function applyTrafficMultiplier(
  bytes: number,
  multiplier: unknown,
): number {
  return bytes * getTrafficMultiplierFactor(multiplier);
}

export function getTrafficUsage(
  totalUp: number,
  totalDown: number,
  type: TrafficLimitType | undefined,
  multiplier: unknown,
): number {
  let usage: number;

  switch (type ?? "sum") {
    case "max":
      usage = Math.max(totalUp, totalDown);
      break;
    case "min":
      usage = Math.min(totalUp, totalDown);
      break;
    case "up":
      usage = totalUp;
      break;
    case "down":
      usage = totalDown;
      break;
    case "sum":
    default:
      usage = totalUp + totalDown;
      break;
  }

  return applyTrafficMultiplier(usage, multiplier);
}
