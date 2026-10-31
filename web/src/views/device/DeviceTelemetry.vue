<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { api, type Schemas } from '@/api/client'
import { unwrap, type Device } from '@/stores/devices'
import { useEventsStore } from '@/stores/events'
import { dateTime } from '@/lib/format'

const props = defineProps<{ device: Device }>()
const rows = ref<Schemas['Telemetry'][]>([])
const error = ref<string | null>(null)

async function load() {
  try {
    rows.value = unwrap(await api.GET('/devices/{id}/telemetry', { params: { path: { id: props.device.id }, query: { limit: 200 } } }))
  } catch (e) {
    error.value = (e as Error).message
  }
}

const unsubscribe = useEventsStore().subscribe((e) => {
  if (e.type === 'device.seen' && e.device_id === props.device.id) void load()
})
onMounted(load)
onBeforeUnmount(unsubscribe)

function yesNo(v?: boolean | null) {
  return v == null ? '—' : v ? 'yes' : 'no'
}
</script>

<template>
  <div>
    <p class="mb-4 text-xs text-zinc-500">One row per device wake (each /api/display request). Charts come later.</p>
    <p v-if="error" class="mb-4 text-sm text-red-600">{{ error }}</p>
    <div class="overflow-x-auto rounded-lg border border-zinc-200 bg-white">
      <table class="w-full text-sm">
        <thead class="border-b border-zinc-200 bg-zinc-50 text-left text-xs text-zinc-500">
          <tr>
            <th class="px-3 py-2 font-medium">Time</th>
            <th class="px-3 py-2 font-medium">Battery</th>
            <th class="px-3 py-2 font-medium">RSSI</th>
            <th class="px-3 py-2 font-medium">Wake</th>
            <th class="px-3 py-2 font-medium">Awake</th>
            <th class="px-3 py-2 font-medium">USB</th>
            <th class="px-3 py-2 font-medium">Cached</th>
            <th class="px-3 py-2 font-medium">Firmware</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-zinc-100">
          <tr v-if="!rows.length">
            <td colspan="8" class="px-3 py-10 text-center text-zinc-400">No requests from this device yet.</td>
          </tr>
          <tr v-for="t in rows" :key="t.id">
            <td class="px-3 py-1.5 whitespace-nowrap">{{ dateTime(t.at) }}</td>
            <td class="px-3 py-1.5 whitespace-nowrap">
              <template v-if="t.battery_percent != null">{{ Math.round(t.battery_percent) }}% · </template>
              {{ t.battery_voltage != null ? `${t.battery_voltage.toFixed(2)} V` : '—' }}
            </td>
            <td class="px-3 py-1.5">{{ t.rssi ?? '—' }}</td>
            <td class="px-3 py-1.5">{{ t.wake_reason ?? '—' }}</td>
            <td class="px-3 py-1.5">{{ t.wake_time != null ? `${t.wake_time} ms` : '—' }}</td>
            <td class="px-3 py-1.5">{{ yesNo(t.usb_connected) }}</td>
            <td class="px-3 py-1.5">{{ yesNo(t.image_cached) }}</td>
            <td class="px-3 py-1.5">{{ t.firmware_version ?? '—' }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
