export function durationLabel(seconds: number) {
  const value = Math.max(0, Math.floor(seconds))
  return `${Math.floor(value / 60)} นาที ${value % 60} วินาที`
}
