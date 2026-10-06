import { useCallback, useEffect, useState } from 'react'
import type { PublicationStatus, Video } from '../types'
import { sampleVideos } from '../data/videos'
import { apiVideosEnabled, listVideos, publishVideo, removeVideo, retryVideo } from '../lib/videos'

export function useVideoCatalog(admin: boolean, path: string, accountEpoch: number) {
  const [videos, setVideos] = useState<Video[]>(apiVideosEnabled ? [] : sampleVideos)
  const [loading, setLoading] = useState(apiVideosEnabled)
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  const retry = () => setRevision((v) => v + 1)
  const processing = videos.some(
    (v) => v.processingStatus === 'pending' || v.processingStatus === 'processing',
  )
  const shouldPoll = admin && (path === '/admin/videos' || path === '/admin/upload') && processing
  useEffect(() => {
    if (!apiVideosEnabled) return
    const controller = new AbortController()
    let inFlight = false
    const refresh = async () => {
      if (inFlight || controller.signal.aborted) return
      inFlight = true
      try {
        const result = await listVideos(admin, controller.signal)
        if (!controller.signal.aborted) {
          setVideos(result)
          setError('')
        }
      } catch {
        if (!controller.signal.aborted) setError('โหลดวิดีโอไม่สำเร็จ กรุณาลองใหม่')
      } finally {
        inFlight = false
        if (!controller.signal.aborted) setLoading(false)
      }
    }
    void refresh()
    const timer = shouldPoll
      ? window.setInterval(() => {
          if (document.visibilityState === 'visible') void refresh()
        }, 3000)
      : undefined
    return () => {
      controller.abort()
      window.clearInterval(timer)
    }
  }, [admin, accountEpoch, revision, shouldPoll])
  const uploaded = useCallback((video: Video) => {
    setVideos((items) => [video, ...items.filter((v) => v.id !== video.id)])
    setRevision((v) => v + 1)
  }, [])
  const create = (video: Omit<Video, 'id'>) => {
    const id = Date.now()
    setVideos((items) => [...items, { ...video, id }])
    return id
  }
  const publish = async (id: number, status: PublicationStatus) => {
    if (!admin) return
    if (apiVideosEnabled) uploaded(await publishVideo(id, status))
    else
      setVideos((items) =>
        items.map((v) => (v.id === id ? { ...v, publicationStatus: status } : v)),
      )
  }
  const remove = async (id: number) => {
    if (!admin) return
    if (apiVideosEnabled) await removeVideo(id)
    setVideos((items) => items.filter((v) => v.id !== id))
  }
  const retryProcessing = async (id: number) => {
    await retryVideo(id)
    retry()
  }
  return { videos, loading, error, retry, uploaded, create, publish, remove, retryProcessing }
}
