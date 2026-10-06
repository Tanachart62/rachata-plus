import type { Video } from '../types'
import { ApiError, isObject, positiveId } from './http'
export function readVideo(value: unknown): Video {
  if (
    !isObject(value) ||
    !positiveId(value.id) ||
    typeof value.title !== 'string' ||
    typeof value.description !== 'string' ||
    typeof value.category !== 'string' ||
    !Number.isSafeInteger(value.durationSeconds) ||
    (value.durationSeconds as number) < 0 ||
    !['draft', 'published', 'hidden'].includes(value.publicationStatus as string) ||
    !['pending', 'processing', 'ready', 'failed'].includes(value.processingStatus as string)
  )
    throw new ApiError('service_unavailable')
  for (const field of ['playbackUrl', 'previewUrl']) {
    if (
      value[field] !== undefined &&
      (typeof value[field] !== 'string' ||
        !/^\/media\/\d+\/(full|preview)\/index\.m3u8$/.test(value[field] as string))
    )
      throw new ApiError('service_unavailable')
  }
  if (value.processingError !== undefined && typeof value.processingError !== 'string')
    throw new ApiError('service_unavailable')
  return {
    id: value.id,
    title: value.title,
    description: value.description,
    category: value.category,
    durationSeconds: value.durationSeconds as number,
    publicationStatus: value.publicationStatus as Video['publicationStatus'],
    processingStatus: value.processingStatus as Video['processingStatus'],
    processingError: value.processingError as string | undefined,
    playbackUrl: value.playbackUrl as string | undefined,
    previewUrl: value.previewUrl as string | undefined,
  }
}
