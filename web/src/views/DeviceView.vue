<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { ArrowLeft, Ban, Check, Trash2 } from 'lucide-vue-next'
import StatusBadge from '@/components/StatusBadge.vue'
import DeviceOverview from './device/DeviceOverview.vue'
import DeviceImage from './device/DeviceImage.vue'
import DeviceLogs from './device/DeviceLogs.vue'
import DeviceTelemetry from './device/DeviceTelemetry.vue'
import { useDevicesStore } from '@/stores/devices'

const route = useRoute()
const router = useRouter()
const store = useDevicesStore()

const id = computed(() => Number(route.params.id))
const device = computed(() => store.byId(id.value))
const error = ref<string | null>(null)
const busy = ref(false)

const tabs = [
  { key: 'overview', label: 'Overview', component: DeviceOverview },
  { key: 'image', label: 'Image', component: DeviceImage },
  { key: 'logs', label: 'Logs', component: DeviceLogs },
  { key: 'telemetry', label: 'Telemetry', component: DeviceTelemetry },
] as const
const tab = computed(() => tabs.find((t) => t.key === route.query.tab) ?? tabs[0])

onMounted(async () => {
  try {
    if (!store.loaded) await store.load()
    else await store.refreshOne(id.value)
  } catch (e) {
    error.value = (e as Error).message
  }
})

async function run(action: () => Promise<unknown>) {
  busy.value = true
  error.value = null
  try {
    await action()
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

function remove() {
  if (!device.value) return
  if (confirm('Delete this device? Its logs and telemetry are removed too. It reappears as pending if it contacts the server again.')) {
    void run(async () => {
      await store.remove(id.value)
      await router.push('/devices')
    })
  }
}
</script>

<template>
  <div class="mx-auto max-w-5xl px-8 py-8">
    <RouterLink to="/devices" class="mb-4 inline-flex items-center gap-1 text-sm text-zinc-500 hover:text-zinc-900">
      <ArrowLeft class="size-4" />
      Devices
    </RouterLink>

    <p v-if="error" class="mb-4 rounded-md bg-red-50 px-4 py-2 text-sm text-red-700">{{ error }}</p>

    <div v-if="!device && store.loaded" class="text-sm text-zinc-500">Device not found.</div>

    <template v-if="device">
      <div class="mb-6 flex flex-wrap items-center gap-3">
        <h1 class="text-xl font-semibold">{{ device.label || device.friendly_id }}</h1>
        <StatusBadge :status="device.status" />
        <div class="ml-auto flex gap-2">
          <button
            v-if="device.status !== 'active'"
            type="button"
            class="inline-flex items-center gap-1.5 rounded-md bg-zinc-900 px-3 py-1.5 text-sm text-white hover:bg-zinc-700 disabled:opacity-50"
            :disabled="busy"
            @click="run(() => store.approve(device!.id))"
          >
            <Check class="size-4" />
            {{ device.status === 'pending' ? 'Approve' : 'Enable' }}
          </button>
          <button
            v-else
            type="button"
            class="inline-flex items-center gap-1.5 rounded-md border border-zinc-200 bg-white px-3 py-1.5 text-sm hover:bg-zinc-50 disabled:opacity-50"
            :disabled="busy"
            @click="run(() => store.disable(device!.id))"
          >
            <Ban class="size-4" />
            Disable
          </button>
          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-md border border-zinc-200 bg-white px-3 py-1.5 text-sm text-red-600 hover:bg-red-50 disabled:opacity-50"
            :disabled="busy"
            @click="remove"
          >
            <Trash2 class="size-4" />
            Delete
          </button>
        </div>
      </div>

      <nav class="mb-6 flex gap-1 border-b border-zinc-200">
        <RouterLink
          v-for="t in tabs"
          :key="t.key"
          :to="{ query: t.key === 'overview' ? {} : { tab: t.key } }"
          class="-mb-px border-b-2 px-3 py-2 text-sm"
          :class="t.key === tab.key ? 'border-zinc-900 font-medium text-zinc-900' : 'border-transparent text-zinc-500 hover:text-zinc-900'"
        >
          {{ t.label }}
        </RouterLink>
      </nav>

      <component :is="tab.component" :device="device" />
    </template>
  </div>
</template>
