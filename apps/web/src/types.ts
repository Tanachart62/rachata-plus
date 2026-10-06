export type Viewer = 'guest' | 'user' | 'member' | 'admin'
export type PublicationStatus = 'draft' | 'published' | 'hidden'

export interface Video {
  id: number
  title: string
  description: string
  category: string
  durationSeconds: number
  publicationStatus: PublicationStatus
  processingStatus?: 'pending' | 'processing' | 'ready' | 'failed'
  processingError?: string
  playbackUrl?: string
  previewUrl?: string
}

export interface Profile {
  username: string
  name: string
  email: string
  emailPending: boolean
}

export interface WatchHistoryEntry {
  videoId: number
  position: number
  watchedAt: number
  revision?: number
}
