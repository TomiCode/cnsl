// Flattens binding data into template paths for editor completion: .Data.ha.sensor_x.state …
// Arrays contribute the fields of their first element (as you'd use them inside `range`).
export interface DataField {
  label: string
  detail: string
}

export function dataPaths(data: Record<string, { data?: unknown }>, maxDepth = 5): DataField[] {
  const out: DataField[] = []
  const walk = (value: unknown, path: string, depth: number) => {
    out.push({ label: path, detail: describe(value) })
    if (depth >= maxDepth || value == null) return
    if (Array.isArray(value)) {
      if (value.length && typeof value[0] === 'object') walk(value[0], `${path}[0]`, depth + 1)
      return
    }
    if (typeof value === 'object') {
      for (const [k, v] of Object.entries(value as Record<string, unknown>)) {
        if (/^[A-Za-z_][A-Za-z0-9_]*$/.test(k)) walk(v, `${path}.${k}`, depth + 1)
      }
    }
  }
  for (const [alias, entry] of Object.entries(data)) walk(entry.data, `.Data.${alias}`, 0)
  // "[0]" isn't template syntax; inside range the element is ".", so offer the bare field names too.
  return out
    .map((f) => ({ ...f, label: f.label.replace(/\[0\]/g, '') }))
    .filter((f, i, all) => all.findIndex((g) => g.label === f.label) === i)
}

function describe(v: unknown): string {
  if (v == null) return 'null'
  if (Array.isArray(v)) return `list (${v.length})`
  if (typeof v === 'object') return 'object'
  const s = JSON.stringify(v)
  return s.length > 40 ? `${s.slice(0, 40)}…` : s
}
