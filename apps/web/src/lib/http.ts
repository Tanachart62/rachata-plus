export class ApiError extends Error {
  code: string
  constructor(code: string) {
    super(code)
    this.code = code
  }
}
export async function jsonRequest(
  path: string,
  options: { method?: string; body?: unknown; signal?: AbortSignal; keepalive?: boolean } = {},
): Promise<unknown> {
  const controller = new AbortController()
  const timer = window.setTimeout(() => controller.abort(), 10000)
  try {
    const response = await fetch(path, {
      method: options.method ?? (options.body === undefined ? 'GET' : 'POST'),
      credentials: 'same-origin',
      headers: options.body === undefined ? undefined : { 'Content-Type': 'application/json' },
      body: options.body === undefined ? undefined : JSON.stringify(options.body),
      signal: options.signal
        ? AbortSignal.any([options.signal, controller.signal])
        : controller.signal,
      keepalive: options.keepalive,
    })
    if (response.status === 204) return null
    const data: unknown = await response.json()
    if (!response.ok)
      throw new ApiError(
        isObject(data) && typeof data.error === 'string' ? data.error : 'service_unavailable',
      )
    return data
  } catch (error) {
    if (options.signal?.aborted) throw options.signal.reason
    if (error instanceof ApiError) throw error
    throw new ApiError('service_unavailable')
  } finally {
    window.clearTimeout(timer)
  }
}
export function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}
export function positiveId(value: unknown): value is number {
  return Number.isSafeInteger(value) && (value as number) > 0
}
