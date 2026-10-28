import { useState } from 'react'
import { Icon } from './UI'

export function PasswordField({ id, label, value, onChange, error, hint, autoComplete, disabled = false }: {
  id: string; label: string; value: string; onChange: (value: string) => void;
  error?: string; hint?: string; autoComplete: 'current-password' | 'new-password'; disabled?: boolean;
}) {
  const [visible, setVisible] = useState(false)
  return <div className="rp-field"><label htmlFor={id}>{label}</label><div className="auth-password">
    <input id={id} name={id} type={visible ? 'text' : 'password'} value={value} onChange={event => onChange(event.target.value)} autoComplete={autoComplete} required disabled={disabled} aria-invalid={Boolean(error)} aria-describedby={error ? `${id}-error` : hint ? `${id}-hint` : undefined} />
    <button type="button" className="auth-reveal" onClick={() => setVisible(!visible)} aria-label={`${visible ? 'ซ่อน' : 'แสดง'}${label}`} aria-pressed={visible} title={visible ? 'ซ่อนรหัสผ่าน' : 'แสดงรหัสผ่าน'} disabled={disabled}><Icon name={visible ? 'eye-off' : 'eye'} /></button>
  </div>{hint && <small id={`${id}-hint`}>{hint}</small>}{error && <span className="auth-error" id={`${id}-error`}>{error}</span>}</div>
}
