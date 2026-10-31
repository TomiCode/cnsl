<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { RouterLink } from 'vue-router'
import { Activity, BellDot, Database, MonitorSmartphone, RefreshCw } from 'lucide-vue-next'
import { useSystemStore } from '@/stores/system'
import { useDevicesStore } from '@/stores/devices'
import { useEventsStore } from '@/stores/events'

const system = useSystemStore()
const devices = useDevicesStore()
const events = useEventsStore()

onMounted(() => {
  events.connect()
  void devices.load()
})

const statusClass = computed(() => {
  if (!system.reachable) return 'bg-red-100 text-red-700'
  return system.health?.status === 'ok' ? 'bg-emerald-100 text-emerald-700' : 'bg-amber-100 text-amber-700'
})
const statusLabel = computed(() => (system.reachable ? (system.health?.status ?? '…') : 'unreachable'))
const active = computed(() => devices.devices.filter((d) => d.status === 'active').length)
</script>

<template>
  <div class="mx-auto max-w-4xl px-8 py-8">
    <div class="mb-6 flex items-center justify-between">
      <h1 class="text-xl font-semibold">Overview</h1>
      <button
        type="button"
        class="inline-flex items-center gap-1.5 rounded-md border border-zinc-200 bg-white px-3 py-1.5 text-sm hover:bg-zinc-50"
        @click="system.refreshHealth"
      >
        <RefreshCw class="size-4" />
        Refresh
      </button>
    </div>

    <RouterLink
      v-if="devices.pending.length"
      to="/devices"
      class="mb-4 flex items-center gap-3 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900 hover:bg-amber-100"
    >
      <BellDot class="size-5" />
      {{ devices.pending.length }} device{{ devices.pending.length > 1 ? 's' : '' }} waiting for approval
    </RouterLink>

    <div class="grid gap-4 sm:grid-cols-3">
      <div class="rounded-lg border border-zinc-200 bg-white p-5">
        <div class="mb-3 flex items-center gap-2 text-sm text-zinc-500">
          <Activity class="size-4" />
          Server
        </div>
        <span class="rounded-full px-2.5 py-0.5 text-sm font-medium" :class="statusClass">{{ statusLabel }}</span>
        <span class="ml-2 text-xs" :class="events.connected ? 'text-emerald-600' : 'text-zinc-400'">
          {{ events.connected ? 'live' : 'offline' }}
        </span>
      </div>
      <div class="rounded-lg border border-zinc-200 bg-white p-5">
        <div class="mb-3 flex items-center gap-2 text-sm text-zinc-500">
          <Database class="size-4" />
          Database
        </div>
        <span class="text-sm font-medium">{{ system.health?.database ?? '—' }}</span>
      </div>
      <RouterLink to="/devices" class="rounded-lg border border-zinc-200 bg-white p-5 hover:bg-zinc-50">
        <div class="mb-3 flex items-center gap-2 text-sm text-zinc-500">
          <MonitorSmartphone class="size-4" />
          Devices
        </div>
        <span class="text-sm font-medium">{{ active }} active</span>
      </RouterLink>
    </div>
  </div>
</template>
