import type { Video } from '../types'

// Replace this sample array with data from the Go API later.
export const sampleVideos: Video[] = Array.from({ length: 8 }, (_, index) => ({
  id: index + 1,
  title: `ชื่อวิดีโอ ${String(index + 1).padStart(2, '0')}`,
  description: 'พื้นที่สำหรับคำอธิบายวิดีโอสั้น ๆ บอกเล่าเรื่องราวก่อนเริ่มรับชม',
  category: ['บันเทิง', 'ความรู้', 'ไลฟ์สไตล์', 'อื่น ๆ'][index % 4],
  durationSeconds: 750,
}))
