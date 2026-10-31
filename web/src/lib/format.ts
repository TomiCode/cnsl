const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })

export function timeAgo(iso?: string | null): string {
  if (!iso) return 'never'
  const seconds = Math.round((new Date(iso).getTime() - Date.now()) / 1000)
  const abs = Math.abs(seconds)
  if (abs < 45) return 'just now'
  if (abs < 3600) return rtf.format(Math.round(seconds / 60), 'minute')
  if (abs < 86400) return rtf.format(Math.round(seconds / 3600), 'hour')
  return rtf.format(Math.round(seconds / 86400), 'day')
}

export function dateTime(iso?: string | null): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleString()
}

export function duration(seconds: number): string {
  if (seconds < 60) return `${seconds} s`
  if (seconds < 3600) return `${Math.round(seconds / 60)} min`
  const h = Math.floor(seconds / 3600)
  const m = Math.round((seconds % 3600) / 60)
  return m ? `${h} h ${m} min` : `${h} h`
}

// Signal quality buckets for Wi-Fi RSSI (dBm).
export function signalQuality(rssi?: number | null): 'good' | 'fair' | 'poor' | 'unknown' {
  if (rssi == null) return 'unknown'
  if (rssi >= -60) return 'good'
  if (rssi >= -75) return 'fair'
  return 'poor'
}

export function imageUrl(filename?: string | null): string | null {
  return filename ? `/device/images/${filename}` : null
}
