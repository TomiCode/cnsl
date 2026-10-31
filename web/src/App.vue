<script setup lang="ts">
import { onMounted } from 'vue'
import { RouterLink, RouterView } from 'vue-router'
import { LayoutDashboard, MonitorSmartphone, PanelsTopLeft, ListVideo, Plug, Settings } from 'lucide-vue-next'
import { useSystemStore } from '@/stores/system'

const system = useSystemStore()
onMounted(system.refreshHealth)

const nav = [
  { to: '/', label: 'Overview', icon: LayoutDashboard },
  { to: '/devices', label: 'Devices', icon: MonitorSmartphone },
  { to: '/screens', label: 'Screens', icon: PanelsTopLeft },
  { to: '/playlists', label: 'Playlists', icon: ListVideo },
  { to: '/connectors', label: 'Connectors', icon: Plug },
  { to: '/settings', label: 'Settings', icon: Settings },
]
</script>

<template>
  <div class="flex h-full">
    <aside class="flex w-56 shrink-0 flex-col border-r border-zinc-200 bg-white">
      <div class="px-5 py-4 font-mono text-lg font-semibold tracking-tight">cnsl</div>
      <nav class="flex flex-col gap-0.5 px-2">
        <RouterLink
          v-for="item in nav"
          :key="item.to"
          :to="item.to"
          class="flex items-center gap-2.5 rounded-md px-3 py-2 text-sm text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900"
          active-class="bg-zinc-100 font-medium text-zinc-900"
          :exact-active-class="item.to === '/' ? 'bg-zinc-100 font-medium text-zinc-900' : undefined"
        >
          <component :is="item.icon" class="size-4" />
          {{ item.label }}
        </RouterLink>
      </nav>
      <div class="mt-auto px-5 py-4 text-xs text-zinc-400">
        <span v-if="system.health">{{ system.health.version }}</span>
        <span v-else-if="!system.reachable" class="text-red-500">server unreachable</span>
      </div>
    </aside>
    <main class="min-w-0 flex-1 overflow-y-auto">
      <RouterView />
    </main>
  </div>
</template>
