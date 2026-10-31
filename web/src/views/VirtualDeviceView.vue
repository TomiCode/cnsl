<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { AlarmClock, MessageSquareText, Power, RotateCcw } from 'lucide-vue-next'
import { duration } from '@/lib/format'

// A browser-based TRMNL that speaks the real firmware protocol against /device/api/* (the device
// listener, mirrored on the dashboard origin). Its identity is kept in localStorage.

const WIDTH = 800
const HEIGHT = 480
const STORAGE_KEY = 'cnsl.virtual-device'

interface Identity {
  mac: string
  api_key: string
  friendly_id: string
}

type State =
  | { kind: 'off' }
  | { kind: 'pending' }
  | { kind: 'showing'; filename: string; refresh: number }
  | { kind: 'error'; message: string }

const identity = ref<Identity | null>(load())
const state = ref<State>({ kind: 'off' })
const nextWakeAt = ref<number | null>(null)
const now = ref(Date.now())
const activity = ref<string[]>([])
let timer: ReturnType<typeof setTimeout> | undefined
let clock: ReturnType<typeof setInterval> | undefined

function load(): Identity | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    return raw ? (JSON.parse(raw) as Identity) : null
  } catch {
    return null
  }
}

function note(text: string) {
  activity.value.unshift(`${new Date().toLocaleTimeString()}  ${text}`)
  activity.value.length = Math.min(activity.value.length, 50)
}

function randomMac() {
  const bytes = crypto.getRandomValues(new Uint8Array(6))
  bytes[0] = (bytes[0]! & 0xfc) | 0x02 // locally administered, unicast
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0').toUpperCase()).join(':')
}

const baseHeaders = (id: Identity) => ({
  ID: id.mac,
  Model: 'virtual',
  'FW-Version': 'virtual-1.0',
  Width: String(WIDTH),
  Height: String(HEIGHT),
})

async function powerOn() {
  const mac = identity.value?.mac ?? randomMac()
  try {
    const res = await fetch('/device/api/setup', { headers: { ...baseHeaders({ mac } as Identity) } })
    const body = await res.json()
    if (body.status !== 200) throw new Error(body.message ?? `setup status ${body.status}`)
    identity.value = { mac, api_key: body.api_key, friendly_id: body.friendly_id }
    localStorage.setItem(STORAGE_KEY, JSON.stringify(identity.value))
    note(`setup ok — pairing code ${body.friendly_id}`)
    await wake('powercycle')
  } catch (e) {
    state.value = { kind: 'error', message: (e as Error).message }
    note(`setup failed: ${(e as Error).message}`)
  }
}

async function wake(reason: 'timer' | 'button' | 'powercycle') {
  const id = identity.value
  if (!id) return
  clearTimeout(timer)
  try {
    const res = await fetch('/device/api/display', {
      headers: {
        ...baseHeaders(id),
        'Access-Token': id.api_key,
        'Update-Source': reason,
        'Refresh-Rate': String(state.value.kind === 'showing' ? state.value.refresh : 0),
        'Battery-Voltage': (3.9 + Math.random() * 0.2).toFixed(2),
        'Percent-Charged': String(80 + Math.floor(Math.random() * 15)),
        RSSI: String(-50 - Math.floor(Math.random() * 20)),
        'USB-Connected': 'false',
        'Wake-Time': String(1500 + Math.floor(Math.random() * 1500)),
        'Image-Cached': 'false',
      },
    })
    const body = await res.json()
    if (body.status === 202) {
      state.value = { kind: 'pending' }
      note('display: 202 not registered — waiting for approval')
      schedule(10)
    } else if (body.status === 0) {
      const changed = state.value.kind !== 'showing' || state.value.filename !== body.filename
      state.value = { kind: 'showing', filename: body.filename, refresh: body.refresh_rate }
      note(`display: ${changed ? 'new image' : 'unchanged'} ${body.filename.slice(0, 8)}…, sleep ${duration(body.refresh_rate)}`)
      schedule(body.refresh_rate)
    } else {
      throw new Error(`unexpected status ${body.status}`)
    }
  } catch (e) {
    state.value = { kind: 'error', message: (e as Error).message }
    note(`display failed: ${(e as Error).message}`)
    schedule(30)
  }
}

function schedule(seconds: number) {
  clearTimeout(timer)
  nextWakeAt.value = Date.now() + seconds * 1000
  timer = setTimeout(() => wake('timer'), seconds * 1000)
}

async function sendLog() {
  const id = identity.value
  if (!id) return
  const entry = {
    id: Math.floor(Math.random() * 1e6),
    message: 'Test log from the virtual device',
    level: 'info',
    created_at: Math.floor(Date.now() / 1000),
    source_path: 'virtual/device.ts',
    source_line: 1,
    wifi_signal: -55,
    wake_reason: 'button',
  }
  const res = await fetch('/device/api/log', {
    method: 'POST',
    headers: { 'Access-Token': id.api_key, ID: id.mac, 'Content-Type': 'application/json' },
    body: JSON.stringify({ logs: [entry] }),
  })
  note(res.ok ? 'log sent' : `log rejected: ${res.status}`)
}

function forget() {
  if (!confirm('Forget this virtual device? It stays in the device list until you delete it there.')) return
  clearTimeout(timer)
  localStorage.removeItem(STORAGE_KEY)
  identity.value = null
  state.value = { kind: 'off' }
  nextWakeAt.value = null
  note('identity forgotten')
}

const countdown = computed(() => {
  if (nextWakeAt.value == null) return null
  return Math.max(0, Math.round((nextWakeAt.value - now.value) / 1000))
})

onMounted(() => {
  clock = setInterval(() => (now.value = Date.now()), 1000)
  if (identity.value) void wake('powercycle')
})
onBeforeUnmount(() => {
  clearTimeout(timer)
  clearInterval(clock)
})
</script>

<template>
  <div class="mx-auto max-w-5xl px-8 py-8">
    <h1 class="mb-1 text-xl font-semibold">Virtual device</h1>
    <p class="mb-6 text-sm text-zinc-500">
      An {{ WIDTH }}×{{ HEIGHT }} test device in your browser. It uses the same setup → display → log protocol as real firmware, so
      it appears under <RouterLink to="/devices" class="underline">Devices</RouterLink> and needs approval. It only polls while this page is open.
    </p>

    <div class="mb-4 flex flex-wrap gap-2">
      <button
        v-if="!identity"
        type="button"
        class="inline-flex items-center gap-1.5 rounded-md bg-zinc-900 px-3 py-1.5 text-sm text-white hover:bg-zinc-700"
        @click="powerOn"
      >
        <Power class="size-4" />
        Power on
      </button>
      <template v-else>
        <button
          type="button"
          class="inline-flex items-center gap-1.5 rounded-md bg-zinc-900 px-3 py-1.5 text-sm text-white hover:bg-zinc-700"
          @click="wake('button')"
        >
          <AlarmClock class="size-4" />
          Wake now
        </button>
        <button
          type="button"
          class="inline-flex items-center gap-1.5 rounded-md border border-zinc-200 bg-white px-3 py-1.5 text-sm hover:bg-zinc-50"
          @click="sendLog"
        >
          <MessageSquareText class="size-4" />
          Send test log
        </button>
        <button
          type="button"
          class="inline-flex items-center gap-1.5 rounded-md border border-zinc-200 bg-white px-3 py-1.5 text-sm text-zinc-600 hover:bg-zinc-50"
          @click="forget"
        >
          <RotateCcw class="size-4" />
          Forget
        </button>
      </template>
    </div>

    <div class="grid gap-6 lg:grid-cols-3">
      <div class="lg:col-span-2">
        <!-- The "bezel" -->
        <div class="rounded-2xl bg-zinc-800 p-4 shadow-lg">
          <div class="relative overflow-hidden rounded-sm bg-[#e8e6df]" :style="{ aspectRatio: `${WIDTH} / ${HEIGHT}` }">
            <img
              v-if="state.kind === 'showing'"
              :src="`/device/images/${state.filename}`"
              alt="Virtual device screen"
              class="absolute inset-0 size-full mix-blend-multiply [image-rendering:pixelated]"
            />
            <div v-else class="absolute inset-0 flex flex-col items-center justify-center gap-2 text-center text-zinc-700">
              <template v-if="state.kind === 'off'">
                <span class="text-sm">Powered off</span>
              </template>
              <template v-else-if="state.kind === 'pending'">
                <span class="font-mono text-4xl font-bold tracking-widest">{{ identity?.friendly_id }}</span>
                <span class="text-sm">Approve this device in cnsl</span>
              </template>
              <template v-else-if="state.kind === 'error'">
                <span class="text-sm">Error: {{ state.message }}</span>
              </template>
            </div>
          </div>
        </div>
        <div class="mt-2 flex justify-between text-xs text-zinc-500">
          <span v-if="identity" class="font-mono">{{ identity.mac }}</span>
          <span v-if="countdown != null">next wake in {{ duration(countdown) }}</span>
        </div>
      </div>

      <div class="rounded-lg border border-zinc-200 bg-white p-4">
        <div class="mb-2 text-sm font-medium">Activity</div>
        <div v-if="!activity.length" class="text-xs text-zinc-400">Nothing yet.</div>
        <pre class="max-h-96 overflow-y-auto text-xs leading-relaxed whitespace-pre-wrap text-zinc-600">{{ activity.join('\n') }}</pre>
      </div>
    </div>
  </div>
</template>
