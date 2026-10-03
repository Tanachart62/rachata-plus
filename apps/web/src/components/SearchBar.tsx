import { useRef, useState } from 'react'
import { Icon } from './UI'

export function getSearchQuery(path: string) {
  if (path.split('?')[0] !== '/search') return ''
  const params = new URLSearchParams(path.slice(path.indexOf('?') + 1))
  return (params.get('q') ?? '').slice(0, 120)
}

export function SearchBar({ path }: { path: string }) {
  const [query, setQuery] = useState(() => getSearchQuery(path))
  const input = useRef<HTMLInputElement>(null)

  return (
    <form
      className="rp-nav-search"
      role="search"
      aria-label="ค้นหาวิดีโอ"
      onSubmit={event => {
        event.preventDefault()
        const term = query.trim().replace(/\s+/g, ' ')
        setQuery(term)
        window.location.hash = term ? `/search?${new URLSearchParams({ q: term })}` : '/search'
      }}
    >
      <label className="rp-sr" htmlFor="video-search">ค้นหาชื่อวิดีโอหรือหมวดหมู่</label>
      <input
        ref={input}
        id="video-search"
        type="search"
        name="q"
        placeholder="ค้นหาวิดีโอ หรือหมวดหมู่"
        value={query}
        onChange={event => setQuery(event.target.value)}
        maxLength={120}
        autoComplete="off"
        enterKeyHint="search"
      />
      {query && (
        <button type="button" className="search-clear" aria-label="ล้างคำค้นหา" onClick={() => { setQuery(''); input.current?.focus(); if (path.split('?')[0] === '/search') window.location.hash = '/search' }}>
          <Icon name="close" />
        </button>
      )}
      <button type="submit" className="search-submit" aria-label="ค้นหา"><Icon name="search" /></button>
    </form>
  )
}
