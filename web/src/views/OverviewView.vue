<script setup lang="ts">
import { computed } from 'vue'
import { Activity, Database, RefreshCw } from 'lucide-vue-next'
import { useSystemStore } from '@/stores/system'

const system = useSystemStore()

const statusClass = computed(() => {
  if (!system.reachable) return 'bg-red-100 text-red-700'
  return system.health?.status === 'ok' ? 'bg-emerald-100 text-emerald-700' : 'bg-amber-100 text-amber-700'
})
const statusLabel = computed(() => (system.reachable ? (system.health?.status ?? '…') : 'unreachable'))
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

    <div class="grid gap-4 sm:grid-cols-2">
      <div class="rounded-lg border border-zinc-200 bg-white p-5">
        <div class="mb-3 flex items-center gap-2 text-sm text-zinc-500">
          <Activity class="size-4" />
          Server
        </div>
        <span class="rounded-full px-2.5 py-0.5 text-sm font-medium" :class="statusClass">{{ statusLabel }}</span>
      </div>
      <div class="rounded-lg border border-zinc-200 bg-white p-5">
        <div class="mb-3 flex items-center gap-2 text-sm text-zinc-500">
          <Database class="size-4" />
          Database
        </div>
        <span class="text-sm font-medium">{{ system.health?.database ?? '—' }}</span>
      </div>
    </div>
  </div>
</template>
