import { useCallback, useEffect, useRef, useState } from 'react'
import type { RefObject } from 'react'
import type { ApiUser, AccountLibrary } from '../lib/auth'
import {
  accountError,
  accountLibrary,
  accountProgress,
  accountSavedVideo,
  apiAuthEnabled,
  AuthError,
  saveAccountProgress,
  saveAccountVideo,
} from '../lib/auth'
import type { Video, WatchHistoryEntry } from '../types'
import { ProgressQueue } from '../lib/progress-queue'

export function useAccountLibrary(
  user: ApiUser | null,
  epoch: number,
  epochRef: RefObject<number>,
  path: string,
) {
  const owner = user?.id
  const ownerKey = `${epoch}:${owner ?? 'guest'}`
  const [stateOwner, setStateOwner] = useState('')
  const [videos, setVideos] = useState<Video[]>([])
  const [saved, setSaved] = useState(new Set<number>())
  const [history, setHistory] = useState<WatchHistoryEntry[]>([])
  const [status, setStatus] = useState<'loading' | 'ready' | 'error'>('loading')
  const [error, setError] = useState('')
  const [cursors, setCursors] = useState({ savedNext: '', historyNext: '' })
  const [refresh, setRefresh] = useState(0)
  const [paging, setPaging] = useState(false)
  const [watchLoaded, setWatchLoaded] = useState('')
  const mutation = useRef(0)
  const writes = useRef(0)
  const dirtySnapshot = useRef(false)
  const requests = useRef(new Set<number>())
  const queue = useRef<ProgressQueue | null>(null)
  const controller = useRef<AbortController | null>(null)
  const current = useCallback(() => !!owner && epochRef.current === epoch, [owner, epoch, epochRef])
  const finish = useCallback(() => {
    writes.current--
    mutation.current++
    if (!writes.current && dirtySnapshot.current) {
      dirtySnapshot.current = false
      setRefresh((v) => v + 1)
    }
  }, [])
  useEffect(() => {
    const reset = new AbortController()
    queueMicrotask(() => {
      if (reset.signal.aborted || epochRef.current !== epoch) return
      setStateOwner(ownerKey)
      setVideos([])
      setSaved(new Set())
      setHistory([])
      setStatus('loading')
      setError('')
      setCursors({ savedNext: '', historyNext: '' })
      setWatchLoaded('')
      setPaging(false)
    })
    writes.current = 0
    mutation.current++
    requests.current.clear()
    dirtySnapshot.current = false
    if (!owner || !apiAuthEnabled) return () => reset.abort()
    const q = new ProgressQueue(
      {
        read: (id) => accountProgress(owner, id),
        save: (id, position, revision, flush) =>
          saveAccountProgress(owner, id, position, revision, flush),
      },
      current,
      (entry, pending) => {
        mutation.current++
        setError('')
        if (!pending)
          setHistory((items) => [entry, ...items.filter((e) => e.videoId !== entry.videoId)])
      },
    )
    queue.current = q
    return () => {
      reset.abort()
      q.stop()
      queue.current = null
      controller.current?.abort()
    }
  }, [owner, epoch, epochRef, ownerKey, current])
  useEffect(() => {
    if (!owner || !apiAuthEnabled) return
    const abort = new AbortController()
    controller.current = abort
    const version = mutation.current
    accountLibrary(owner, abort.signal)
      .then((data) => {
        if (abort.signal.aborted || !current()) return
        if (version !== mutation.current || writes.current) {
          dirtySnapshot.current = true
          if (!writes.current) setRefresh((v) => v + 1)
          return
        }
        setVideos(data.videos)
        setSaved(new Set(data.saved))
        setHistory(data.history)
        setCursors(data)
        setStatus('ready')
        setError('')
      })
      .catch((e) => {
        if (!abort.signal.aborted && current()) {
          setStatus('error')
          setError(accountError(e))
        }
      })
    return () => abort.abort()
  }, [owner, epoch, refresh, current])
  const watchId = path.match(/^\/watch\/(\d+)$/)?.[1]
  const watchKey = `${epoch}:${watchId ?? ''}`
  useEffect(() => {
    if (!owner || !watchId || !apiAuthEnabled) return
    const abort = new AbortController()
    accountProgress(owner, Number(watchId), abort.signal)
      .then((entry) => {
        if (!abort.signal.aborted && current()) {
          if (entry.watchedAt > 0)
            setHistory((items) =>
              items.some((e) => e.videoId === entry.videoId && e.watchedAt > entry.watchedAt)
                ? items
                : [entry, ...items.filter((e) => e.videoId !== entry.videoId)],
            )
          setWatchLoaded(watchKey)
        }
      })
      .catch((e) => {
        if (!abort.signal.aborted && current()) {
          setError(accountError(e))
          setWatchLoaded(watchKey)
        }
      })
    return () => abort.abort()
  }, [owner, watchId, watchKey, refresh, current])
  const retry = () => {
    queue.current?.retry()
    setRefresh((v) => v + 1)
  }
  const toggle = async (id: number) => {
    if (!owner || !current()) {
      window.location.hash = '/login'
      return
    }
    if (requests.current.has(id)) return
    requests.current.add(id)
    mutation.current++
    writes.current++
    try {
      const target = !(await accountSavedVideo(owner, id))
      if (!current()) return
      await saveAccountVideo(owner, id, target)
      if (!current()) return
      setSaved((items) => {
        const next = new Set(items)
        if (target) next.add(id)
        else next.delete(id)
        return next
      })
      setError('')
    } catch (e) {
      if (current()) setError(accountError(e))
    } finally {
      if (current()) {
        requests.current.delete(id)
        finish()
      }
    }
  }
  const progress = useCallback(
    async (id: number, position: number, flush = false) => {
      if (!owner || !current() || !queue.current) throw new AuthError('account_changed')
      mutation.current++
      writes.current++
      setHistory((items) => [
        { videoId: id, position, watchedAt: Date.now() },
        ...items.filter((e) => e.videoId !== id),
      ])
      try {
        await queue.current.enqueue(id, position, flush)
        if (current()) setError('')
      } catch (e) {
        if (current()) setError('บันทึกประวัติไม่สำเร็จ: ' + accountError(e))
        throw e
      } finally {
        if (current()) finish()
      }
    },
    [owner, current, finish],
  )
  const loadMore = async (kind: 'saved' | 'history') => {
    if (!owner || paging || !current()) return
    const cursor = kind === 'saved' ? cursors.savedNext : cursors.historyNext
    if (!cursor) return
    setPaging(true)
    const version = mutation.current
    try {
      const data: AccountLibrary = await accountLibrary(
        owner,
        undefined,
        kind === 'saved' ? { savedCursor: cursor } : { historyCursor: cursor },
      )
      if (!current()) return
      if (version !== mutation.current || writes.current) {
        dirtySnapshot.current = true
        if (!writes.current) setRefresh((v) => v + 1)
        return
      }
      setVideos((items) => {
        const merged = new Map(items.map((video) => [video.id, video]))
        for (const video of data.videos) merged.set(video.id, video)
        return [...merged.values()]
      })
      if (kind === 'saved') setSaved((items) => new Set([...items, ...data.saved]))
      else
        setHistory((items) => [
          ...items,
          ...data.history.filter((e) => !items.some((old) => old.videoId === e.videoId)),
        ])
      setCursors((old) => ({
        ...old,
        [kind === 'saved' ? 'savedNext' : 'historyNext']:
          kind === 'saved' ? data.savedNext : data.historyNext,
      }))
    } catch (e) {
      if (current()) setError(accountError(e))
    } finally {
      if (current()) setPaging(false)
    }
  }
  return {
    videos: stateOwner === ownerKey ? videos : [],
    saved: stateOwner === ownerKey ? saved : new Set<number>(),
    history: stateOwner === ownerKey ? history : [],
    loading:
      !!owner &&
      (stateOwner !== ownerKey || status === 'loading' || (!!watchId && watchLoaded !== watchKey)),
    status,
    error,
    retry,
    toggle,
    progress,
    cursors,
    paging,
    loadMore,
  }
}
