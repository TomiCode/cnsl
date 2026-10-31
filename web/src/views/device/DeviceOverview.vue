<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { Eye, EyeOff, TriangleAlert } from 'lucide-vue-next'
import DeviceVitals from '@/components/DeviceVitals.vue'
import { useDevicesStore, type Device } from '@/stores/devices'
import { usePlaylistsStore } from '@/stores/playlists'
import { dateTime, duration, imageUrl, timeAgo } from '@/lib/format'

const props = defineProps<{ device: Device }>()
const store = useDevicesStore()

const model = computed(() => store.modelOf(props.device))
const shown = computed(() => imageUrl(props.device.latest?.served_filename))
const showKey = ref(false)

const sizeMismatch = computed(() => {
  const d = props.device
  const m = model.value
  return m && d.reported_width && d.reported_height && (d.reported_width !== m.width || d.reported_height !== m.height)
})

// Settings form
const form = reactive({ label: '', model_id: null as number | null, refresh_rate: 900, sleep: false, sleep_start: '22:00', sleep_end: '06:00' })
const saving = ref(false)
const message = ref<{ ok: boolean; text: string } | null>(null)

function reset() {
  const d = props.device
  form.label = d.label
  form.model_id = d.model_id ?? null
  form.refresh_rate = d.refresh_rate
  form.sleep = !!(d.sleep_start && d.sleep_end)
  form.sleep_start = d.sleep_start ?? '22:00'
  form.sleep_end = d.sleep_end ?? '06:00'
}
watch(() => props.device.id, reset, { immediate: true })

const playlists = usePlaylistsStore()
const playlistBusy = ref(false)
onMounted(() => {
  if (!playlists.loaded) void playlists.load()
})
async function changePlaylist(e: Event) {
  const v = (e.target as HTMLSelectElement).value
  playlistBusy.value = true
  message.value = null
  try {
    await store.setPlaylist(props.device.id, v === '' ? null : Number(v))
  } catch (err) {
    message.value = { ok: false, text: (err as Error).message }
  } finally {
    playlistBusy.value = false
  }
}

async function save() {
  saving.value = true
  message.value = null
  try {
    await store.update(props.device.id, {
      label: form.label,
      model_id: form.model_id ?? undefined,
      refresh_rate: form.refresh_rate,
      sleep_start: form.sleep ? form.sleep_start : '',
      sleep_end: form.sleep ? form.sleep_end : '',
    })
    message.value = { ok: true, text: 'Saved' }
  } catch (e) {
    message.value = { ok: false, text: (e as Error).message }
  } finally {
    saving.value = false
  }
}

const input =
  'w-full rounded-md border border-zinc-300 bg-white px-3 py-1.5 text-sm focus:border-zinc-500 focus:outline-none'
</script>

<template>
  <div class="grid gap-6 lg:grid-cols-5">
    <div class="flex flex-col gap-6 lg:col-span-3">
      <div class="rounded-lg border border-zinc-200 bg-white p-4">
        <div class="mb-3 flex items-center justify-between text-sm">
          <span class="font-medium">Currently shown</span>
          <span class="text-xs text-zinc-400">{{ device.latest ? `served ${timeAgo(device.latest.at)}` : 'never served' }}</span>
        </div>
        <img
          v-if="shown"
          :src="shown"
          alt="Image last served to the device"
          class="w-full rounded border border-zinc-200 bg-white"
          :style="model ? { aspectRatio: `${model.width} / ${model.height}` } : undefined"
        />
        <div v-else class="flex aspect-[4/3] items-center justify-center rounded border border-dashed border-zinc-300 text-sm text-zinc-400">
          Nothing served yet
        </div>
      </div>

      <div class="rounded-lg border border-zinc-200 bg-white p-4">
        <div class="mb-3 text-sm font-medium">Latest status</div>
        <DeviceVitals :telemetry="device.latest" class="mb-3" />
        <dl class="grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
          <dt class="text-zinc-500">Last seen</dt>
          <dd :title="dateTime(device.last_seen_at)">{{ timeAgo(device.last_seen_at) }}</dd>
          <dt class="text-zinc-500">Wake reason</dt>
          <dd>{{ device.latest?.wake_reason ?? '—' }}</dd>
          <dt class="text-zinc-500">Battery voltage</dt>
          <dd>{{ device.latest?.battery_voltage != null ? `${device.latest.battery_voltage.toFixed(2)} V` : '—' }}</dd>
          <dt class="text-zinc-500">USB / charging</dt>
          <dd>
            {{ device.latest?.usb_connected == null ? '—' : device.latest.usb_connected ? 'connected' : 'unplugged' }}
            <template v-if="device.latest?.charging"> · charging</template>
          </dd>
          <dt class="text-zinc-500">Firmware</dt>
          <dd>{{ device.firmware_version ?? '—' }}</dd>
        </dl>
      </div>
    </div>

    <div class="flex flex-col gap-6 lg:col-span-2">
      <div class="rounded-lg border border-zinc-200 bg-white p-4">
        <div class="mb-1 text-sm font-medium">Content</div>
        <p class="mb-3 text-xs text-zinc-500">Without a playlist the device shows a placeholder frame. Photos go into playlists as image screens.</p>
        <select :value="device.playlist_id ?? ''" :class="input" :disabled="playlistBusy" @change="changePlaylist">
          <option value="">No playlist</option>
          <option v-for="p in playlists.playlists" :key="p.id" :value="p.id">{{ p.name }} ({{ p.items.length }} items)</option>
        </select>
        <p v-if="device.calibrating_until && new Date(device.calibrating_until) > new Date()" class="mt-2 text-xs text-amber-700">
          Showing the calibration pattern until {{ dateTime(device.calibrating_until) }}.
        </p>
      </div>

      <form class="rounded-lg border border-zinc-200 bg-white p-4" @submit.prevent="save">
        <div class="mb-4 text-sm font-medium">Settings</div>
        <label class="mb-3 block text-sm">
          <span class="mb-1 block text-zinc-600">Label</span>
          <input v-model="form.label" :class="input" :placeholder="device.friendly_id" maxlength="100" />
        </label>
        <label class="mb-3 block text-sm">
          <span class="mb-1 block text-zinc-600">Model</span>
          <select v-model="form.model_id" :class="input">
            <option :value="null" disabled>Choose a model</option>
            <option v-for="m in store.models" :key="m.id" :value="m.id">
              {{ m.name }} ({{ m.width }}×{{ m.height }}, {{ 2 ** m.bit_depth }} grays)
            </option>
          </select>
        </label>
        <label class="mb-3 block text-sm">
          <span class="mb-1 block text-zinc-600">Refresh every (seconds)</span>
          <input v-model.number="form.refresh_rate" type="number" min="30" max="86400" :class="input" />
          <span class="mt-1 block text-xs text-zinc-400">{{ duration(form.refresh_rate || 0) }}</span>
        </label>
        <label class="mb-2 flex items-center gap-2 text-sm">
          <input v-model="form.sleep" type="checkbox" class="rounded border-zinc-300" />
          <span class="text-zinc-600">Sleep window (no refreshes)</span>
        </label>
        <div v-if="form.sleep" class="mb-3 flex items-center gap-2 text-sm">
          <input v-model="form.sleep_start" type="time" :class="input" />
          <span class="text-zinc-400">to</span>
          <input v-model="form.sleep_end" type="time" :class="input" />
        </div>
        <div class="mt-4 flex items-center gap-3">
          <button type="submit" class="rounded-md bg-zinc-900 px-3 py-1.5 text-sm text-white hover:bg-zinc-700 disabled:opacity-50" :disabled="saving">
            Save
          </button>
          <button type="button" class="text-sm text-zinc-500 hover:text-zinc-900" @click="reset">Reset</button>
          <span v-if="message" class="text-sm" :class="message.ok ? 'text-emerald-600' : 'text-red-600'">{{ message.text }}</span>
        </div>
      </form>

      <div class="rounded-lg border border-zinc-200 bg-white p-4">
        <div class="mb-3 text-sm font-medium">Identity</div>
        <p v-if="sizeMismatch" class="mb-3 flex gap-2 rounded-md bg-amber-50 px-3 py-2 text-xs text-amber-800">
          <TriangleAlert class="size-4 shrink-0" />
          The device reports {{ device.reported_width }}×{{ device.reported_height }}, but its model is {{ model?.width }}×{{ model?.height }}.
        </p>
        <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
          <dt class="text-zinc-500">Pairing code</dt>
          <dd class="font-mono">{{ device.friendly_id }}</dd>
          <dt class="text-zinc-500">MAC</dt>
          <dd class="font-mono">{{ device.mac }}</dd>
          <dt class="text-zinc-500">Reported</dt>
          <dd>
            {{ device.reported_model ?? '—' }}
            <template v-if="device.reported_width">· {{ device.reported_width }}×{{ device.reported_height }}</template>
          </dd>
          <dt class="text-zinc-500">API key</dt>
          <dd class="flex min-w-0 items-center gap-1">
            <span class="truncate font-mono text-xs">{{ showKey ? device.api_key : '••••••••••••' }}</span>
            <button type="button" class="text-zinc-400 hover:text-zinc-700" :title="showKey ? 'Hide' : 'Show'" @click="showKey = !showKey">
              <component :is="showKey ? EyeOff : Eye" class="size-4" />
            </button>
          </dd>
          <dt class="text-zinc-500">Added</dt>
          <dd>{{ dateTime(device.created_at) }}</dd>
        </dl>
      </div>
    </div>
  </div>
</template>
