<script setup lang="ts">
import type { Device } from '@/stores/devices'

defineProps<{ status: Device['status'] }>()

const styles: Record<Device['status'], string> = {
  pending: 'bg-amber-100 text-amber-800',
  active: 'bg-emerald-100 text-emerald-800',
  disabled: 'bg-zinc-200 text-zinc-600',
}
</script>

<template>
  <span class="inline-flex rounded-full px-2 py-0.5 text-xs font-medium" :class="styles[status]">{{ status }}</span>
</template>
