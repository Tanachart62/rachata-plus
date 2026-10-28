import { ApiError, isObject, jsonRequest } from './http'
import { coverBase64 } from './cover-image'
import { readVideo } from './video-schema'
import type { Video } from '../types'

export async function directUpload(file: File, title: string, description: string, category: string, progress: (percent: number) => void, signal: AbortSignal, cover?: File | null): Promise<Video | null> {
  const result = await jsonRequest('/api/admin/uploads', { body: { filename: file.name, size: file.size, title, description, category }, signal })
  if (!isObject(result)) throw new ApiError('invalid_upload')
  if (result.mode === 'local') return null
  if (result.mode !== 's3' || typeof result.id !== 'string' || !/^[a-f0-9]{32}$/.test(result.id) ||
    !Number.isSafeInteger(result.partSize) || (result.partSize as number) < 5 * 1024 * 1024 ||
    !Number.isSafeInteger(result.parts) || (result.parts as number) < 1 || (result.parts as number) > 10000) throw new ApiError('invalid_upload')
  const endpoint = `/api/admin/uploads/${result.id}`
  const partSize = result.partSize as number
  const parts = result.parts as number
  let completing = false
  try {
    for (let number = 1; number <= parts; number++) {
      const chunk = file.slice((number - 1) * partSize, Math.min(number * partSize, file.size))
      for (let attempt = 0; ; attempt++) {
        signal.throwIfAborted()
        try {
          const signed = await jsonRequest(endpoint + '/part', { body: { part: number }, signal })
          if (!isObject(signed) || typeof signed.url !== 'string') throw new ApiError('invalid_upload')
          const url = new URL(signed.url)
          if (url.protocol !== 'https:' && !['localhost', '127.0.0.1'].includes(url.hostname)) throw new ApiError('invalid_upload')
          const response = await fetch(url, { method: 'PUT', body: chunk, signal, credentials: 'omit', referrerPolicy: 'no-referrer' })
          if (!response.ok) throw new ApiError('part_upload_failed')
          break
        } catch (error) {
          if (signal.aborted || attempt >= 2) throw error
          await new Promise(resolve => window.setTimeout(resolve, 500 * (attempt + 1)))
        }
      }
      progress(Math.min(99, Math.floor(Math.min(number * partSize, file.size) / file.size * 100)))
    }
    const encodedCover = cover ? await coverBase64(cover) : ''
    completing = true
    for (let attempt = 0; ; attempt++) {
      try {
        const completed = await jsonRequest(endpoint + '/complete', { body: { cover: encodedCover }, signal })
        if (!isObject(completed)) throw new ApiError('invalid_upload')
        progress(100)
        return readVideo(completed.video)
      } catch (error) { if (signal.aborted || attempt >= 2) throw error }
    }
  } finally {
    // Never abort an uncertain Complete: its DB transaction may have committed.
    if (!completing) await jsonRequest(endpoint, { method: 'DELETE', body: {} }).catch(() => {})
  }
}
