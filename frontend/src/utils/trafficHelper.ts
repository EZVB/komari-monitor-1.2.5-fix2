export type TrafficLimitType = "sum" | "max" | "min" | "up" | "down";

export function normalizeTrafficMultiplier(value: unknown): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 1;
}

export function applyTrafficMultiplier(
  bytes: number,
  multiplier: unknown,
): number {
  return bytes * normalizeTrafficMultiplier(multiplier);
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
