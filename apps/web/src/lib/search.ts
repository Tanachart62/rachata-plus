export function getSearchQuery(path: string) {
  const separator = path.indexOf('?')
  if (separator === -1 || path.slice(0, separator) !== '/search') return ''
  const params = new URLSearchParams(path.slice(separator + 1))
  return (params.get('q') ?? '').slice(0, 120)
}
