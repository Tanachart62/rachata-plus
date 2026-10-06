import { Icon, Skeleton } from './UI'

export interface MembershipPlan {
  id: string
  name: string
  subtitle: string
  price: string
  priceNote: string
  free: boolean
  features: { label: string; value: string }[]
}

interface PlanCardProps {
  plan: MembershipPlan
  selected: boolean
  onSelect: (id: string) => void
}

export function PlanCard({ plan, selected, onSelect }: PlanCardProps) {
  return (
    <label className={`plan-card ${selected ? 'is-selected' : ''}`}>
      <input
        type="radio"
        name="membership-plan"
        value={plan.id}
        checked={selected}
        onChange={() => onSelect(plan.id)}
        aria-labelledby={`plan-name-${plan.id}`}
        aria-describedby={`plan-price-${plan.id}`}
      />
      <span className={`plan-card-banner plan-color-${plan.id}`}>
        <span id={`plan-name-${plan.id}`} className="plan-name">{plan.name}</span>
        <span className="plan-subtitle">{plan.subtitle}</span>
        <span className="plan-check" aria-hidden="true">{selected && <Icon name="check" />}</span>
      </span>
      <span className="plan-price" id={`plan-price-${plan.id}`}>
        <span className="plan-label">ค่าบริการ</span>
        <strong>{plan.price}</strong>
        <small>{plan.priceNote}</small>
      </span>
      <span className="plan-features">
        {plan.features.map(feature => (
          <span className="plan-feature" key={feature.label}>
            <span className="plan-label">{feature.label}</span>
            <span>{feature.value}</span>
          </span>
        ))}
      </span>
      <span className="plan-selection" aria-hidden="true">{selected ? 'เลือกแล้ว' : 'เลือกแพ็กเกจนี้'}</span>
    </label>
  )
}

export function PlanSkeleton() {
  return (
    <div className="plan-card plan-card-skeleton" aria-hidden="true">
      <Skeleton shape="plan-banner" />
      <Skeleton shape="meta" />
      <Skeleton shape="title" />
      {Array.from({ length: 2 }, (_, index) => (
        <div className="plan-feature" key={index}><Skeleton shape="line" /><Skeleton shape="line2" /></div>
      ))}
    </div>
  )
}
