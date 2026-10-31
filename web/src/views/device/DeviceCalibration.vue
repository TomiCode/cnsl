<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { Loader2, Play, RotateCcw, Square } from 'lucide-vue-next'
import { api, type Schemas } from '@/api/client'
import { unwrap, useDevicesStore, type Device } from '@/stores/devices'

const props = defineProps<{ device: Device }>()
const store = useDevicesStore()

const model = computed(() => store.modelOf(props.device))
const curve = reactive<Schemas['ToneCurve']>({ gamma: 1, black: 0, white: 1 })
const preview = ref<string | null>(null)
const busy = ref(false)
const previewing = ref(false)
const message = ref<{ ok: boolean; text: string } | null>(null)
const now = ref(Date.now())
let clock: ReturnType<typeof setInterval> | undefined

const calibrating = computed(() => {
  const until = props.device.calibrating_until
  return until != null && new Date(until).getTime() > now.value
})
const remaining = computed(() => {
  const until = props.device.calibrating_until
  return until ? Math.max(0, Math.round((new Date(until).getTime() - now.value) / 60000)) : 0
})
const saved = computed(() => model.value?.tone_curve)
const changed = computed(
  () => !!saved.value && (saved.value.gamma !== curve.gamma || saved.value.black !== curve.black || saved.value.white !== curve.white),
)

function resetToSaved() {
  if (saved.value) Object.assign(curve, saved.value)
}
watch(() => model.value?.id, resetToSaved, { immediate: true })

// Debounced preview of the pattern with the unsaved curve.
let timer: ReturnType<typeof setTimeout> | undefined
watch(curve, () => {
  clearTimeout(timer)
  timer = setTimeout(renderPreview, 400)
})

async function renderPreview() {
  if (!model.value) return
  previewing.value = true
  try {
    const res = unwrap(await api.POST('/device-models/{id}/tone-curve/preview', { params: { path: { id: model.value.id } }, body: { ...curve } }))
    preview.value = res.device
  } catch (e) {
    message.value = { ok: false, text: (e as Error).message }
  } finally {
    previewing.value = false
  }
}

async function run(action: () => Promise<unknown>, ok: string) {
  busy.value = true
  message.value = null
  try {
    await action()
    message.value = { ok: true, text: ok }
  } catch (e) {
    message.value = { ok: false, text: (e as Error).message }
  } finally {
    busy.value = false
  }
}

const apply = () =>
  run(() => store.setToneCurve(model.value!.id, { ...curve }), 'Curve saved. The device picks it up on its next wake; screens re-render.')

onMounted(() => {
  clock = setInterval(() => (now.value = Date.now()), 15000)
  void renderPreview()
})
onBeforeUnmount(() => {
  clearInterval(clock)
  clearTimeout(timer)
})

const sliders = [
  { key: 'gamma', label: 'Gamma', min: 0.3, max: 3, step: 0.05, help: '> 1 darkens mid-tones, < 1 lightens them' },
  { key: 'black', label: 'Black point', min: 0, max: 0.5, step: 0.01, help: 'inputs darker than this become black' },
  { key: 'white', label: 'White point', min: 0.5, max: 1, step: 0.01, help: 'inputs lighter than this become white' },
] as const
</script>

<template>
  <div v-if="!model" class="text-sm text-zinc-500">Assign a model to this device first.</div>
  <div v-else class="grid gap-6 lg:grid-cols-5">
    <div class="flex flex-col gap-6 lg:col-span-2">
      <div class="rounded-lg border border-zinc-200 bg-white p-4 text-sm">
        <div class="mb-1 font-medium">1. Show the pattern on the device</div>
        <p class="mb-3 text-xs text-zinc-500">
          The device shows the calibration pattern instead of its playlist and wakes every minute, for 30 minutes. Press its button to wake it
          right away.
        </p>
        <div class="flex items-center gap-3">
          <button
            v-if="!calibrating"
            type="button"
            class="inline-flex items-center gap-1.5 rounded-md bg-zinc-900 px-3 py-1.5 text-white hover:bg-zinc-700 disabled:opacity-50"
            :disabled="busy || device.status !== 'active'"
            @click="run(() => store.startCalibration(device.id), 'Calibration mode on')"
          >
            <Play class="size-4" />
            Start calibration
          </button>
          <button
            v-else
            type="button"
            class="inline-flex items-center gap-1.5 rounded-md border border-zinc-200 px-3 py-1.5 hover:bg-zinc-50 disabled:opacity-50"
            :disabled="busy"
            @click="run(() => store.stopCalibration(device.id), 'Calibration mode off')"
          >
            <Square class="size-4" />
            Stop
          </button>
          <span v-if="calibrating" class="text-xs text-emerald-700">on · {{ remaining }} min left</span>
          <span v-else-if="device.status !== 'active'" class="text-xs text-zinc-400">device must be active</span>
        </div>
      </div>

      <div class="rounded-lg border border-zinc-200 bg-white p-4 text-sm">
        <div class="mb-1 font-medium">2. Adjust until the 16 steps look even</div>
        <p class="mb-4 text-xs text-zinc-500">The curve belongs to the {{ model.name }} model and applies to all its screens and images.</p>
        <div v-for="s in sliders" :key="s.key" class="mb-4">
          <div class="mb-1 flex justify-between">
            <label :for="`curve-${s.key}`" class="text-zinc-600">{{ s.label }}</label>
            <span class="font-mono text-xs">{{ curve[s.key].toFixed(2) }}</span>
          </div>
          <input :id="`curve-${s.key}`" v-model.number="curve[s.key]" type="range" :min="s.min" :max="s.max" :step="s.step" class="w-full" />
          <p class="text-xs text-zinc-400">{{ s.help }}</p>
        </div>
        <div class="flex flex-wrap items-center gap-3">
          <button
            type="button"
            class="rounded-md bg-zinc-900 px-3 py-1.5 text-white hover:bg-zinc-700 disabled:opacity-50"
            :disabled="busy || !changed"
            @click="apply"
          >
            Save curve
          </button>
          <button type="button" class="inline-flex items-center gap-1 text-zinc-500 hover:text-zinc-900" :disabled="!changed" @click="resetToSaved">
            <RotateCcw class="size-3.5" />
            Revert
          </button>
          <button type="button" class="text-zinc-500 hover:text-zinc-900" @click="Object.assign(curve, { gamma: 1, black: 0, white: 1 })">Linear</button>
        </div>
        <p v-if="message" class="mt-3 text-xs" :class="message.ok ? 'text-emerald-700' : 'text-red-600'">{{ message.text }}</p>
      </div>
    </div>

    <div class="rounded-lg border border-zinc-200 bg-white p-4 lg:col-span-3">
      <div class="mb-3 flex items-center justify-between text-sm">
        <span class="font-medium">Pattern with this curve</span>
        <Loader2 v-if="previewing" class="size-4 animate-spin text-zinc-400" />
      </div>
      <img v-if="preview" :src="preview" alt="Calibration pattern preview" class="w-full rounded border border-zinc-200" />
      <div v-else class="flex aspect-[4/3] items-center justify-center rounded border border-dashed border-zinc-300 text-sm text-zinc-400">Rendering…</div>
      <p class="mt-2 text-xs text-zinc-400">A monitor shows grays differently from e-ink: judge on the device, not here.</p>
    </div>
  </div>
</template>
