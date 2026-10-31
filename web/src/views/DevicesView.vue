<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { Check, MonitorSmartphone, Trash2, TestTube2 } from 'lucide-vue-next'
import StatusBadge from '@/components/StatusBadge.vue'
import DeviceVitals from '@/components/DeviceVitals.vue'
import { useDevicesStore, type Device } from '@/stores/devices'
import { timeAgo } from '@/lib/format'

const store = useDevicesStore()
const error = ref<string | null>(null)
const busy = ref<number | null>(null)

onMounted(() => store.load().catch((e: Error) => (error.value = e.message)))

async function run(id: number, action: () => Promise<unknown>) {
  busy.value = id
  error.value = null
  try {
    await action()
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = null
  }
}

function reject(device: Device) {
  if (confirm(`Remove pending device ${device.friendly_id}? It will reappear if it contacts the server again.`)) {
    void run(device.id, () => store.remove(device.id))
  }
}

function name(device: Device) {
  return device.label || device.friendly_id
}
</script>

<template>
  <div class="mx-auto max-w-5xl px-8 py-8">
    <div class="mb-6 flex items-center justify-between">
      <h1 class="text-xl font-semibold">Devices</h1>
      <RouterLink
        to="/virtual"
        class="inline-flex items-center gap-1.5 rounded-md border border-zinc-200 bg-white px-3 py-1.5 text-sm hover:bg-zinc-50"
      >
        <TestTube2 class="size-4" />
        Virtual device
      </RouterLink>
    </div>

    <p v-if="error" class="mb-4 rounded-md bg-red-50 px-4 py-2 text-sm text-red-700">{{ error }}</p>

    <section v-if="store.pending.length" class="mb-8">
      <h2 class="mb-3 text-sm font-medium text-zinc-500">Waiting for approval</h2>
      <div class="flex flex-col gap-2">
        <div
          v-for="d in store.pending"
          :key="d.id"
          class="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-lg border border-amber-200 bg-amber-50/50 px-4 py-3"
        >
          <div class="font-mono text-lg font-semibold tracking-wider">{{ d.friendly_id }}</div>
          <div class="min-w-56 flex-1 text-sm text-zinc-600">
            <div>
              {{ store.modelOf(d)?.name ?? d.reported_model ?? 'Unknown model' }}
              <span class="text-zinc-400">· {{ d.mac }}</span>
            </div>
            <div class="text-xs text-zinc-400">
              First seen {{ timeAgo(d.created_at) }}<template v-if="d.firmware_version"> · firmware {{ d.firmware_version }}</template>
            </div>
          </div>
          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-md bg-zinc-900 px-3 py-1.5 text-sm text-white hover:bg-zinc-700 disabled:opacity-50"
            :disabled="busy === d.id || d.model_id == null"
            :title="d.model_id == null ? 'Assign a model first (open the device)' : undefined"
            @click="run(d.id, () => store.approve(d.id))"
          >
            <Check class="size-4" />
            Approve
          </button>
          <RouterLink :to="`/devices/${d.id}`" class="text-sm text-zinc-600 hover:underline">Details</RouterLink>
          <button
            type="button"
            class="rounded-md p-1.5 text-zinc-400 hover:bg-zinc-100 hover:text-red-600"
            title="Remove"
            @click="reject(d)"
          >
            <Trash2 class="size-4" />
          </button>
        </div>
      </div>
    </section>

    <section>
      <h2 v-if="store.pending.length" class="mb-3 text-sm font-medium text-zinc-500">Devices</h2>
      <div
        v-if="store.loaded && !store.registered.length"
        class="flex flex-col items-center gap-2 rounded-lg border border-dashed border-zinc-300 bg-white px-6 py-12 text-center text-sm text-zinc-500"
      >
        <MonitorSmartphone class="size-8 text-zinc-300" />
        <p>No devices yet.</p>
        <p class="max-w-md text-xs text-zinc-400">
          Point your TRMNL at this server's device URL in its Wi-Fi setup. It will show up here as pending. Or try a
          <RouterLink to="/virtual" class="underline">virtual device</RouterLink>.
        </p>
      </div>
      <div v-else-if="store.registered.length" class="overflow-x-auto rounded-lg border border-zinc-200 bg-white">
        <table class="w-full text-sm">
          <thead class="border-b border-zinc-200 bg-zinc-50 text-left text-xs text-zinc-500">
            <tr>
              <th class="px-4 py-2 font-medium">Device</th>
              <th class="px-4 py-2 font-medium">Status</th>
              <th class="px-4 py-2 font-medium">Battery &amp; signal</th>
              <th class="px-4 py-2 font-medium">Last seen</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-zinc-100">
            <tr v-for="d in store.registered" :key="d.id" class="hover:bg-zinc-50">
              <td class="px-4 py-3">
                <RouterLink :to="`/devices/${d.id}`" class="font-medium hover:underline">{{ name(d) }}</RouterLink>
                <div class="text-xs text-zinc-400">{{ store.modelOf(d)?.name ?? '—' }}</div>
              </td>
              <td class="px-4 py-3"><StatusBadge :status="d.status" /></td>
              <td class="px-4 py-3"><DeviceVitals :telemetry="d.latest" /></td>
              <td class="px-4 py-3 text-zinc-600">{{ timeAgo(d.last_seen_at) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>
</template>
