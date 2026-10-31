<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ArrowDown, ArrowUp, ListVideo, Plus, Trash2, X } from 'lucide-vue-next'
import type { Schemas } from '@/api/client'
import { usePlaylistsStore, type Playlist } from '@/stores/playlists'
import { useScreensStore } from '@/stores/screens'
import { useDevicesStore } from '@/stores/devices'
import { duration, imageUrl } from '@/lib/format'

type Item = Schemas['PlaylistItem'] & { custom: boolean; rate: number }

const playlists = usePlaylistsStore()
const screens = useScreensStore()
const devices = useDevicesStore()

const editingId = ref<number | 'new' | null>(null)
const name = ref('')
const items = ref<Item[]>([])
const error = ref<string | null>(null)
const saving = ref(false)

onMounted(async () => {
  try {
    await Promise.all([playlists.load(), screens.load(), devices.loaded ? Promise.resolve() : devices.load()])
  } catch (e) {
    error.value = (e as Error).message
  }
})

function edit(p: Playlist | null) {
  error.value = null
  editingId.value = p ? p.id : 'new'
  name.value = p?.name ?? ''
  items.value = (p?.items ?? []).map((it) => ({ ...it, custom: it.refresh_rate != null, rate: it.refresh_rate ?? 900 }))
}

function addItem() {
  const first = screens.screens[0]
  if (!first) return
  items.value.push({ screen_id: first.id, dwell: 1, enabled: true, custom: false, rate: 900 })
}

function move(i: number, delta: number) {
  const j = i + delta
  if (j < 0 || j >= items.value.length) return
  const list = items.value
  ;[list[i], list[j]] = [list[j]!, list[i]!]
}

async function save() {
  saving.value = true
  error.value = null
  const body = {
    name: name.value,
    items: items.value.map((it) => ({
      id: it.id,
      screen_id: it.screen_id,
      dwell: it.dwell,
      enabled: it.enabled,
      refresh_rate: it.custom ? it.rate : null,
    })),
  }
  try {
    if (editingId.value === 'new') await playlists.create(body)
    else if (editingId.value != null) await playlists.update(editingId.value, body)
    editingId.value = null
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    saving.value = false
  }
}

async function remove(p: Playlist) {
  if (!confirm(`Delete playlist "${p.name}"? Devices using it fall back to their static image.`)) return
  try {
    await playlists.remove(p.id)
  } catch (e) {
    error.value = (e as Error).message
  }
}

const screenById = computed(() => new Map(screens.screens.map((s) => [s.id, s])))
function screenName(id: number) {
  return screenById.value.get(id)?.name ?? `#${id}`
}
function modelName(screenId: number) {
  const sc = screenById.value.get(screenId)
  return devices.models.find((m) => m.id === sc?.model_id)?.name ?? ''
}
// How long an item stays: dwell wakes × its refresh (or the device's rate).
function stay(dwell: number, rate: number | null | undefined) {
  if (rate == null) return dwell === 1 ? '1 wake at the device rate' : `${dwell} wakes at the device rate`
  return `≈ ${duration(dwell * rate)}`
}

const input = 'rounded-md border border-zinc-300 bg-white px-2.5 py-1.5 text-sm focus:border-zinc-500 focus:outline-none'
</script>

<template>
  <div class="mx-auto max-w-5xl px-8 py-8">
    <div class="mb-2 flex items-center justify-between">
      <h1 class="text-xl font-semibold">Playlists</h1>
      <button
        v-if="editingId == null"
        type="button"
        class="inline-flex items-center gap-1.5 rounded-md bg-zinc-900 px-3 py-1.5 text-sm text-white hover:bg-zinc-700"
        @click="edit(null)"
      >
        <Plus class="size-4" />
        New playlist
      </button>
    </div>
    <p class="mb-6 text-sm text-zinc-500">
      A device cycles through its playlist one wake at a time. Each item stays for a number of wakes, optionally with its own refresh
      interval. Items whose screen is for a different model than the device are skipped.
    </p>

    <p v-if="error" class="mb-4 rounded-md bg-red-50 px-4 py-2 text-sm text-red-700">{{ error }}</p>

    <!-- Editor -->
    <form v-if="editingId != null" class="mb-8 rounded-lg border border-zinc-300 bg-white p-4" @submit.prevent="save">
      <label class="mb-4 block text-sm">
        <span class="mb-1 block text-zinc-600">Name</span>
        <input v-model="name" :class="[input, 'w-72']" maxlength="100" placeholder="Living room" required />
      </label>

      <div class="mb-3 flex flex-col gap-2">
        <div v-for="(it, i) in items" :key="i" class="flex flex-wrap items-center gap-2 rounded-md border border-zinc-200 bg-zinc-50 px-3 py-2 text-sm">
          <span class="w-5 text-zinc-400">{{ i + 1 }}</span>
          <select v-model="it.screen_id" :class="[input, 'min-w-48']">
            <option v-for="sc in screens.screens" :key="sc.id" :value="sc.id">{{ sc.name }} · {{ modelName(sc.id) }}</option>
          </select>
          <label class="flex items-center gap-1.5">
            for
            <input v-model.number="it.dwell" type="number" min="1" max="1000" :class="[input, 'w-16']" />
            wake{{ it.dwell === 1 ? '' : 's' }}
          </label>
          <label class="flex items-center gap-1.5">
            <input v-model="it.custom" type="checkbox" />
            refresh every
          </label>
          <input v-if="it.custom" v-model.number="it.rate" type="number" min="30" max="86400" step="30" :class="[input, 'w-24']" title="seconds" />
          <span v-if="it.custom" class="text-zinc-400">s</span>
          <span class="text-xs text-zinc-500">{{ stay(it.dwell, it.custom ? it.rate : null) }}</span>
          <label class="ml-auto flex items-center gap-1.5 text-zinc-600">
            <input v-model="it.enabled" type="checkbox" />
            on
          </label>
          <button type="button" class="rounded p-1 text-zinc-400 hover:bg-zinc-200" title="Move up" @click="move(i, -1)"><ArrowUp class="size-4" /></button>
          <button type="button" class="rounded p-1 text-zinc-400 hover:bg-zinc-200" title="Move down" @click="move(i, 1)"><ArrowDown class="size-4" /></button>
          <button type="button" class="rounded p-1 text-zinc-400 hover:bg-zinc-200 hover:text-red-600" title="Remove" @click="items.splice(i, 1)"><X class="size-4" /></button>
        </div>
      </div>
      <button
        type="button"
        class="mb-4 inline-flex items-center gap-1.5 text-sm text-zinc-600 hover:text-zinc-900 disabled:opacity-50"
        :disabled="!screens.screens.length"
        @click="addItem"
      >
        <Plus class="size-4" />
        {{ screens.screens.length ? 'Add screen' : 'Create a screen first' }}
      </button>

      <div class="flex items-center gap-3">
        <button type="submit" class="rounded-md bg-zinc-900 px-3 py-1.5 text-sm text-white hover:bg-zinc-700 disabled:opacity-50" :disabled="saving">Save</button>
        <button type="button" class="text-sm text-zinc-500 hover:text-zinc-900" @click="editingId = null">Cancel</button>
      </div>
    </form>

    <!-- List -->
    <div
      v-if="playlists.loaded && !playlists.playlists.length && editingId == null"
      class="flex flex-col items-center gap-2 rounded-lg border border-dashed border-zinc-300 bg-white px-6 py-12 text-center text-sm text-zinc-500"
    >
      <ListVideo class="size-8 text-zinc-300" />
      <p>No playlists yet.</p>
    </div>
    <div class="flex flex-col gap-4">
      <div v-for="p in playlists.playlists" :key="p.id" class="rounded-lg border border-zinc-200 bg-white">
        <div class="flex items-center gap-3 border-b border-zinc-100 px-4 py-3">
          <span class="font-medium">{{ p.name }}</span>
          <span class="text-xs text-zinc-400">{{ p.device_count }} device{{ p.device_count === 1 ? '' : 's' }}</span>
          <button type="button" class="ml-auto text-sm text-zinc-600 hover:underline" @click="edit(p)">Edit</button>
          <button type="button" class="rounded p-1 text-zinc-400 hover:text-red-600" title="Delete" @click="remove(p)"><Trash2 class="size-4" /></button>
        </div>
        <div class="flex gap-3 overflow-x-auto p-4">
          <div v-if="!p.items.length" class="text-sm text-zinc-400">Empty</div>
          <div v-for="(it, i) in p.items" :key="it.id ?? i" class="w-44 shrink-0" :class="{ 'opacity-40': !it.enabled }">
            <img
              v-if="screenById.get(it.screen_id)?.latest_image"
              :src="imageUrl(screenById.get(it.screen_id)?.latest_image)!"
              alt=""
              class="mb-1 aspect-[4/3] w-full rounded-sm border border-zinc-200 bg-white object-contain"
            />
            <div v-else class="mb-1 aspect-[4/3] w-full rounded-sm border border-dashed border-zinc-300" />
            <div class="truncate text-sm">{{ i + 1 }}. {{ screenName(it.screen_id) }}</div>
            <div class="text-xs text-zinc-500">{{ stay(it.dwell, it.refresh_rate) }}</div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
