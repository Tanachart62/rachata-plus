import type { MembershipPlan } from '../components/PlanCard'

// The API enforces these limits for full-video playback.
export const membershipPlans: MembershipPlan[] = [
  {
    id: 'free', name: 'Free', subtitle: 'เริ่มต้นสำรวจเรื่องราว',
    price: 'ฟรี', priceNote: 'ไม่มีค่าใช้จ่าย', free: true,
    features: [
      { label: 'รายละเอียดและตัวอย่าง', value: 'ดูได้ทุกเรื่อง' },
      { label: 'วิดีโอเต็ม', value: 'ต้องอัปเกรดแพ็กเกจ' },
      { label: 'คุณภาพวิดีโอเต็ม', value: '—' },
      { label: 'ดูภายหลังและโปรไฟล์', value: 'ใช้งานได้' },
    ],
  },
  {
    id: 'basic', name: 'Basic', subtitle: 'รับชมตามคุณภาพต้นฉบับ',
    price: '219 THB', priceNote: 'ไม่มีการเรียกเก็บเงินในเดโม่นี้', free: false,
    features: [
      { label: 'คุณภาพวิดีโอเต็ม', value: '1080p · Full HD' },
      { label: 'การรับชมพร้อมกัน', value: '2 จอพร้อมกัน' },
    ],
  },
  {
    id: 'standard', name: 'Standard', subtitle: 'รับชมตามคุณภาพต้นฉบับ',
    price: '419 THB', priceNote: 'ไม่มีการเรียกเก็บเงินในเดโม่นี้', free: false,
    features: [
      { label: 'คุณภาพวิดีโอเต็ม', value: '1440p · 2K' },
      { label: 'การรับชมพร้อมกัน', value: '2 จอพร้อมกัน' },
    ],
  },
  {
    id: 'premium', name: 'Premium', subtitle: 'รับชมตามคุณภาพต้นฉบับ',
    price: '789 THB', priceNote: 'ไม่มีการเรียกเก็บเงินในเดโม่นี้', free: false,
    features: [
      { label: 'คุณภาพวิดีโอเต็ม', value: '4K · Ultra HD' },
      { label: 'การรับชมพร้อมกัน', value: '4 จอพร้อมกัน' },
    ],
  },
]
