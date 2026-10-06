import type { Video, WatchHistoryEntry } from '../types'
import { readVideo } from './video-schema'
import { ApiError, isObject, jsonRequest, positiveId } from './http'
export { ApiError as AuthError } from './http'
export const apiAuthEnabled = import.meta.env.VITE_AUTH_MODE !== 'demo'
export interface ApiUser {
  id: number
  username: string
  name: string
  email: string
  role: 'user' | 'admin'
  member: boolean
  plan: string
  expiresAt: number
  accountStatus: "active" | "suspended"
  membershipRevision: number
}
export interface AccountLibrary {
  userId: number
  saved: number[]
  history: WatchHistoryEntry[]
  savedNext: string
  historyNext: string
  videos: Video[]
}
export function readUser(data: unknown): ApiUser {
  const u = isObject(data) ? data.user : null
  if (
    !isObject(u) ||
    !positiveId(u.id) ||
    typeof u.username !== 'string' ||
    typeof u.name !== 'string' ||
    typeof u.email !== 'string' ||
    (u.role !== 'user' && u.role !== 'admin') ||
    (u.accountStatus !== undefined && u.accountStatus !== 'active' && u.accountStatus !== 'suspended') ||
    (u.plan !== undefined && !['', 'basic', 'standard', 'premium'].includes(u.plan as string)) ||
    (u.expiresAt !== undefined && (!Number.isSafeInteger(u.expiresAt) || (u.expiresAt as number) < 0)) ||
    (u.membershipRevision !== undefined && (!Number.isSafeInteger(u.membershipRevision) || (u.membershipRevision as number) < 0)) ||
    (u.member === true && (!u.plan || !Number.isSafeInteger(u.expiresAt) || (u.expiresAt as number) <= 0))
  )
    throw new ApiError('service_unavailable')
  return {
    id: u.id,
    username: u.username,
    name: u.name,
    email: u.email,
    role: u.role,
    member: u.member === true,
    accountStatus: u.accountStatus === "suspended" ? "suspended" : "active",
    plan: typeof u.plan === "string" ? u.plan : "",
    expiresAt: Number.isSafeInteger(u.expiresAt) ? u.expiresAt as number : 0,
    membershipRevision: Number.isSafeInteger(u.membershipRevision) ? u.membershipRevision as number : 0,
  }
}
function readHistory(e: unknown): WatchHistoryEntry {
  if (
    !isObject(e) ||
    !positiveId(e.videoId) ||
    !Number.isSafeInteger(e.position) ||
    (e.position as number) < 0 ||
    !Number.isSafeInteger(e.watchedAt) ||
    (e.watchedAt as number) < 0 ||
    !Number.isSafeInteger(e.revision) ||
    (e.revision as number) < 0
  )
    throw new ApiError('service_unavailable')
  return {
    videoId: e.videoId,
    position: e.position as number,
    watchedAt: e.watchedAt as number,
    revision: e.revision as number,
  }
}
function owner(data: unknown, expected: number): Record<string, unknown> {
  if (!isObject(data)) throw new ApiError('service_unavailable')
  if (data.userId !== expected) throw new ApiError('account_changed')
  return data
}
export async function registerAccount(
  username: string,
  name: string,
  email: string,
  password: string,
) {
  return readUser(
    await jsonRequest('/auth/register', { body: { username, name, email, password } }),
  )
}
export async function loginAccount(email: string, password: string) {
  return readUser(await jsonRequest('/auth/login', { body: { email, password } }))
}
export async function currentAccount(signal?: AbortSignal) {
  try {
    return readUser(await jsonRequest('/auth/me', { signal }))
  } catch (error) {
    if (error instanceof ApiError && error.code === 'unauthenticated') return null
    throw error
  }
}
export async function logoutAccount(expectedUserId: number) {
  await jsonRequest('/auth/logout', { body: { expectedUserId } })
}
export function accountError(error: unknown) {
  const code = error instanceof ApiError ? error.code : 'service_unavailable'
  const messages: Record<string, string> = {
    invalid_current_password: 'รหัสผ่านปัจจุบันไม่ถูกต้อง',
    email_already_exists: 'อีเมลนี้มีบัญชีใช้งานแล้ว',
    invalid_name: 'ชื่อ–นามสกุลต้องมี 1–100 ตัวอักษร',
    invalid_password_length: 'รหัสผ่านต้องมี 15–128 ตัวอักษร',
    password_unchanged: 'รหัสผ่านใหม่ต้องต่างจากรหัสผ่านปัจจุบัน',
    unauthenticated: 'เซสชันหมดอายุ กรุณาเข้าสู่ระบบใหม่',
    membership_required: 'ต้องมีสิทธิ์สมาชิกเพื่อบันทึกการรับชมวิดีโอเต็ม',
    account_suspended: 'บัญชีถูกระงับ กรุณาติดต่อผู้ดูแลระบบ',
    session_not_found: 'เซสชันนี้หมดอายุหรือถูกตัดไปแล้ว กรุณาโหลดรายการใหม่',
    cannot_suspend_self: 'ไม่สามารถระงับบัญชีแอดมินที่กำลังใช้งานอยู่',
    admin_required: 'บัญชีนี้ไม่มีสิทธิ์จัดการสมาชิก',
    user_not_found: 'ไม่พบบัญชีที่ใช้อีเมลนี้',
    membership_conflict: 'สิทธิ์ถูกแก้ไขแล้ว กรุณาค้นหาบัญชีใหม่ก่อนบันทึก',
    membership_too_long: 'วันหมดอายุรวมต้องไม่เกิน 730 วันนับจากวันนี้',
    too_many_requests: 'ส่งคำขอถี่เกินไป กรุณารอสักครู่แล้วลองใหม่',
    invalid_request: 'กรุณาตรวจข้อมูลที่กรอก',
    account_changed: 'บัญชีเปลี่ยนไป กรุณาตรวจสถานะบัญชีแล้วลองใหม่',
    progress_conflict: 'ประวัติเปลี่ยนจากอุปกรณ์อื่น กรุณาลองบันทึกใหม่',
    video_not_found: 'วิดีโอนี้ไม่พร้อมใช้งานหรือถูกนำออกแล้ว',
  }
  return messages[code] ?? 'เชื่อมต่อระบบไม่ได้ กรุณาลองใหม่'
}

export async function updateAccount(
  expectedUserId: number,
  name: string,
  email: string,
  currentPassword: string,
  signal?: AbortSignal,
) {
  const u = readUser(
    await jsonRequest('/auth/profile', {
      body: { expectedUserId, name, email, currentPassword },
      signal,
    }),
  )
  if (u.id !== expectedUserId) throw new ApiError('account_changed')
  return u
}
export async function changeAccountPassword(
  expectedUserId: number,
  currentPassword: string,
  newPassword: string,
  signal?: AbortSignal,
) {
  await jsonRequest('/auth/password', {
    body: { expectedUserId, currentPassword, newPassword },
    signal,
  })
}
export async function accountLibrary(
  expectedUserId: number,
  signal?: AbortSignal,
  cursors: { savedCursor?: string; historyCursor?: string } = {},
): Promise<AccountLibrary> {
  const params = new URLSearchParams(cursors)
  const d = owner(await jsonRequest('/auth/library?' + params, { signal }), expectedUserId)
  if (
    !Array.isArray(d.saved) ||
    !d.saved.every(positiveId) ||
    !Array.isArray(d.history) ||
    typeof d.savedNext !== 'string' ||
    typeof d.historyNext !== 'string' ||
    d.saved.length > 50 ||
    d.history.length > 50 ||
    !Array.isArray(d.videos) ||
    d.videos.length > 100
  )
    throw new ApiError('service_unavailable')
  return {
    userId: expectedUserId,
    saved: d.saved,
    history: d.history.map(readHistory),
    savedNext: d.savedNext,
    historyNext: d.historyNext,
    videos: (d.videos as unknown[]).map(readVideo),
  }
}
export async function accountSavedVideo(expectedUserId: number, id: number, signal?: AbortSignal) {
  const d = owner(await jsonRequest(`/auth/watchlist/${id}`, { signal }), expectedUserId)
  if (typeof d.saved !== 'boolean') throw new ApiError('service_unavailable')
  return d.saved
}
export async function saveAccountVideo(
  expectedUserId: number,
  id: number,
  saved: boolean,
  signal?: AbortSignal,
) {
  await jsonRequest(`/auth/watchlist/${id}`, { body: { expectedUserId, saved }, signal })
}
export async function accountProgress(expectedUserId: number, id: number, signal?: AbortSignal) {
  return readHistory(
    owner(await jsonRequest(`/auth/history/${id}`, { signal }), expectedUserId).history,
  )
}
export async function saveAccountProgress(
  expectedUserId: number,
  id: number,
  position: number,
  expectedRevision: number,
  keepalive = false,
) {
  return readHistory(
    owner(
      await jsonRequest(`/auth/history/${id}`, {
        body: { expectedUserId, position, expectedRevision },
        keepalive,
      }),
      expectedUserId,
    ).history,
  )
}
