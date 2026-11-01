<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { Loader2, Play, RotateCcw, Square } from 'lucide-vue-next'
import { api, type Schemas } from '@/api/client'
import { unwrap, useDevicesStore, type Device } from '@/stores/devices'

type Curve = Schemas['ToneCurve']

const props = defineProps<{ device: Device }>()
const store = useDevicesStore()

const model = computed(() => store.modelOf(props.device))
const levelCount = computed(() => 2 ** (model.value?.bit_depth ?? 4))
const curve = reactive<{ gamma: number; black: number; white: number; levels: number[] | null }>({ gamma: 1, black: 0, white: 1, levels: null })
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

const even = () => Array.from({ length: levelCount.value }, (_, i) => Math.round((i / (levelCount.value - 1)) * 100) / 100)
const round2 = (v: number) => Math.round(v * 100) / 100

function body(): Curve {
  return { gamma: curve.gamma, black: curve.black, white: curve.white, levels: curve.levels ? curve.levels.map(round2) : null }
}
const savedJSON = computed(() => {
  const s = model.value?.tone_curve
  return s ? JSON.stringify({ gamma: s.gamma, black: s.black, white: s.white, levels: s.levels ? s.levels.map(round2) : null }) : ''
})
const changed = computed(() => JSON.stringify(body()) !== savedJSON.value)

function resetToSaved() {
  const s = model.value?.tone_curve
  if (!s) return
  curve.gamma = s.gamma
  curve.black = s.black
  curve.white = s.white
  curve.levels = s.levels ? s.levels.map(round2) : null
}
watch(() => model.value?.id, resetToSaved, { immediate: true })

// Debounced preview of the pattern with the unsaved calibration.
let timer: ReturnType<typeof setTimeout> | undefined
watch(curve, () => {
  clearTimeout(timer)
  timer = setTimeout(renderPreview, 400)
}, { deep: true })

async function renderPreview() {
  if (!model.value) return
  previewing.value = true
  try {
    const res = unwrap(await api.POST('/device-models/{id}/tone-curve/preview', { params: { path: { id: model.value.id } }, body: body() }))
    preview.value = res.panel ?? res.device
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
  run(() => store.setToneCurve(model.value!.id, body()), 'Calibration saved. The device picks it up on its next wake; screens re-render.')

function toggleMeasured(on: boolean) {
  curve.levels = on ? even() : null
}
function sameAsPrevious(i: number) {
  if (curve.levels && i > 0) curve.levels[i] = curve.levels[i - 1]!
}

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
  <div v-else class="space-y-6">
    <div class="grid gap-6 lg:grid-cols-2">
      <div class="rounded-lg border border-zinc-200 bg-white p-4 text-sm">
        <div class="mb-1 font-medium">1. Show the pattern on the device</div>
        <p class="mb-3 text-xs text-zinc-500">
          The device shows the calibration pattern instead of its playlist and wakes every minute, for 30 minutes. Press its button to wake it
          right away. After saving, the pattern updates on the next wake.
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
        <div class="mb-1 font-medium">3. Overall contrast (optional)</div>
        <p class="mb-3 text-xs text-zinc-500">Applied to every image before the levels; affects all screens of {{ model.name }}.</p>
        <div v-for="s in sliders" :key="s.key" class="mb-3">
          <div class="mb-1 flex justify-between">
            <label :for="`curve-${s.key}`" class="text-zinc-600">{{ s.label }}</label>
            <span class="font-mono text-xs">{{ curve[s.key].toFixed(2) }}</span>
          </div>
          <input :id="`curve-${s.key}`" v-model.number="curve[s.key]" type="range" :min="s.min" :max="s.max" :step="s.step" class="w-full" />
          <p class="text-xs text-zinc-400">{{ s.help }}</p>
        </div>
      </div>
    </div>

    <div class="rounded-lg border border-zinc-200 bg-white p-4 text-sm">
      <div class="mb-1 flex flex-wrap items-center gap-3">
        <span class="font-medium">2. Measure the {{ levelCount }} gray levels</span>
        <label class="flex items-center gap-1.5 text-zinc-600">
          <input type="checkbox" :checked="curve.levels !== null" @change="toggleMeasured(($event.target as HTMLInputElement).checked)" />
          use measured levels
        </label>
      </div>
      <p class="mb-4 text-xs text-zinc-500">
        Look at the numbered bands on the panel and set how bright each one looks (0 = black, 100 = white). Bands that look the same should
        get the same value; a band that looks lighter than its right neighbour may get a higher value. Photos are then dithered with the
        levels that really differ, and the previews show the panel's grays. The lower ramp on the pattern shows one step per distinct level.
      </p>
      <div v-if="curve.levels" class="flex gap-1 overflow-x-auto pb-2">
        <div v-for="(v, i) in curve.levels" :key="i" class="flex w-12 shrink-0 flex-col items-center gap-1">
          <span class="font-mono text-xs">{{ Math.round(v * 100) }}</span>
          <input
            v-model.number="curve.levels[i]"
            type="range"
            min="0"
            max="1"
            step="0.01"
            class="h-32 [direction:rtl] [writing-mode:vertical-lr]"
            :aria-label="`Level ${i} brightness`"
          />
          <span class="size-8 rounded border border-zinc-300" :style="{ background: `rgb(${v * 255},${v * 255},${v * 255})` }" />
          <span class="text-xs font-semibold">{{ i }}</span>
          <button v-if="i > 0" type="button" class="text-[10px] text-zinc-400 hover:text-zinc-900" title="Same as the band on the left" @click="sameAsPrevious(i)">= {{ i - 1 }}</button>
        </div>
      </div>
      <p v-else class="text-xs text-zinc-400">Off: the {{ levelCount }} levels are treated as evenly spaced.</p>

      <div class="mt-4 flex flex-wrap items-center gap-3 border-t border-zinc-100 pt-4">
        <button type="button" class="rounded-md bg-zinc-900 px-3 py-1.5 text-white hover:bg-zinc-700 disabled:opacity-50" :disabled="busy || !changed" @click="apply">
          Save calibration
        </button>
        <button type="button" class="inline-flex items-center gap-1 text-zinc-500 hover:text-zinc-900" :disabled="!changed" @click="resetToSaved">
          <RotateCcw class="size-3.5" />
          Revert
        </button>
        <button v-if="curve.levels" type="button" class="text-zinc-500 hover:text-zinc-900" @click="curve.levels = even()">Even levels</button>
        <span v-if="message" class="text-xs" :class="message.ok ? 'text-emerald-700' : 'text-red-600'">{{ message.text }}</span>
      </div>
    </div>

    <div class="rounded-lg border border-zinc-200 bg-white p-4">
      <div class="mb-3 flex items-center justify-between text-sm">
        <span class="font-medium">Pattern as the panel should show it</span>
        <Loader2 v-if="previewing" class="size-4 animate-spin text-zinc-400" />
      </div>
      <img v-if="preview" :src="preview" alt="Calibration pattern preview" class="w-full rounded border border-zinc-200" />
      <div v-else class="flex aspect-[4/3] items-center justify-center rounded border border-dashed border-zinc-300 text-sm text-zinc-400">Rendering…</div>
      <p class="mt-2 text-xs text-zinc-400">
        Emulated with the levels above. If it matches the panel, previews everywhere will too. A monitor can't show e-ink exactly, so
        judge smoothness on the device.
      </p>
    </div>
  </div>
</template>
