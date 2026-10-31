import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { Schemas } from '@/api/client'

export type ServerEvent = Schemas['Event']
type Listener = (e: ServerEvent) => void

const eventTypes: ServerEvent['type'][] = [
  'device.created',
  'device.updated',
  'device.deleted',
  'device.seen',
  'device.logs',
]

// One shared SSE connection to /api/v1/events; stores and views subscribe to it.
export const useEventsStore = defineStore('events', () => {
  const connected = ref(false)
  const listeners = new Set<Listener>()
  let source: EventSource | null = null

  function connect() {
    if (source) return
    source = new EventSource('/api/v1/events')
    source.onopen = () => {
      // After a reconnect we may have missed events; let everyone refetch.
      if (!connected.value) listeners.forEach((l) => l({ type: 'device.updated' }))
      connected.value = true
    }
    source.onerror = () => {
      connected.value = false
    }
    for (const type of eventTypes) {
      source.addEventListener(type, (msg) => {
        const event = JSON.parse((msg as MessageEvent).data) as ServerEvent
        listeners.forEach((l) => l(event))
      })
    }
  }

  function subscribe(listener: Listener): () => void {
    connect()
    listeners.add(listener)
    return () => listeners.delete(listener)
  }

  return { connected, connect, subscribe }
})
