<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { RouterLink, onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import { ArrowLeft, CircleAlert, CircleCheck, Eye, Loader2, Play, Plus, Save, Trash2, X } from 'lucide-vue-next'
import CodeEditor from '@/components/CodeEditor.vue'
import BackgroundPanel from '@/components/BackgroundPanel.vue'
import DataPanel from '@/components/DataPanel.vue'
import { dataPaths } from '@/lib/datapaths'
import { setDataFields } from '@/lib/monaco'
import { api, type Schemas } from '@/api/client'
import { ApiError, unwrap, useDevicesStore } from '@/stores/devices'
import { useEventsStore } from '@/stores/events'
import { useScreensStore } from '@/stores/screens'
import { dateTime, duration, imageUrl, timeAgo } from '@/lib/format'
import type { TemplateError } from '@/lib/monaco'

type Layout = Schemas['Layout']
interface DraftRegion {
  id?: number
  key: string
  template: string
  transparent: boolean
  x: number
  y: number
  w: number
  h: number
  z: number
}

const route = useRoute()
const router = useRouter()
const devices = useDevicesStore()
const screensStore = useScreensStore()
const id = computed(() => Number(route.params.id))

const screen = ref<Schemas['Screen'] | null>(null)
const functions = ref<string[]>([])
const layouts = ref<Layout[]>([])
const draft = reactive({
  name: '',
  model_id: 0,
  render_interval: 900,
  head: '',
  layout: 'full',
  background: { kind: 'none' } as Schemas['Background'],
  regions: [] as DraftRegion[],
})
type Draft = typeof draft
const tab = ref('')
const editor = ref<InstanceType<typeof CodeEditor> | null>(null)

const preview = ref<Schemas['Preview'] | null>(null)
const previewMode = ref<'device' | 'raw'>('device')
const zoom = ref<'fit' | 'actual'>('fit')
const previewing = ref(false)
const saving = ref(false)
const errors = ref<TemplateError[]>([])
const message = ref<{ ok: boolean; text: string } | null>(null)
const renders = ref<Schemas['Render'][]>([])
const screenData = ref<Record<string, Schemas['BindingData']>>({})
const sample = ref(false)

const isImage = computed(() => screen.value?.kind === 'image')
const model = computed(() => devices.models.find((m) => m.id === draft.model_id))
const isFree = computed(() => draft.layout === 'free')
const activeRegion = computed(() => draft.regions.find((r) => r.key === tab.value))

const slotTemplate = `<div class="layout layout--col layout--center">\n  <span class="value value--large">{{ .Region.Key }}</span>\n</div>\n<div class="title_bar">\n  <span class="title">{{ .Screen.Name }}</span>\n</div>\n`
const freeTemplate = `<span class="value value--large value--tnums">{{ date "15:04" .Now }}</span>\n`

function fromScreen(s: Schemas['Screen']): Draft {
  return {
    name: s.name,
    model_id: s.model_id,
    render_interval: s.render_interval,
    head: s.head,
    layout: s.layout,
    background: { ...s.background, asset_ids: [...(s.background.asset_ids ?? [])] },
    regions: s.regions.map((r) => ({ id: r.id, key: r.key, template: r.template, transparent: r.transparent, x: r.x, y: r.y, w: r.w, h: r.h, z: r.z })),
  }
}
const snapshot = (d: Draft) => JSON.stringify(d)
const dirty = computed(() => !!screen.value && snapshot(draft) !== snapshot(fromScreen(screen.value)))
// Layout, region set and background only show in previews after saving.
const structuralDirty = computed(() => {
  const s = screen.value
  if (!s) return false
  const shape = (d: Draft) =>
    JSON.stringify([d.layout, d.background, d.model_id, d.regions.map((r) => [r.key, r.transparent, r.x, r.y, r.w, r.h, r.z])])
  return shape(draft) !== shape(fromScreen(s))
})

function reset(s: Schemas['Screen']) {
  screen.value = s
  Object.assign(draft, fromScreen(s))
  if (!tab.value || (!draft.regions.some((r) => r.key === tab.value) && !['head', 'background', 'regions'].includes(tab.value))) {
    tab.value = s.kind === 'image' ? 'background' : (draft.regions[0]?.key ?? 'head')
  }
}

async function loadData() {
  try {
    screenData.value = unwrap(await api.GET('/screens/{id}/data', { params: { path: { id: id.value }, query: { sample: sample.value } } }))
    setDataFields(dataPaths(screenData.value))
  } catch (e) {
    message.value = { ok: false, text: (e as Error).message }
  }
}

async function loadRenders() {
  renders.value = unwrap(await api.GET('/screens/{id}/renders', { params: { path: { id: id.value } } }))
}

onMounted(async () => {
  try {
    const [sc, sys] = await Promise.all([
      api.GET('/screens/{id}', { params: { path: { id: id.value } } }),
      api.GET('/system'),
      devices.loaded ? Promise.resolve() : devices.load(),
    ])
    const system = unwrap(sys)
    functions.value = system.template_functions
    layouts.value = system.layouts
    reset(unwrap(sc))
    await loadRenders()
    void runPreview()
    if (!isImage.value) void loadData()
  } catch (e) {
    message.value = { ok: false, text: (e as Error).message }
  }
})

const unsubscribe = useEventsStore().subscribe((e) => {
  if (e.type === 'screen.rendered' && e.screen_id === id.value) void loadRenders()
})
onBeforeUnmount(unsubscribe)

// Mirrors render.ApplyLayout: keep regions whose key matches a slot, move the others' content
// into the remaining slots, create the rest from the slot template.
function changeLayout(key: string) {
  const layout = layouts.value.find((l) => l.key === key)
  if (!layout || !model.value) return
  if (key === 'free') {
    draft.layout = key
    if (!draft.regions.length) addFreeRegion()
    return
  }
  const byKey = new Map(draft.regions.map((r) => [r.key, r]))
  const leftovers = draft.regions.filter((r) => !layout.slots.some((s) => s.key === r.key))
  draft.regions = layout.slots.map((s) => {
    let r = byKey.get(s.key)
    if (!r) {
      const reused = leftovers.shift()
      r = reused ? { ...reused, key: s.key } : { key: s.key, template: s.view === 'full' ? slotTemplate : slotTemplate, transparent: false, x: 0, y: 0, w: 0, h: 0, z: 0 }
    }
    return {
      ...r,
      x: Math.round(s.x * model.value!.width),
      y: Math.round(s.y * model.value!.height),
      w: Math.round(s.w * model.value!.width),
      h: Math.round(s.h * model.value!.height),
      z: 0,
    }
  })
  draft.layout = key
  if (!draft.regions.some((r) => r.key === tab.value)) tab.value = draft.regions[0]?.key ?? 'head'
}

function addFreeRegion() {
  const m = model.value
  if (!m) return
  let n = draft.regions.length + 1
  while (draft.regions.some((r) => r.key === `region_${n}`)) n++
  const w = Math.round(m.width / 3)
  const h = Math.round(m.height / 4)
  draft.regions.push({ key: `region_${n}`, template: freeTemplate, transparent: true, x: Math.round(m.width / 20), y: Math.round(m.height / 20), w, h, z: draft.regions.length })
  tab.value = 'regions'
}

function removeRegion(i: number) {
  draft.regions.splice(i, 1)
}

async function runPreview() {
  if (!screen.value || previewing.value) return
  previewing.value = true
  try {
    const templates = Object.fromEntries(draft.regions.map((r) => [r.key, r.template]))
    const res = unwrap(
      await api.POST('/screens/{id}/preview', { params: { path: { id: id.value } }, body: { templates, head: draft.head, sample: sample.value } }),
    )
    preview.value = res
    errors.value = res.errors
  } catch (e) {
    message.value = { ok: false, text: (e as Error).message }
  } finally {
    previewing.value = false
  }
}

async function save() {
  if (!screen.value || saving.value) return
  saving.value = true
  message.value = null
  try {
    const res = await api.PUT('/screens/{id}', {
      params: { path: { id: id.value } },
      body: {
        name: draft.name,
        model_id: draft.model_id,
        render_interval: draft.render_interval,
        head: draft.head,
        layout: draft.layout,
        background: draft.background,
        regions: draft.regions.map((r) => ({ ...r })),
      },
    })
    if (res.error) {
      const p = res.error as Schemas['TemplateProblem']
      if (p.template_errors?.length) {
        errors.value = p.template_errors
        tab.value = p.template_errors[0]!.region
        throw new ApiError(`Not saved: ${p.template_errors.length} template error(s)`)
      }
      unwrap(res)
    }
    reset(res.data!)
    message.value = { ok: true, text: 'Saved — rendering' }
    void runPreview()
  } catch (e) {
    message.value = { ok: false, text: (e as Error).message }
  } finally {
    saving.value = false
  }
}

async function renderNow() {
  try {
    const r = unwrap(await api.POST('/screens/{id}/renders', { params: { path: { id: id.value } } }))
    message.value = r.status === 'ok' ? { ok: true, text: 'Rendered' } : { ok: false, text: r.error ?? 'Render failed' }
    await loadRenders()
    void runPreview()
  } catch (e) {
    message.value = { ok: false, text: (e as Error).message }
  }
}

async function remove() {
  if (!confirm(`Delete screen "${screen.value?.name}"? It is removed from all playlists.`)) return
  await screensStore.remove(id.value)
  if (screen.value) Object.assign(draft, fromScreen(screen.value)) // not dirty anymore
  await router.push('/screens')
}

onBeforeRouteLeave(() => !dirty.value || confirm('Leave without saving your changes?'))
const beforeUnload = (e: BeforeUnloadEvent) => {
  if (dirty.value) e.preventDefault()
}
window.addEventListener('beforeunload', beforeUnload)
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload))

function reveal(e: TemplateError) {
  tab.value = e.region
  setTimeout(() => editor.value?.revealLine(e.line))
}

const tabs = computed(() => {
  if (isImage.value) return [{ key: 'background', label: 'Image' }]
  return [
    ...draft.regions.map((r) => ({ key: r.key, label: r.key })),
    ...(isFree.value ? [{ key: 'regions', label: 'Regions' }] : []),
    { key: 'data', label: 'Data' },
    { key: 'background', label: 'Background' },
    { key: 'head', label: 'Head (CSS)' },
  ]
})
const regionErrors = computed(() => errors.value.filter((e) => e.region === tab.value))

const intervals = [60, 300, 600, 900, 1800, 3600, 3 * 3600, 6 * 3600, 12 * 3600, 86400]
const btn = 'inline-flex items-center gap-1.5 rounded-md border border-zinc-200 bg-white px-3 py-1.5 text-sm hover:bg-zinc-50 disabled:opacity-50'
const input = 'rounded-md border border-zinc-300 bg-white px-2.5 py-1.5 text-sm focus:border-zinc-500 focus:outline-none'
const num = 'w-20 rounded-md border border-zinc-300 bg-white px-2 py-1 text-sm focus:border-zinc-500 focus:outline-none'
</script>

<template>
  <div class="flex h-full flex-col">
    <!-- Header -->
    <header class="flex flex-wrap items-center gap-2 border-b border-zinc-200 bg-white px-4 py-2.5">
      <RouterLink to="/screens" class="rounded-md p-1.5 text-zinc-500 hover:bg-zinc-100" title="Back to screens">
        <ArrowLeft class="size-4" />
      </RouterLink>
      <input v-model="draft.name" :class="[input, 'w-44 font-medium']" maxlength="100" aria-label="Screen name" />
      <select v-model="draft.model_id" :class="input" aria-label="Device model">
        <option v-for="m in devices.models" :key="m.id" :value="m.id">{{ m.name }}</option>
      </select>
      <select v-if="!isImage" :value="draft.layout" :class="input" aria-label="Layout" @change="changeLayout(($event.target as HTMLSelectElement).value)">
        <option v-for="l in layouts" :key="l.key" :value="l.key">{{ l.name }}</option>
      </select>
      <label class="flex items-center gap-1.5 text-sm text-zinc-500">
        every
        <select v-model.number="draft.render_interval" :class="input">
          <option v-for="s in intervals" :key="s" :value="s">{{ duration(s) }}</option>
        </select>
      </label>
      <span v-if="dirty" class="text-xs text-amber-600">unsaved</span>
      <span v-if="message" class="text-xs" :class="message.ok ? 'text-emerald-600' : 'text-red-600'">{{ message.text }}</span>
      <div class="ml-auto flex gap-2">
        <button type="button" :class="btn" :disabled="previewing" title="Preview (Ctrl+Enter)" @click="runPreview">
          <component :is="previewing ? Loader2 : Eye" class="size-4" :class="{ 'animate-spin': previewing }" />
          Preview
        </button>
        <button
          type="button"
          class="inline-flex items-center gap-1.5 rounded-md bg-zinc-900 px-3 py-1.5 text-sm text-white hover:bg-zinc-700 disabled:opacity-50"
          :disabled="saving || !dirty"
          title="Save (Ctrl+S)"
          @click="save"
        >
          <Save class="size-4" />
          Save
        </button>
        <button type="button" :class="[btn, 'text-red-600']" title="Delete screen" @click="remove">
          <Trash2 class="size-4" />
        </button>
      </div>
    </header>

    <div v-if="screen" class="grid min-h-0 flex-1 grid-rows-2 lg:grid-cols-2 lg:grid-rows-1">
      <!-- Editor side -->
      <section class="flex min-h-0 flex-col border-zinc-200 lg:border-r">
        <div class="flex gap-1 overflow-x-auto border-b border-zinc-200 bg-zinc-50 px-2 text-sm">
          <button
            v-for="t in tabs"
            :key="t.key"
            type="button"
            class="-mb-px shrink-0 border-b-2 px-3 py-2"
            :class="tab === t.key ? 'border-zinc-900 font-medium' : 'border-transparent text-zinc-500 hover:text-zinc-900'"
            @click="tab = t.key"
          >
            {{ t.label }}
            <span v-if="errors.some((e) => e.region === t.key)" class="ml-1 inline-block size-1.5 rounded-full bg-red-500 align-middle" />
          </button>
        </div>

        <!-- Region template -->
        <template v-if="activeRegion">
          <div class="flex items-center gap-3 border-b border-zinc-100 px-3 py-1.5 text-xs text-zinc-500">
            <label class="flex items-center gap-1.5">
              <input v-model="activeRegion.transparent" type="checkbox" />
              transparent (show the background through)
            </label>
            <span class="ml-auto">{{ activeRegion.w }}×{{ activeRegion.h }} px</span>
          </div>
          <div class="min-h-0 flex-1">
            <CodeEditor
              :key="`region-${activeRegion.key}`"
              ref="editor"
              v-model="activeRegion.template"
              :functions="functions"
              :errors="regionErrors"
              @save="save"
              @preview="runPreview"
            />
          </div>
        </template>

        <div v-else-if="tab === 'data'" class="min-h-0 flex-1 overflow-y-auto p-4">
          <DataPanel
            :screen-id="screen.id"
            :region-keys="draft.regions.map((r) => r.key)"
            :data="screenData"
            @saved="loadData().then(runPreview)"
            @refresh="loadData"
          />
        </div>

        <div v-else-if="tab === 'head'" class="min-h-0 flex-1">
          <CodeEditor key="head" v-model="draft.head" :functions="functions" @save="save" @preview="runPreview" />
        </div>

        <div v-else-if="tab === 'background'" class="min-h-0 flex-1 overflow-y-auto p-4">
          <p class="mb-3 text-sm text-zinc-500">
            <template v-if="isImage">This screen shows an image (or a slideshow), prepared for the device in Go — no HTML involved.</template>
            <template v-else>Drawn behind all regions. Mark regions transparent to let it show through.</template>
          </p>
          <BackgroundPanel v-model="draft.background" :image-only="isImage" />
        </div>

        <!-- Free regions -->
        <div v-else-if="tab === 'regions'" class="min-h-0 flex-1 overflow-y-auto p-4 text-sm">
          <p class="mb-3 text-zinc-500">
            Free regions are placed by coordinates in device pixels ({{ model?.width }}×{{ model?.height }}). Higher z is drawn on top.
          </p>
          <div class="space-y-2">
            <div v-for="(r, i) in draft.regions" :key="r.id ?? `new-${i}`" class="flex flex-wrap items-center gap-2 rounded-md border border-zinc-200 bg-zinc-50 px-3 py-2">
              <input v-model="r.key" class="w-32 rounded-md border border-zinc-300 bg-white px-2 py-1 font-mono text-xs" pattern="[a-z0-9_]+" aria-label="Region key" />
              <label class="flex items-center gap-1 text-zinc-500">x<input v-model.number="r.x" type="number" min="0" :class="num" /></label>
              <label class="flex items-center gap-1 text-zinc-500">y<input v-model.number="r.y" type="number" min="0" :class="num" /></label>
              <label class="flex items-center gap-1 text-zinc-500">w<input v-model.number="r.w" type="number" min="1" :class="num" /></label>
              <label class="flex items-center gap-1 text-zinc-500">h<input v-model.number="r.h" type="number" min="1" :class="num" /></label>
              <label class="flex items-center gap-1 text-zinc-500">z<input v-model.number="r.z" type="number" class="w-14 rounded-md border border-zinc-300 bg-white px-2 py-1 text-sm" /></label>
              <button type="button" class="ml-auto rounded p-1 text-zinc-400 hover:text-red-600" title="Remove region" @click="removeRegion(i)"><X class="size-4" /></button>
            </div>
          </div>
          <button type="button" class="mt-3 inline-flex items-center gap-1.5 text-zinc-600 hover:text-zinc-900" @click="addFreeRegion">
            <Plus class="size-4" />
            Add region
          </button>
        </div>
      </section>

      <!-- Preview -->
      <section class="flex min-h-0 flex-col overflow-y-auto bg-zinc-100">
        <div class="flex flex-wrap items-center gap-2 border-b border-zinc-200 bg-white px-3 py-2 text-sm">
          <div class="flex overflow-hidden rounded-md border border-zinc-200">
            <button
              v-for="m in ['device', 'raw'] as const"
              :key="m"
              type="button"
              class="px-2.5 py-1"
              :class="previewMode === m ? 'bg-zinc-900 text-white' : 'bg-white text-zinc-600 hover:bg-zinc-50'"
              @click="previewMode = m"
            >
              {{ m === 'device' ? `Device (${model ? 2 ** model.bit_depth : '?'} grays)` : 'Browser' }}
            </button>
          </div>
          <div class="flex overflow-hidden rounded-md border border-zinc-200">
            <button
              v-for="z in ['fit', 'actual'] as const"
              :key="z"
              type="button"
              class="px-2.5 py-1"
              :class="zoom === z ? 'bg-zinc-900 text-white' : 'bg-white text-zinc-600 hover:bg-zinc-50'"
              @click="zoom = z"
            >
              {{ z === 'fit' ? 'Fit' : '1:1' }}
            </button>
          </div>
          <label v-if="!isImage" class="flex items-center gap-1.5 text-zinc-600" title="Use made-up data instead of fetching from connectors">
            <input v-model="sample" type="checkbox" @change="runPreview" />
            sample data
          </label>
          <span v-if="preview" class="ml-auto text-xs text-zinc-400">{{ preview.duration_ms }} ms · {{ model?.width }}×{{ model?.height }}</span>
        </div>

        <div class="p-4" :class="{ 'overflow-auto': zoom === 'actual' }">
          <p v-if="structuralDirty" class="mb-3 rounded-md bg-amber-50 px-3 py-2 text-xs text-amber-800">
            Layout, region and background changes show in the preview after saving.
          </p>
          <div v-if="preview" class="relative" :class="zoom === 'fit' ? 'w-full' : 'w-max'">
            <img
              :src="previewMode === 'device' ? (preview.panel ?? preview.device) : preview.raw"
              alt="Screen preview"
              class="border border-zinc-300 bg-white shadow-sm"
              :class="zoom === 'fit' ? 'w-full' : 'max-w-none [image-rendering:pixelated]'"
            />
            <!-- Outline free regions to help placing them -->
            <template v-if="isFree && model">
              <div
                v-for="r in draft.regions"
                :key="r.key"
                class="pointer-events-none absolute border-2 border-dashed"
                :class="r.key === tab ? 'border-sky-500' : 'border-sky-300/70'"
                :style="{ left: `${(r.x / model.width) * 100}%`, top: `${(r.y / model.height) * 100}%`, width: `${(r.w / model.width) * 100}%`, height: `${(r.h / model.height) * 100}%` }"
              >
                <span class="bg-sky-500 px-1 text-[10px] text-white">{{ r.key }}</span>
              </div>
            </template>
          </div>
          <div v-else class="flex aspect-[4/3] items-center justify-center rounded border border-dashed border-zinc-300 text-sm text-zinc-400">
            {{ previewing ? 'Rendering…' : isImage ? 'Choose an image and save' : 'No preview yet' }}
          </div>

          <ul v-if="preview?.data_errors" class="mt-4 space-y-1">
            <li v-for="(msg, alias) in preview.data_errors" :key="alias" class="flex items-start gap-2 rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-900">
              <CircleAlert class="mt-0.5 size-4 shrink-0" />
              <span><span class="font-mono text-xs">.Data.{{ alias }}</span> {{ msg }}</span>
            </li>
          </ul>
          <ul v-if="errors.length" class="mt-4 space-y-1">
            <li v-for="(e, i) in errors" :key="i">
              <button type="button" class="flex w-full items-start gap-2 rounded-md bg-red-50 px-3 py-2 text-left text-sm text-red-800 hover:bg-red-100" @click="reveal(e)">
                <CircleAlert class="mt-0.5 size-4 shrink-0" />
                <span><span class="font-mono text-xs">{{ e.region }}:{{ e.line }}{{ e.column ? `:${e.column}` : '' }}</span> {{ e.message }}</span>
              </button>
            </li>
          </ul>

          <div class="mt-6 rounded-lg border border-zinc-200 bg-white">
            <div class="flex items-center justify-between border-b border-zinc-100 px-3 py-2 text-sm">
              <span class="font-medium">Renders</span>
              <button type="button" :class="btn" @click="renderNow">
                <Play class="size-4" />
                Render now
              </button>
            </div>
            <ul class="max-h-56 divide-y divide-zinc-100 overflow-y-auto text-xs">
              <li v-if="!renders.length" class="px-3 py-3 text-zinc-400">
                No stored renders yet. Screens render on save, on their interval while in use, and when a device needs them.
              </li>
              <li v-for="r in renders" :key="r.id" class="flex items-center gap-2 px-3 py-1.5">
                <component :is="r.status === 'ok' ? CircleCheck : CircleAlert" class="size-3.5 shrink-0" :class="r.status === 'ok' ? 'text-emerald-600' : 'text-red-600'" />
                <span :title="dateTime(r.created_at)" class="w-24 shrink-0 text-zinc-500">{{ timeAgo(r.created_at) }}</span>
                <span class="w-16 shrink-0 text-zinc-500">{{ r.trigger }}</span>
                <span class="w-14 shrink-0 text-zinc-400">{{ r.duration_ms }} ms</span>
                <a v-if="r.filename" :href="imageUrl(r.filename)!" target="_blank" class="truncate font-mono text-zinc-500 hover:underline">{{ r.filename.slice(0, 12) }}…</a>
                <span v-else class="truncate text-red-700" :title="r.error ?? ''">{{ r.error }}</span>
              </li>
            </ul>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>
