<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { RouterLink, onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import { ArrowLeft, Blocks, CircleAlert, CircleCheck, Eye, Loader2, Play, Plus, Save, Trash2, X } from 'lucide-vue-next'
import CodeEditor from '@/components/CodeEditor.vue'
import BlockEditor from '@/components/BlockEditor.vue'
import { blocksToHTML, componentBlock, type Block } from '@/lib/blocks'
import BackgroundPanel from '@/components/BackgroundPanel.vue'
import DataPanel from '@/components/DataPanel.vue'
import { dataPaths } from '@/lib/datapaths'
import { setDataFields } from '@/lib/monaco'
import { api, type Schemas } from '@/api/client'
import { ApiError, unwrap, useDevicesStore } from '@/stores/devices'
import { useEventsStore } from '@/stores/events'
import { useScreensStore } from '@/stores/screens'
import { useConnectorsStore } from '@/stores/connectors'
import { callFor, useLibraryStore, type Component } from '@/stores/library'
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
  arrange: Arrange
  blocks: Block[] | null // null = edited as HTML
}
type Arrange = Schemas['Arrange']

// New regions only list their content; the region arranges it (see render.Arrange).
const defaultArrange = (): Arrange => ({ mode: 'stack', align: 'center', gap: 'medium', title_bar: '{{ .Screen.Name }}' })
const freeArrange = (): Arrange => ({ mode: 'stack', align: 'center', gap: 'small' })
const arrangeModes: { value: Arrange['mode']; label: string }[] = [
  { value: 'stack', label: 'Stack' },
  { value: 'row', label: 'Row' },
  { value: 'grid', label: 'Grid' },
  { value: 'columns', label: 'Columns' },
  { value: '', label: 'Custom HTML layout' },
]

const route = useRoute()
const router = useRouter()
const devices = useDevicesStore()
const screensStore = useScreensStore()
const library = useLibraryStore()
const connectors = useConnectorsStore()
const id = computed(() => Number(route.params.id))

const screen = ref<Schemas['Screen'] | null>(null)
const functions = ref<string[]>([])
const layouts = ref<Layout[]>([])
const languages = ref<Schemas['SystemInfo']['languages']>([])
const draft = reactive({
  name: '',
  language: '',
  rotation: 0 as Schemas['Rotation'],
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
// The screen as designed: portrait rotations swap the panel's width and height.
const canvas = computed(() => {
  const m = model.value
  if (!m) return undefined
  const portrait = draft.rotation === 90 || draft.rotation === 270
  return { width: portrait ? m.height : m.width, height: portrait ? m.width : m.height }
})
const rotations: { value: Schemas['Rotation']; label: string }[] = [
  { value: 0, label: 'Landscape' },
  { value: 90, label: 'Portrait (turned left)' },
  { value: 270, label: 'Portrait (turned right)' },
  { value: 180, label: 'Landscape (upside down)' },
]

// Rotating between landscape and portrait re-fits preset layouts and moves free regions inside the new canvas.
function changeRotation(value: Schemas['Rotation']) {
  const wasPortrait = draft.rotation === 90 || draft.rotation === 270
  draft.rotation = value
  const isPortrait = value === 90 || value === 270
  if (wasPortrait === isPortrait || !canvas.value) return
  if (draft.layout === 'free') {
    for (const r of draft.regions) {
      r.w = Math.min(r.w, canvas.value.width)
      r.h = Math.min(r.h, canvas.value.height)
      r.x = Math.min(r.x, canvas.value.width - r.w)
      r.y = Math.min(r.y, canvas.value.height - r.h)
    }
  } else {
    changeLayout(draft.layout)
  }
}
const isFree = computed(() => draft.layout === 'free')
const activeRegion = computed(() => draft.regions.find((r) => r.key === tab.value))

const slotTemplate = `<span class="value value--large">{{ .Region.Key }}</span>\n`
const freeTemplate = `<span class="value value--large value--tnums">{{ date "15:04" .Now }}</span>\n`

function fromScreen(s: Schemas['Screen']): Draft {
  return {
    name: s.name,
    language: s.language,
    rotation: s.rotation,
    model_id: s.model_id,
    render_interval: s.render_interval,
    head: s.head,
    layout: s.layout,
    background: { ...s.background, asset_ids: [...(s.background.asset_ids ?? [])] },
    regions: s.regions.map((r) => ({
      id: r.id, key: r.key, template: r.template, transparent: r.transparent, x: r.x, y: r.y, w: r.w, h: r.h, z: r.z,
      arrange: { ...r.arrange },
      blocks: r.blocks ? JSON.parse(JSON.stringify(r.blocks)) : null,
    })),
  }
}
const snapshot = (d: Draft) => JSON.stringify(d)
const dirty = computed(() => !!screen.value && snapshot(draft) !== snapshot(fromScreen(screen.value)))
// Layout, region set and background only show in previews after saving.
const structuralDirty = computed(() => {
  const s = screen.value
  if (!s) return false
  const shape = (d: Draft) =>
    JSON.stringify([d.layout, d.rotation, d.background, d.model_id, d.regions.map((r) => [r.key, r.transparent, r.x, r.y, r.w, r.h, r.z])])
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
      library.ensure(),
    ])
    const system = unwrap(sys)
    functions.value = system.template_functions
    layouts.value = system.layouts
    languages.value = system.languages
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
  if (!layout || !canvas.value) return
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
      r = reused ? { ...reused, key: s.key } : { key: s.key, template: slotTemplate, transparent: false, x: 0, y: 0, w: 0, h: 0, z: 0, arrange: defaultArrange(), blocks: null }
    }
    return {
      ...r,
      x: Math.round(s.x * canvas.value!.width),
      y: Math.round(s.y * canvas.value!.height),
      w: Math.round(s.w * canvas.value!.width),
      h: Math.round(s.h * canvas.value!.height),
      z: 0,
    }
  })
  draft.layout = key
  if (!draft.regions.some((r) => r.key === tab.value)) tab.value = draft.regions[0]?.key ?? 'head'
}

function addFreeRegion() {
  const m = canvas.value
  if (!m) return
  let n = draft.regions.length + 1
  while (draft.regions.some((r) => r.key === `region_${n}`)) n++
  const w = Math.round(m.width / 3)
  const h = Math.round(m.height / 4)
  draft.regions.push({ key: `region_${n}`, template: freeTemplate, transparent: true, x: Math.round(m.width / 20), y: Math.round(m.height / 20), w, h, z: draft.regions.length, arrange: freeArrange(), blocks: null })
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
    const arrange = Object.fromEntries(draft.regions.map((r) => [r.key, r.arrange]))
    const blocks = Object.fromEntries(draft.regions.filter((r) => r.blocks).map((r) => [r.key, r.blocks!]))
    const res = unwrap(
      await api.POST('/screens/{id}/preview', { params: { path: { id: id.value } }, body: { templates, arrange, blocks, head: draft.head, sample: sample.value } }),
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
        language: draft.language,
        rotation: draft.rotation,
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

// Insert a component call at the cursor, wired to a binding of the connector type it expects.
const picker = ref(false)
async function insertComponent(c: Component) {
  picker.value = false
  const region = activeRegion.value
  if (!region) return
  try {
    const paths = await dataArgs(c, region.key)
    if (region.blocks) region.blocks.push(componentBlock(c, paths))
    else editor.value?.insert(callFor(c, paths) + '\n')
  } catch (e) {
    message.value = { ok: false, text: (e as Error).message }
  }
}

// dataArgs picks, for each data input of a component, a binding of the expected connector type
// visible to the region, and reports the inputs it couldn't fill.
async function dataArgs(c: Component, regionKey: string): Promise<Record<string, string>> {
  const [bindings] = await Promise.all([
    api.GET('/screens/{id}/bindings', { params: { path: { id: id.value } } }).then(unwrap),
    connectors.loaded ? Promise.resolve() : connectors.load(),
  ])
  const visible = bindings.filter((b) => b.enabled && (!b.region_key || b.region_key === regionKey))
  const args: Record<string, string> = {}
  const missing: string[] = []
  for (const i of c.inputs.filter((i) => i.kind === 'data')) {
    const b = visible.find((b) => !i.connector || connectors.byId(b.connector_id)?.type === i.connector)
    if (!b) {
      missing.push(connectors.typeOf(i.connector)?.name ?? 'data')
      continue
    }
    args[i.name] = `.Data.${b.alias}` + itemPath(i.sample, screenData.value[b.alias]?.data)
  }
  message.value = missing.length
    ? { ok: false, text: `No ${missing.join(', ')} binding for this region yet — add one in the Data tab and fix the .Data path` }
    : null
  return args
}

// Blocks: add, switch modes, preview while editing.
async function addBlock(c: Component | null) {
  const region = activeRegion.value
  if (!region?.blocks) return
  if (!c) {
    region.blocks.push({ html: '' })
    return
  }
  try {
    region.blocks.push(componentBlock(c, await dataArgs(c, region.key)))
  } catch (e) {
    message.value = { ok: false, text: (e as Error).message }
  }
}

function useBlocks() {
  const r = activeRegion.value
  if (!r || r.blocks) return
  // The current HTML becomes the first block, so nothing is lost.
  r.blocks = r.template.trim() ? [{ html: r.template.trim() }] : []
  if (!r.arrange.mode) Object.assign(r.arrange, isFree.value ? freeArrange() : defaultArrange())
}

function useHTML() {
  const r = activeRegion.value
  if (!r?.blocks) return
  r.template = blocksToHTML(r.blocks, library.components)
  r.blocks = null
}

const paths = computed(() => dataPaths(screenData.value).map((f) => f.label))
const highlightBlock = ref<number | null>(null)
let blockTimer: ReturnType<typeof setTimeout> | undefined
watch(
  () => JSON.stringify(draft.regions.map((r) => r.blocks)),
  (_now, before) => {
    if (!screen.value || before === undefined) return
    clearTimeout(blockTimer)
    blockTimer = setTimeout(() => void runPreview(), 700)
  },
)

// An input whose sample looks like one entry of a keyed binding (a Home Assistant entity, say)
// gets the binding's first entry: ".sensor_outdoor_temperature".
function itemPath(sample: unknown, data: unknown): string {
  if (!isObject(sample) || !isObject(data)) return ''
  const first = Object.entries(data).find(([, v]) => isObject(v))
  if (!first) return ''
  const shared = Object.keys(sample).filter((k) => k in (first[1] as object)).length
  return shared >= 2 && /^[A-Za-z_]\w*$/.test(first[0]) ? `.${first[0]}` : ''
}
const isObject = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v)

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
// The region arranges its content, so a template with its own layout would nest layouts.
const ownLayout = computed(() => {
  const r = activeRegion.value
  const html = r?.blocks ? r.blocks.map((b) => b.html ?? '').join('\n') : (r?.template ?? '')
  return !!r?.arrange.mode && /class="[^"]*\b(layout|title_bar)\b/.test(html)
})

function arrangeChanged() {
  const a = activeRegion.value?.arrange
  if (!a) return
  a.align ||= 'center'
  a.gap ||= 'medium'
  a.cols ??= 0
  void runPreview()
}

const regionErrors = computed(() => errors.value.filter((e) => e.region === tab.value))

const intervals = [60, 300, 600, 900, 1800, 3600, 3 * 3600, 6 * 3600, 12 * 3600, 86400]
const btn = 'inline-flex items-center gap-1.5 rounded-md border border-zinc-200 bg-white px-3 py-1.5 text-sm hover:bg-zinc-50 disabled:opacity-50'
const input = 'rounded-md border border-zinc-300 bg-white px-2.5 py-1.5 text-sm focus:border-zinc-500 focus:outline-none'
const mini = 'rounded border border-zinc-300 bg-white px-1.5 py-0.5 text-xs focus:border-zinc-500 focus:outline-none'
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
      <select :value="draft.rotation" :class="input" aria-label="Orientation" title="How the device hangs" @change="changeRotation(Number(($event.target as HTMLSelectElement).value) as Schemas['Rotation'])">
        <option v-for="r in rotations" :key="r.value" :value="r.value">{{ r.label }}</option>
      </select>
      <select v-if="!isImage" v-model="draft.language" :class="input" aria-label="Language" title="Language of dates and labels">
        <option value="">Default language</option>
        <option v-for="l in languages" :key="l.code" :value="l.code">{{ l.name }}</option>
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
          <div class="flex flex-wrap items-center gap-2 border-b border-zinc-100 px-3 py-1.5 text-xs text-zinc-500">
            <div class="flex overflow-hidden rounded border border-zinc-300">
              <button
                type="button"
                class="px-2 py-0.5"
                :class="activeRegion.blocks ? 'bg-zinc-900 text-white' : 'bg-white hover:bg-zinc-50'"
                title="Edit with blocks: pick components and fill in forms"
                @click="useBlocks"
              >
                Blocks
              </button>
              <button
                type="button"
                class="border-l border-zinc-300 px-2 py-0.5"
                :class="activeRegion.blocks ? 'bg-white hover:bg-zinc-50' : 'bg-zinc-900 text-white'"
                title="Edit the template as HTML"
                @click="useHTML"
              >
                HTML
              </button>
            </div>
            <span>Arrange</span>
            <select v-model="activeRegion.arrange.mode" :class="mini" aria-label="Arrangement" @change="arrangeChanged">
              <option v-for="m in arrangeModes" :key="m.value" :value="m.value">{{ m.label }}</option>
            </select>
            <template v-if="activeRegion.arrange.mode">
              <select v-if="activeRegion.arrange.mode === 'grid'" v-model.number="activeRegion.arrange.cols" :class="mini" aria-label="Grid columns" @change="runPreview">
                <option :value="0">auto columns</option>
                <option v-for="n in 6" :key="n" :value="n">{{ n }} column{{ n > 1 ? 's' : '' }}</option>
              </select>
              <select v-if="activeRegion.arrange.mode !== 'columns'" v-model="activeRegion.arrange.align" :class="mini" aria-label="Vertical alignment" @change="runPreview">
                <option value="top">top</option>
                <option value="center">center</option>
                <option value="bottom">bottom</option>
              </select>
              <select v-model="activeRegion.arrange.gap" :class="mini" aria-label="Spacing" @change="runPreview">
                <option value="none">no gap</option>
                <option value="small">small gap</option>
                <option value="medium">medium gap</option>
                <option value="large">large gap</option>
                <option value="xlarge">extra large gap</option>
              </select>
              <label v-if="!isFree" class="flex min-w-48 flex-1 items-center gap-1.5">
                title bar
                <input v-model="activeRegion.arrange.title_bar" :class="[mini, 'flex-1 font-mono']" placeholder="none" @change="runPreview" />
              </label>
            </template>
          </div>
          <p v-if="ownLayout" class="border-b border-amber-100 bg-amber-50 px-3 py-1.5 text-xs text-amber-800">
            This template still has its own <code>layout</code> or <code>title_bar</code>. With an arrangement, list only the content —
            the region adds the layout and title bar.
          </p>
          <div class="flex items-center gap-3 border-b border-zinc-100 px-3 py-1.5 text-xs text-zinc-500">
            <label class="flex items-center gap-1.5">
              <input v-model="activeRegion.transparent" type="checkbox" />
              transparent (show the background through)
            </label>
            <div class="relative ml-auto">
              <button type="button" class="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900" @click="picker = !picker">
                <Blocks class="size-3.5" />
                Insert component
              </button>
              <div v-if="picker" class="absolute right-0 z-20 mt-1 max-h-96 w-80 overflow-y-auto rounded-md border border-zinc-200 bg-white py-1 shadow-lg">
                <button
                  v-for="c in library.components"
                  :key="c.id"
                  type="button"
                  class="block w-full px-3 py-1.5 text-left hover:bg-zinc-50"
                  @click="insertComponent(c)"
                >
                  <span class="text-sm text-zinc-900">{{ c.name }}</span>
                  <span class="ml-1 font-mono text-[10px] text-zinc-400">{{ c.key }}</span>
                  <span class="block text-xs text-zinc-500">{{ c.description }}</span>
                </button>
                <RouterLink to="/library" class="block border-t border-zinc-100 px-3 py-1.5 text-xs text-zinc-500 hover:text-zinc-900">Manage components…</RouterLink>
              </div>
            </div>
            <span>{{ activeRegion.w }}×{{ activeRegion.h }} px</span>
          </div>
          <div v-if="activeRegion.blocks" class="min-h-0 flex-1 overflow-y-auto bg-zinc-50">
            <BlockEditor
              v-model="activeRegion.blocks"
              :components="library.components"
              :data-paths="paths"
              :highlight="highlightBlock"
              @add="addBlock"
            />
          </div>
          <div v-else class="min-h-0 flex-1">
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
            Free regions are placed by coordinates in device pixels ({{ canvas?.width }}×{{ canvas?.height }}). Higher z is drawn on top.
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
          <span v-if="preview" class="ml-auto text-xs text-zinc-400">{{ preview.duration_ms }} ms · {{ canvas?.width }}×{{ canvas?.height }}</span>
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
            <template v-if="isFree && canvas">
              <div
                v-for="r in draft.regions"
                :key="r.key"
                class="pointer-events-none absolute border-2 border-dashed"
                :class="r.key === tab ? 'border-sky-500' : 'border-sky-300/70'"
                :style="{ left: `${(r.x / canvas.width) * 100}%`, top: `${(r.y / canvas.height) * 100}%`, width: `${(r.w / canvas.width) * 100}%`, height: `${(r.h / canvas.height) * 100}%` }"
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
