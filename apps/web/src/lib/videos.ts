import type { PublicationStatus, Video } from '../types'
import { apiAuthEnabled } from './auth'
import { readVideo } from './video-schema'
import { ApiError, isObject, jsonRequest } from './http'
import { prepareCover } from './cover-image'
import { directUpload } from './s3-upload'

export const apiVideosEnabled = apiAuthEnabled
export function videoError(code: string) {
  const messages: Record<string, string> = {
    unauthenticated: 'กรุณาเข้าสู่ระบบใหม่',
    admin_required: 'บัญชีนี้ไม่มีสิทธิ์แอดมิน',
    video_not_ready_or_missing: 'วิดีโอยังแปลงไม่เสร็จ หรือถูกลบแล้ว',
    video_not_retryable: 'วิดีโอนี้ยังไม่พร้อมให้ลองแปลงใหม่',
    invalid_video_file: 'เลือกไฟล์ MP4, WebM หรือ MOV ที่ไม่ใช่ไฟล์ว่าง',
    invalid_cover: 'เลือกภาพ JPEG หรือ PNG ที่เปิดอ่านได้',
    invalid_metadata: 'กรุณาตรวจชื่อและหมวดหมู่วิดีโอ',
    too_many_uploads: 'มีงานอัปโหลดค้างอยู่ครบ 3 งานแล้ว กรุณารอหรือยกเลิกงานเดิมก่อน',
    direct_upload_required: 'กรุณารีเฟรชหน้าเพื่อใช้การอัปโหลดเข้า S3',
    part_upload_failed: 'ส่งไฟล์บางส่วนไป S3 ไม่สำเร็จ กรุณาตรวจการเชื่อมต่อแล้วลองใหม่',
    video_edit_conflict: 'ข้อมูลวิดีโอเปลี่ยนแล้ว กรุณาปิดหน้าต่างและโหลดรายการใหม่ก่อนแก้ไข',
    account_changed: 'บัญชีที่กำลังใช้งานเปลี่ยนแล้ว กรุณาโหลดหน้าใหม่',
    storage_budget_exceeded: 'พื้นที่จัดเก็บไม่พอ กรุณาลบวิดีโอที่ไม่ใช้แล้วหรือติดต่อผู้ดูแล',
    upload_incomplete_or_too_large: 'อัปโหลดไม่ครบหรือเกินงบพื้นที่ของเครื่อง กรุณาลองใหม่',
  }
  return messages[code] ?? 'เชื่อมต่อระบบวิดีโอไม่สำเร็จ กรุณาลองอีกครั้ง'
}
async function request(path: string, method = 'GET', body?: unknown, signal?: AbortSignal) {
  try {
    const data = await jsonRequest(path, { method, body, signal })
    if (data !== null && !isObject(data)) throw new Error('ข้อมูลวิดีโอไม่ถูกต้อง')
    return data as { videos: Video[]; video: Video }
  } catch (error) {
    if (error instanceof ApiError) throw new Error(videoError(error.code))
    throw error
  }
}
export async function listVideos(admin: boolean, signal?: AbortSignal): Promise<Video[]> {
  const data = await request(admin ? '/api/admin/videos' : '/api/videos', 'GET', undefined, signal)
  if (!Array.isArray(data.videos)) throw new Error('ข้อมูลวิดีโอไม่ถูกต้อง')
  return data.videos.map(readVideo)
}
export async function getVideo(id: number, signal?: AbortSignal): Promise<Video> {
  return readVideo((await request(`/api/videos/${id}`, 'GET', undefined, signal)).video)
}
export async function publishVideo(
  id: number,
  publicationStatus: PublicationStatus,
): Promise<Video> {
  return readVideo((await request(`/api/admin/videos/${id}`, 'PATCH', { publicationStatus })).video)
}
export async function removeVideo(id: number) {
  await request(`/api/admin/videos/${id}`, 'DELETE')
}
export async function retryVideo(id: number) {
  await request(`/api/admin/videos/${id}/retry`, 'POST', {})
}
export async function editVideo(video: Video, actorId: number, title: string, description: string, category: string, cover: File | null, signal: AbortSignal): Promise<Video> {
  const body = new FormData()
  body.append('expectedUserId', String(actorId))
  body.append('expectedRevision', String(video.metadataRevision ?? 0))
  body.append('title', title)
  body.append('description', description)
  body.append('category', category)
  if (cover) body.append('cover', await prepareCover(cover))
  const response = await fetch(`/api/admin/videos/${video.id}/edit`, {
    method: 'POST', credentials: 'same-origin', body,
    signal: AbortSignal.any([signal, AbortSignal.timeout(60000)]),
  })
  const data: unknown = await response.json()
  if (!response.ok) throw new Error(videoError(isObject(data) && typeof data.error === 'string' ? data.error : 'service_unavailable'))
  if (!isObject(data)) throw new Error('ข้อมูลวิดีโอไม่ถูกต้อง')
  const next = readVideo(data.video)
  if (next.id !== video.id) throw new Error('ข้อมูลวิดีโอไม่ถูกต้อง')
  return next
}
export async function uploadVideo(
  file: File,
  title: string,
  description: string,
  category: string,
  progress: (percent: number) => void,
  signal: AbortSignal,
  cover?: File | null,
): Promise<Video> {
  const prepared = cover ? await prepareCover(cover) : null
  signal.throwIfAborted()
  try {
    const uploaded = await directUpload(file, title, description, category, progress, signal, prepared)
    if (uploaded) return uploaded
  } catch (error) {
    if (error instanceof ApiError) throw new Error(videoError(error.code))
    throw error
  }
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    const abort = () => xhr.abort()
    signal.addEventListener('abort', abort, { once: true })
    xhr.open('POST', '/api/admin/videos')
    xhr.timeout = 0
    xhr.upload.onprogress = (event) => {
      if (event.lengthComputable) progress(Math.round((event.loaded / event.total) * 100))
    }
    xhr.onload = () => {
      try {
        const data = JSON.parse(xhr.responseText)
        if (xhr.status !== 202) reject(new Error(videoError(data.error)))
        else resolve(readVideo(data.video))
      } catch {
        reject(new Error('เชื่อมต่อระบบวิดีโอไม่สำเร็จ'))
      }
    }
    xhr.onerror = xhr.ontimeout = () => reject(new Error('อัปโหลดไม่สำเร็จ กรุณาลองใหม่'))
    xhr.onabort = () => reject(new Error('ยกเลิกการส่งไฟล์แล้ว'))
    xhr.onloadend = () => signal.removeEventListener('abort', abort)
    const body = new FormData()
    body.append('title', title)
    body.append('description', description)
    body.append('category', category)
    body.append('file', file)
    if (prepared) body.append('cover', prepared)
    if (signal.aborted) {
      signal.removeEventListener('abort', abort)
      reject(new Error('ยกเลิกการส่งไฟล์แล้ว'))
      return
    }
    xhr.send(body)
  })
}
