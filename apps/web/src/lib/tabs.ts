import type { KeyboardEvent } from 'react'

export function moveTab<T extends string>(event: KeyboardEvent<HTMLButtonElement>, values: readonly T[], current: T, select: (value: T) => void, prefix: string) {
  const index = values.indexOf(current)
  const next = event.key === 'Home' ? 0 : event.key === 'End' ? values.length - 1
    : ['ArrowRight', 'ArrowDown'].includes(event.key) ? (index + 1) % values.length
    : ['ArrowLeft', 'ArrowUp'].includes(event.key) ? (index + values.length - 1) % values.length : -1
  if (next < 0) return
  event.preventDefault()
  select(values[next])
  document.getElementById(`${prefix}${values[next]}`)?.focus()
}
