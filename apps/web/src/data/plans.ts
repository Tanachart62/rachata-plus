import type { MembershipPlan } from '../components/PlanCard'

// Display prices and streaming examples; no real billing or API-enforced limits.
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
    id: 'basic', name: 'Basic', subtitle: 'สำหรับการรับชมคนเดียว',
    price: '219 THB', priceNote: 'ไม่มีการเรียกเก็บเงินในเดโม่นี้', free: false,
    features: [
      { label: 'คุณภาพวิดีโอตัวอย่าง', value: '720p · HD' },
      { label: 'รับชมพร้อมกัน (ตัวอย่าง)', value: '1 อุปกรณ์' },
    ],
  },
  {
    id: 'standard', name: 'Standard', subtitle: 'คมชัดขึ้นกับเรื่องที่ชอบ',
    price: '419 THB', priceNote: 'ไม่มีการเรียกเก็บเงินในเดโม่นี้', free: false,
    features: [
      { label: 'คุณภาพวิดีโอตัวอย่าง', value: '1080p · Full HD' },
      { label: 'รับชมพร้อมกัน (ตัวอย่าง)', value: '2 อุปกรณ์' },
    ],
  },
  {
    id: 'premium', name: 'Premium', subtitle: 'ครบทุกประสบการณ์รับชม',
    price: '789 THB', priceNote: 'ไม่มีการเรียกเก็บเงินในเดโม่นี้', free: false,
    features: [
      { label: 'คุณภาพวิดีโอตัวอย่าง', value: '4K · Ultra HD' },
      { label: 'รับชมพร้อมกัน (ตัวอย่าง)', value: '4 อุปกรณ์' },
    ],
  },
]
