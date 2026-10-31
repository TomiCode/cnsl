<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Radio, Search } from 'lucide-vue-next'
import { api, type Schemas } from '@/api/client'
import { unwrap, type Device } from '@/stores/devices'
import { useEventsStore } from '@/stores/events'
import { dateTime } from '@/lib/format'

type DeviceLog = Schemas['DeviceLog']

const props = defineProps<{ device: Device }>()

const levels = ['debug', 'info', 'warn', 'error', 'fatal'] as const
const levelStyle: Record<string, string> = {
  debug: 'text-zinc-400',
  info: 'text-sky-700',
  warn: 'text-amber-700',
  error: 'text-red-700',
  fatal: 'text-red-700 font-semibold',
}

const logs = ref<DeviceLog[]>([])
const nextBefore = ref<number | null>(null)
const selectedLevels = ref<string[]>([])
const query = ref('')
const live = ref(true)
const loading = ref(false)
const error = ref<string | null>(null)
const expanded = ref<number | null>(null)

function filters() {
  return {
    level: selectedLevels.value.length ? selectedLevels.value : undefined,
    q: query.value.trim() || undefined,
  }
}

async function load() {
  loading.value = true
  error.value = null
  try {
    const page = unwrap(await api.GET('/devices/{id}/logs', { params: { path: { id: props.device.id }, query: filters() } }))
    logs.value = page.items
    nextBefore.value = page.next_before_id ?? null
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    loading.value = false
  }
}

async function loadOlder() {
  if (nextBefore.value == null) return
  const page = unwrap(
    await api.GET('/devices/{id}/logs', {
      params: { path: { id: props.device.id }, query: { ...filters(), before_id: nextBefore.value } },
    }),
  )
  logs.value.push(...page.items)
  nextBefore.value = page.next_before_id ?? null
}

async function loadNewer() {
  const newest = logs.value[0]?.id
  if (newest == null) return load()
  const page = unwrap(
    await api.GET('/devices/{id}/logs', {
      params: { path: { id: props.device.id }, query: { ...filters(), after_id: newest, limit: 500 } },
    }),
  )
  logs.value.unshift(...page.items)
}

let searchTimer: ReturnType<typeof setTimeout> | undefined
watch(selectedLevels, load, { deep: true })
watch(query, () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(load, 300)
})

const unsubscribe = useEventsStore().subscribe((e) => {
  if (live.value && e.type === 'device.logs' && e.device_id === props.device.id) void loadNewer()
})
onMounted(load)
onBeforeUnmount(() => {
  unsubscribe()
  clearTimeout(searchTimer)
})

function toggleLevel(level: string) {
  const i = selectedLevels.value.indexOf(level)
  if (i >= 0) selectedLevels.value.splice(i, 1)
  else selectedLevels.value.push(level)
}
</script>

<template>
  <div>
    <div class="mb-4 flex flex-wrap items-center gap-2">
      <button
        v-for="l in levels"
        :key="l"
        type="button"
        class="rounded-full border px-2.5 py-0.5 text-xs"
        :class="selectedLevels.includes(l) ? 'border-zinc-900 bg-zinc-900 text-white' : 'border-zinc-300 bg-white text-zinc-600 hover:bg-zinc-50'"
        @click="toggleLevel(l)"
      >
        {{ l }}
      </button>
      <label class="relative ml-2 flex-1">
        <Search class="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-zinc-400" />
        <input
          v-model="query"
          placeholder="Search messages"
          class="w-full rounded-md border border-zinc-300 bg-white py-1.5 pr-3 pl-8 text-sm focus:border-zinc-500 focus:outline-none"
        />
      </label>
      <button
        type="button"
        class="inline-flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm"
        :class="live ? 'border-emerald-300 bg-emerald-50 text-emerald-700' : 'border-zinc-300 bg-white text-zinc-600'"
        :title="live ? 'New logs appear automatically' : 'Live updates paused'"
        @click="live = !live"
      >
        <Radio class="size-4" />
        Live
      </button>
    </div>

    <p v-if="error" class="mb-4 text-sm text-red-600">{{ error }}</p>

    <!-- Container queries: one line per entry when there's room, message on its own line when narrow. -->
    <div class="@container overflow-hidden rounded-lg border border-zinc-200 bg-white">
      <div v-if="!logs.length" class="px-4 py-10 text-center text-sm text-zinc-400">
        {{ loading ? 'Loading…' : 'No logs yet. The device sends them after errors or when verbose logging is on.' }}
      </div>
      <ul v-else class="divide-y divide-zinc-100 font-mono text-xs">
        <li v-for="l in logs" :key="l.id">
          <button
            type="button"
            class="flex w-full flex-wrap items-baseline gap-x-3 gap-y-0.5 px-3 py-1.5 text-left hover:bg-zinc-50 @3xl:flex-nowrap"
            @click="expanded = expanded === l.id ? null : l.id"
          >
            <span class="shrink-0 whitespace-nowrap text-zinc-400" :title="`received ${dateTime(l.received_at)}`">
              {{ dateTime(l.device_time ?? l.received_at) }}
            </span>
            <span class="w-10 shrink-0" :class="levelStyle[l.level ?? ''] ?? 'text-zinc-500'">{{ l.level ?? '—' }}</span>
            <span class="order-last basis-full break-words text-zinc-800 @3xl:order-none @3xl:min-w-0 @3xl:flex-1 @3xl:basis-auto">
              {{ l.message }}
            </span>
            <span v-if="l.source_path" class="ml-auto shrink-0 whitespace-nowrap text-zinc-400">{{ l.source_path }}:{{ l.source_line }}</span>
          </button>
          <pre v-if="expanded === l.id && l.extra" class="bg-zinc-50 px-3 py-2 whitespace-pre-wrap text-zinc-600">{{ JSON.stringify(l.extra, null, 2) }}</pre>
        </li>
      </ul>
    </div>
    <button
      v-if="nextBefore != null"
      type="button"
      class="mt-3 rounded-md border border-zinc-200 bg-white px-3 py-1.5 text-sm hover:bg-zinc-50"
      @click="loadOlder"
    >
      Load older
    </button>
  </div>
</template>
