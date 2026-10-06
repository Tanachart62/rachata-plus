import { ApiError as AuthError } from './http'
import type { WatchHistoryEntry } from '../types'
type Waiter = { resolve: () => void; reject: (error: unknown) => void }
type Pending = { position: number; flush: boolean; waiters: Waiter[] }
type Slot = { revision?: number; pending?: Pending; inFlight: boolean }
export interface ProgressTransport {
  read: (id: number) => Promise<WatchHistoryEntry>
  save: (
    id: number,
    position: number,
    revision: number,
    flush: boolean,
  ) => Promise<WatchHistoryEntry>
}
// One in-flight request per video. Coalesce queued seeks; never retry an old
// conflicting position when a newer position is already waiting.
export class ProgressQueue {
  private slots = new Map<number, Slot>()
  private stopped = false
  private transport: ProgressTransport
  private current: () => boolean
  private acknowledged: (entry: WatchHistoryEntry, pending: boolean) => void
  constructor(
    transport: ProgressTransport,
    current: () => boolean,
    acknowledged: (entry: WatchHistoryEntry, pending: boolean) => void,
  ) {
    this.transport = transport
    this.current = current
    this.acknowledged = acknowledged
  }
  private pending(slot: Slot): Pending | undefined {
    return slot.pending
  }
  enqueue(id: number, position: number, flush = false): Promise<void> {
    if (!this.valid()) return Promise.reject(new AuthError('account_changed'))
    let slot = this.slots.get(id)
    if (!slot) {
      if (this.slots.size >= 100) {
        const idle = [...this.slots].find(([, old]) => !old.inFlight && !old.pending)
        if (idle) this.slots.delete(idle[0])
        else return Promise.reject(new AuthError('too_many_requests'))
      }
      slot = { inFlight: false }
      this.slots.set(id, slot)
    }
    const target = slot
    const result = new Promise<void>((resolve, reject) => {
      target.pending = {
        position,
        flush: flush || (target.pending?.flush ?? false),
        waiters: [...(target.pending?.waiters ?? []), { resolve, reject }],
      }
    })
    void this.drain(id, target)
    return result
  }
  retry() {
    for (const [id, slot] of this.slots) void this.drain(id, slot)
  }
  stop() {
    this.stopped = true
    for (const slot of this.slots.values()) {
      slot.pending?.waiters.forEach((w) => w.reject(new AuthError('account_changed')))
      slot.pending = undefined
    }
    this.slots.clear()
  }
  private valid() {
    return !this.stopped && this.current()
  }
  private async drain(id: number, slot: Slot) {
    if (slot.inFlight || !slot.pending || !this.valid()) return
    slot.inFlight = true
    let failed = false
    let item = slot.pending
    slot.pending = undefined
    try {
      if (slot.revision === undefined) slot.revision = (await this.transport.read(id)).revision ?? 0
      let conflicts = 0
      for (;;) {
        if (!this.valid()) throw new AuthError('account_changed')
        try {
          const entry = await this.transport.save(id, item.position, slot.revision, item.flush)
          if (!this.valid()) throw new AuthError('account_changed')
          slot.revision = entry.revision
          this.acknowledged(entry, !!slot.pending)
          item.waiters.forEach((w) => w.resolve())
          break
        } catch (error) {
          if (
            !(error instanceof AuthError) ||
            error.code !== 'progress_conflict' ||
            ++conflicts > 2
          )
            throw error
          slot.revision = (await this.transport.read(id)).revision ?? 0
          const newer = this.pending(slot)
          if (newer) {
            item = {
              ...newer,
              flush: item.flush || newer.flush,
              waiters: [...item.waiters, ...newer.waiters],
            }
            slot.pending = undefined
          }
        }
      }
    } catch (error) {
      failed = true
      item.waiters.forEach((w) => w.reject(error))
      // Keep the newest unsaved value for a later event/manual retry.
      if (this.valid() && !slot.pending) slot.pending = { ...item, waiters: [] }
    } finally {
      slot.inFlight = false
    }
    if (!failed && slot.pending && this.valid()) void this.drain(id, slot)
    // Retain ACK revisions for unload saves, but bound long-lived tab memory.
    if (this.slots.size > 100) {
      for (const [key, old] of this.slots) {
        if (key !== id && !old.inFlight && !old.pending) {
          this.slots.delete(key)
          break
        }
      }
    }
  }
}
