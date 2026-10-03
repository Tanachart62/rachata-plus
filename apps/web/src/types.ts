export type Viewer = 'guest' | 'user' | 'member' | 'admin'

export interface Video {
  id: number
  title: string
  description: string
  category: string
  durationSeconds: number
}

export interface Profile {
  name: string
  email: string
  emailPending: boolean
}
