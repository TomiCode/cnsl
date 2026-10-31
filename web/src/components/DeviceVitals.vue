<script setup lang="ts">
import { computed } from 'vue'
import { BatteryCharging, BatteryFull, BatteryLow, BatteryMedium, Wifi, WifiLow, WifiOff, WifiZero } from 'lucide-vue-next'
import type { Schemas } from '@/api/client'
import { signalQuality } from '@/lib/format'

const props = defineProps<{ telemetry?: Schemas['Telemetry'] }>()

const battery = computed(() => {
  const t = props.telemetry
  if (!t) return null
  const percent = t.battery_percent != null ? Math.round(t.battery_percent) : null
  const icon = t.charging
    ? BatteryCharging
    : percent == null || percent > 66
      ? BatteryFull
      : percent > 25
        ? BatteryMedium
        : BatteryLow
  const label = percent != null ? `${percent}%` : t.battery_voltage != null ? `${t.battery_voltage.toFixed(2)} V` : '—'
  return { icon, label, low: percent != null && percent <= 15 }
})

const signal = computed(() => {
  const q = signalQuality(props.telemetry?.rssi)
  const icon = { good: Wifi, fair: WifiLow, poor: WifiZero, unknown: WifiOff }[q]
  return { icon, label: props.telemetry?.rssi != null ? `${props.telemetry.rssi} dBm` : '—', poor: q === 'poor' }
})
</script>

<template>
  <div class="flex items-center gap-4 text-sm text-zinc-600">
    <span v-if="battery" class="inline-flex items-center gap-1" :class="{ 'text-red-600': battery.low }">
      <component :is="battery.icon" class="size-4" />
      {{ battery.label }}
    </span>
    <span class="inline-flex items-center gap-1" :class="{ 'text-red-600': signal.poor }">
      <component :is="signal.icon" class="size-4" />
      {{ signal.label }}
    </span>
  </div>
</template>
