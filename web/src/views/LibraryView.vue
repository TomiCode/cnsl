<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { RouterLink, onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import { Blocks, CircleAlert, Copy, Eye, Loader2, Lock, Plus, Save, Scissors, Search, Trash2, X } from 'lucide-vue-next'
import CodeEditor from '@/components/CodeEditor.vue'
import { api, type Schemas } from '@/api/client'
import { ApiError, unwrap, useDevicesStore } from '@/stores/devices'
import { useConnectorsStore } from '@/stores/connectors'
import { useScreensStore } from '@/stores/screens'
import { useLibraryStore, type Component, type ComponentParam, type Snippet } from '@/stores/library'
import type { TemplateError } from '@/lib/monaco'

const route = useRoute()
const router = useRouter()
const library = useLibraryStore()
const devices = useDevicesStore()
const connectors = useConnectorsStore()
const screens = useScreensStore()

type Tab = 'components' | 'snippets' | 'icons'
const tab = computed<Tab>(() => (route.query.tab === 'snippets' || route.query.tab === 'icons' ? route.query.tab : 'components'))
const languages = ref<Schemas['SystemInfo']['languages']>([])
const functions = ref<string[]>([])
const error = ref<string | null>(null)
const message = ref<string | null>(null)

onMounted(async () => {
  try {
    const [sys] = await Promise.all([
      api.GET('/system'),
      library.load(),
      devices.loaded ? Promise.resolve() : devices.load(),
      connectors.loaded ? Promise.resolve() : connectors.load(),
      screens.loaded ? Promise.resolve() : screens.load(),
    ])
    functions.value = unwrap(sys).template_functions
    languages.value = unwrap(sys).languages
    modelId.value = devices.models[0]?.id ?? 0
    const first = library.components[0]
    if (first && tab.value === 'components') select(first)
  } catch (e) {
    error.value = (e as Error).message
  }
})

// ---- Components ----

interface Draft {
  key: string
  name: string
  description: string
  source: string
  inputs: ComponentParam[]
}
const selected = ref<Component | 'new' | null>(null)
const draft = reactive<Draft>({ key: '', name: '', description: '', source: '', inputs: [] })
const saved = ref('')
const errors = ref<TemplateError[]>([])
const saving = ref(false)
const readOnly = computed(() => selected.value !== 'new' && !!selected.value?.builtin)
const dirty = computed(() => !!selected.value && !readOnly.value && JSON.stringify(draft) !== saved.value)

// Plain deep copy; structuredClone rejects Vue's reactive proxies.
const clone = <T,>(v: T): T => JSON.parse(JSON.stringify(v))

const builtins = computed(() => library.components.filter((c) => c.builtin))
const mine = computed(() => library.components.filter((c) => !c.builtin))

function load(d: Draft) {
  Object.assign(draft, clone(d))
  saved.value = JSON.stringify(draft)
  errors.value = []
  message.value = null
}

function confirmDiscard() {
  return !dirty.value || confirm('Discard your changes to this component?')
}

function select(c: Component) {
  if (!confirmDiscard()) return
  selected.value = c
  load({ key: c.key, name: c.name, description: c.description, source: c.source, inputs: c.inputs })
  void runPreview()
}

function startNew() {
  if (!confirmDiscard()) return
  selected.value = 'new'
  load({
    key: 'my-component',
    name: 'My component',
    description: '',
    source: '{{- with .data }}\n<div class="item">\n  <div class="content">\n    <span class="value value--tnums">{{ . }}</span>\n    <span class="label">{{ $.label }}</span>\n  </div>\n</div>\n{{- end }}\n',
    inputs: [
      { name: 'data', kind: 'data', label: 'Data' },
      { name: 'label', kind: 'string', label: 'Label', default: 'Value' },
    ],
  })
  preview.value = null
}

const body = () => ({ ...clone(draft), inputs: draft.inputs.map(cleanInput) })

// Drops empty optional fields so they are not stored as "".
function cleanInput(i: ComponentParam): ComponentParam {
  const out: ComponentParam = { name: i.name.trim(), kind: i.kind }
  for (const k of ['label', 'help', 'connector'] as const) if (i[k]) out[k] = i[k]
  if (i.kind !== 'data' && i.default !== undefined && i.default !== '') out.default = i.default
  if (i.sample !== undefined) out.sample = i.sample
  return out
}

function problemText(p: Schemas['TemplateProblem']) {
  if (p.template_errors?.length) {
    errors.value = p.template_errors
    return `${p.template_errors.length} template error(s)`
  }
  return p.detail ?? p.title
}

async function save() {
  if (!selected.value || readOnly.value || saving.value) return
  saving.value = true
  error.value = null
  try {
    const res = selected.value === 'new' ? await library.createComponent(body()) : await library.updateComponent(selected.value.id, body())
    if (res.error) throw new ApiError(`Not saved: ${problemText(res.error as Schemas['TemplateProblem'])}`)
    const c = res.data!
    selected.value = c
    load({ key: c.key, name: c.name, description: c.description, source: c.source, inputs: c.inputs })
    message.value = c.used_by.length ? `Saved — re-rendering ${c.used_by.length} screen(s)` : 'Saved'
    void runPreview()
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    saving.value = false
  }
}

async function duplicate() {
  if (!selected.value || selected.value === 'new' || !confirmDiscard()) return
  try {
    const c = await library.duplicateComponent(selected.value.id)
    saved.value = JSON.stringify(draft) // skip the discard prompt
    select(c)
  } catch (e) {
    error.value = (e as Error).message
  }
}

async function removeComponent() {
  const c = selected.value
  if (!c || c === 'new' || !confirm(`Delete component "${c.name}"?`)) return
  try {
    await library.removeComponent(c.id)
    selected.value = null
  } catch (e) {
    error.value = (e as Error).message
  }
}

function addInput() {
  let n = draft.inputs.length + 1
  while (draft.inputs.some((i) => i.name === `input${n}`)) n++
  draft.inputs.push({ name: `input${n}`, kind: 'string' })
}

function setKind(i: ComponentParam, kind: ComponentParam['kind']) {
  i.kind = kind
  i.default = kind === 'bool' ? false : undefined
  if (kind !== 'data') delete i.connector
}

const usedBy = computed(() => {
  const c = selected.value
  if (!c || c === 'new') return []
  return c.used_by.map((id) => ({ id, name: screens.screens.find((s) => s.id === id)?.name ?? `Screen ${id}` }))
})

// Example calls (built here: "}}" can't appear inside a template interpolation).
const usage = computed(() => {
  const first = draft.inputs[0]
  const arg = first ? (first.kind === 'data' ? ' .Data.alias' : ' …') : ''
  return {
    short: `{{ component "${draft.key}"${arg} }}`,
    named: `{{ component "${draft.key}" ${draft.inputs.map((i) => `"${i.name}" …`).join(' ')} }}`,
  }
})

// Preview
const modelId = ref(0)
const view = ref<'full' | 'half_vertical' | 'half_horizontal' | 'quadrant'>('full')
const previewLang = ref('') // '' = the global setting
const preview = ref<Schemas['Preview'] | null>(null)
const previewing = ref(false)
watch([modelId, view, previewLang], () => void runPreview())

async function runPreview() {
  if (!selected.value || !modelId.value || previewing.value) return
  previewing.value = true
  error.value = null
  try {
    const res = await api.POST('/components/preview', { body: { model_id: modelId.value, view: view.value, component: body(), ...(previewLang.value ? { language: previewLang.value } : {}) } })
    if (res.error) {
      if (res.response.status === 422) throw new ApiError(problemText(res.error as Schemas['TemplateProblem']))
      unwrap(res)
    }
    preview.value = res.data!
    errors.value = res.data!.errors.map(sourceError)
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    previewing.value = false
  }
}

// Preview errors are positioned in the wrapper call; the component's own position is inside the
// message ("… template: key:3:5: msg").
function sourceError(e: TemplateError): TemplateError {
  const m = /template: [\w-]+:(\d+):(\d+): (.*)$/.exec(e.message)
  return m ? { ...e, line: Number(m[1]), column: Number(m[2]), message: m[3]! } : { ...e, line: 0, column: 0 }
}

// ---- Snippets ----

const snippet = ref<Snippet | 'new' | null>(null)
const sdraft = reactive({ prefix: '', name: '', description: '', body: '' })

function selectSnippet(s: Snippet) {
  snippet.value = s
  Object.assign(sdraft, { prefix: s.prefix, name: s.name, description: s.description, body: s.body })
}

function newSnippet() {
  snippet.value = 'new'
  Object.assign(sdraft, { prefix: 'my_snippet', name: 'My snippet', description: '', body: '<span class="label">$1</span>' })
}

async function saveSnippet() {
  const s = snippet.value
  if (!s || (s !== 'new' && s.builtin)) return
  error.value = null
  try {
    selectSnippet(await library.saveSnippet(s === 'new' ? null : s.id, { ...sdraft }))
  } catch (e) {
    error.value = (e as Error).message
  }
}

async function removeSnippet() {
  const s = snippet.value
  if (!s || s === 'new' || !confirm(`Delete snippet "${s.prefix}"?`)) return
  try {
    await library.removeSnippet(s.id)
    snippet.value = null
  } catch (e) {
    error.value = (e as Error).message
  }
}

// ---- Icons ----

const iconQuery = ref('')
const iconSet = ref<'' | 'wi' | 'lucide'>('')
const iconSize = ref(32)
const copied = ref('')
const maxIcons = 360
const iconMatches = computed(() => {
  const q = iconQuery.value.trim().toLowerCase()
  return library.icons.filter(
    (i) => (!iconSet.value || i.set === iconSet.value) && (!q || i.name.includes(q) || i.tags?.some((t) => t.includes(q))),
  )
})

function iconCall(name: string) {
  return `{{ icon "${name}" ${iconSize.value} }}`
}

async function copyIcon(name: string) {
  try {
    await navigator.clipboard.writeText(iconCall(name))
    copied.value = name
  } catch {
    copied.value = ''
    error.value = 'Copy failed — select the call below the grid instead'
  }
}

function setTab(t: Tab) {
  if (t !== tab.value && confirmDiscard()) void router.replace({ query: t === 'components' ? {} : { tab: t } })
}

onBeforeRouteLeave(() => confirmDiscard())

const btn = 'inline-flex items-center gap-1.5 rounded-md border border-zinc-200 bg-white px-3 py-1.5 text-sm hover:bg-zinc-50 disabled:opacity-50'
const primary = 'inline-flex items-center gap-1.5 rounded-md bg-zinc-900 px-3 py-1.5 text-sm text-white hover:bg-zinc-700 disabled:opacity-50'
const input = 'w-full rounded-md border border-zinc-300 bg-white px-2.5 py-1.5 text-sm focus:border-zinc-500 focus:outline-none disabled:bg-zinc-50 disabled:text-zinc-500'
const cell = 'w-full rounded border border-zinc-300 bg-white px-1.5 py-1 text-xs focus:border-zinc-500 focus:outline-none disabled:bg-zinc-50'
const listItem = (active: boolean) =>
  ['flex w-full items-center gap-2 rounded-md px-3 py-1.5 text-left text-sm', active ? 'bg-zinc-900 text-white' : 'text-zinc-700 hover:bg-zinc-100']
</script>

<template>
  <div class="flex h-full flex-col">
    <header class="flex flex-wrap items-center gap-3 border-b border-zinc-200 bg-white px-4 py-2.5">
      <h1 class="font-semibold">Library</h1>
      <div class="flex overflow-hidden rounded-md border border-zinc-200 text-sm">
        <button
          v-for="t in ['components', 'snippets', 'icons'] as const"
          :key="t"
          type="button"
          class="px-3 py-1 capitalize"
          :class="tab === t ? 'bg-zinc-900 text-white' : 'bg-white text-zinc-600 hover:bg-zinc-50'"
          @click="setTab(t)"
        >
          {{ t }}
        </button>
      </div>
      <span v-if="dirty" class="text-xs text-amber-600">unsaved</span>
      <span v-if="message" class="text-xs text-emerald-600">{{ message }}</span>
      <span v-if="error" class="text-xs text-red-600">{{ error }}</span>
    </header>

    <!-- Components -->
    <div v-if="tab === 'components'" class="grid min-h-0 flex-1 grid-cols-[14rem_1fr]">
      <aside class="overflow-y-auto border-r border-zinc-200 bg-white p-2">
        <button type="button" :class="[btn, 'mb-3 w-full justify-center']" @click="startNew"><Plus class="size-4" />New component</button>
        <div class="px-3 pb-1 text-xs font-medium uppercase tracking-wide text-zinc-400">Yours</div>
        <p v-if="!mine.length" class="px-3 pb-2 text-xs text-zinc-400">None yet — duplicate a built-in to start.</p>
        <button v-for="c in mine" :key="c.id" type="button" :class="listItem(selected !== 'new' && selected?.id === c.id)" @click="select(c)">
          <span class="truncate">{{ c.name }}</span>
        </button>
        <div class="mt-3 px-3 pb-1 text-xs font-medium uppercase tracking-wide text-zinc-400">Built-in</div>
        <button v-for="c in builtins" :key="c.id" type="button" :class="listItem(selected !== 'new' && selected?.id === c.id)" @click="select(c)">
          <span class="truncate">{{ c.name }}</span>
        </button>
      </aside>

      <div v-if="!selected" class="flex flex-col items-center justify-center gap-2 text-sm text-zinc-400">
        <Blocks class="size-8 text-zinc-300" />
        Components are reusable template parts. Pick one to see it.
      </div>

      <div v-else class="grid min-h-0 grid-rows-[auto_1fr] lg:grid-cols-2 lg:grid-rows-1">
        <!-- Editor -->
        <section class="flex min-h-0 flex-col overflow-y-auto border-zinc-200 lg:border-r">
          <div class="space-y-3 border-b border-zinc-200 bg-white p-4">
            <p v-if="readOnly" class="flex items-center gap-2 rounded-md bg-zinc-100 px-3 py-2 text-xs text-zinc-600">
              <Lock class="size-3.5" />
              Built-in components are read-only and refreshed by updates. Duplicate one to change it.
            </p>
            <div class="grid gap-3 sm:grid-cols-[1fr_12rem]">
              <label class="block text-sm">
                <span class="mb-1 block text-zinc-600">Name</span>
                <input v-model="draft.name" :class="input" :disabled="readOnly" maxlength="100" />
              </label>
              <label class="block text-sm">
                <span class="mb-1 block text-zinc-600">Key</span>
                <input v-model="draft.key" :class="[input, 'font-mono']" :disabled="readOnly" pattern="[a-z][a-z0-9-]*" maxlength="60" />
              </label>
            </div>
            <label class="block text-sm">
              <span class="mb-1 block text-zinc-600">Description</span>
              <input v-model="draft.description" :class="input" :disabled="readOnly" maxlength="500" />
            </label>

            <div>
              <div class="mb-1 flex items-center text-sm text-zinc-600">
                Inputs
                <span class="ml-2 text-xs text-zinc-400">read as <code>.name</code>; the first can be passed without its name</span>
              </div>
              <table class="w-full text-xs">
                <thead class="text-left text-zinc-400">
                  <tr>
                    <th class="pb-1 font-normal">Name</th>
                    <th class="pb-1 font-normal">Kind</th>
                    <th class="pb-1 font-normal">Connector / default</th>
                    <th class="pb-1 font-normal">Help</th>
                    <th />
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(i, n) in draft.inputs" :key="n" class="align-top">
                    <td class="py-0.5 pr-1"><input v-model="i.name" :class="[cell, 'font-mono']" :disabled="readOnly" /></td>
                    <td class="w-24 py-0.5 pr-1">
                      <select :value="i.kind" :class="cell" :disabled="readOnly" @change="setKind(i, ($event.target as HTMLSelectElement).value as ComponentParam['kind'])">
                        <option v-for="k in ['data', 'string', 'number', 'bool']" :key="k" :value="k">{{ k }}</option>
                      </select>
                    </td>
                    <td class="py-0.5 pr-1">
                      <select v-if="i.kind === 'data'" v-model="i.connector" :class="cell" :disabled="readOnly">
                        <option :value="undefined">any</option>
                        <option v-for="t in connectors.types" :key="t.key" :value="t.key">{{ t.name }}</option>
                      </select>
                      <input v-else-if="i.kind === 'bool'" v-model="i.default" type="checkbox" :disabled="readOnly" class="mt-1.5" />
                      <input v-else-if="i.kind === 'number'" v-model.number="i.default" type="number" :class="cell" :disabled="readOnly" />
                      <input v-else v-model="i.default" :class="cell" :disabled="readOnly" />
                    </td>
                    <td class="py-0.5 pr-1"><input v-model="i.help" :class="cell" :disabled="readOnly" /></td>
                    <td class="w-6 py-0.5">
                      <button v-if="!readOnly" type="button" class="rounded p-1 text-zinc-400 hover:text-red-600" title="Remove input" @click="draft.inputs.splice(n, 1)">
                        <X class="size-3.5" />
                      </button>
                    </td>
                  </tr>
                </tbody>
              </table>
              <button v-if="!readOnly" type="button" class="mt-1 inline-flex items-center gap-1 text-xs text-zinc-600 hover:text-zinc-900" @click="addInput">
                <Plus class="size-3.5" />
                Add input
              </button>
            </div>

            <div class="flex flex-wrap items-center gap-2">
              <button v-if="!readOnly" type="button" :class="primary" :disabled="saving || !dirty" title="Save (Ctrl+S)" @click="save">
                <Save class="size-4" />
                {{ selected === 'new' ? 'Create' : 'Save' }}
              </button>
              <button v-if="selected !== 'new'" type="button" :class="btn" @click="duplicate"><Copy class="size-4" />Duplicate</button>
              <button v-if="selected !== 'new' && !readOnly" type="button" :class="[btn, 'text-red-600']" title="Delete component" @click="removeComponent">
                <Trash2 class="size-4" />
              </button>
              <span v-if="usedBy.length" class="ml-auto text-xs text-zinc-500">
                Used by
                <template v-for="(s, n) in usedBy" :key="s.id">
                  <RouterLink :to="`/screens/${s.id}`" class="underline hover:text-zinc-900">{{ s.name }}</RouterLink>{{ n < usedBy.length - 1 ? ', ' : '' }}
                </template>
              </span>
            </div>
          </div>
          <div class="min-h-80 flex-1">
            <CodeEditor
              :key="selected === 'new' ? 'new' : selected.id"
              v-model="draft.source"
              :functions="functions"
              :errors="errors.filter((e) => e.line > 0)"
              :read-only="readOnly"
              @save="save"
              @preview="runPreview"
            />
          </div>
        </section>

        <!-- Preview -->
        <section class="flex min-h-0 flex-col overflow-y-auto bg-zinc-100">
          <div class="flex flex-wrap items-center gap-2 border-b border-zinc-200 bg-white px-3 py-2 text-sm">
            <select v-model.number="modelId" :class="[input, 'w-auto']" aria-label="Device model">
              <option v-for="m in devices.models" :key="m.id" :value="m.id">{{ m.name }}</option>
            </select>
            <select v-model="previewLang" :class="[input, 'w-auto']" aria-label="Preview language">
              <option value="">Default language</option>
              <option v-for="l in languages" :key="l.code" :value="l.code">{{ l.name }}</option>
            </select>
            <select v-model="view" :class="[input, 'w-auto']" aria-label="View size">
              <option value="full">Full screen</option>
              <option value="half_vertical">Half (left)</option>
              <option value="half_horizontal">Half (top)</option>
              <option value="quadrant">Quadrant</option>
            </select>
            <button type="button" :class="btn" :disabled="previewing" title="Preview (Ctrl+Enter)" @click="runPreview">
              <component :is="previewing ? Loader2 : Eye" class="size-4" :class="{ 'animate-spin': previewing }" />
              Preview
            </button>
            <span v-if="preview" class="ml-auto text-xs text-zinc-400">{{ preview.duration_ms }} ms · sample data</span>
          </div>
          <div class="p-4">
            <img v-if="preview" :src="preview.panel ?? preview.device" alt="Component preview" class="w-full border border-zinc-300 bg-white shadow-sm" />
            <div v-else class="flex aspect-[4/3] items-center justify-center rounded border border-dashed border-zinc-300 text-sm text-zinc-400">
              {{ previewing ? 'Rendering…' : 'No preview yet' }}
            </div>
            <ul v-if="errors.length" class="mt-4 space-y-1">
              <li v-for="(e, n) in errors" :key="n" class="flex items-start gap-2 rounded-md bg-red-50 px-3 py-2 text-sm text-red-800">
                <CircleAlert class="mt-0.5 size-4 shrink-0" />
                <span>{{ e.line ? `${e.line}:${e.column} ` : '' }}{{ e.message }}</span>
              </li>
            </ul>
            <div class="mt-4 rounded-lg border border-zinc-200 bg-white p-3 text-xs text-zinc-600">
              <div class="mb-1 font-medium text-zinc-800">Use it in a region</div>
              <code class="block rounded bg-zinc-50 px-2 py-1.5 font-mono" v-text="usage.short" />
              <p v-if="draft.inputs.length > 1" class="mt-1.5">
                or with names:
                <code class="font-mono" v-text="usage.named" />
              </p>
              <p class="mt-1.5">The preview uses each connector's sample data. Inside a component, <code>.Now</code>, <code>.Screen</code>, <code>.Region</code> and <code>.Data</code> are the calling region's.</p>
            </div>
          </div>
        </section>
      </div>
    </div>

    <!-- Icons -->
    <div v-else-if="tab === 'icons'" class="min-h-0 flex-1 overflow-y-auto p-6">
      <div class="mb-4 flex flex-wrap items-center gap-3">
        <label class="relative">
          <Search class="pointer-events-none absolute top-2 left-2.5 size-4 text-zinc-400" />
          <input v-model="iconQuery" :class="[input, 'w-72 pl-8']" placeholder="Search names and tags (sun, battery, wifi…)" />
        </label>
        <div class="flex overflow-hidden rounded-md border border-zinc-200 text-sm">
          <button
            v-for="s in [['', 'All'], ['wi', 'Weather'], ['lucide', 'Outline']] as const"
            :key="s[0]"
            type="button"
            class="px-3 py-1"
            :class="iconSet === s[0] ? 'bg-zinc-900 text-white' : 'bg-white text-zinc-600 hover:bg-zinc-50'"
            @click="iconSet = s[0]"
          >
            {{ s[1] }}
          </button>
        </div>
        <label class="flex items-center gap-1.5 text-sm text-zinc-500">
          size
          <input v-model.number="iconSize" type="number" min="8" max="400" :class="[input, 'w-20']" />
        </label>
        <span class="text-xs text-zinc-400">
          {{ iconMatches.length }} icon{{ iconMatches.length === 1 ? '' : 's' }}{{ iconMatches.length > maxIcons ? ` — showing ${maxIcons}, refine the search` : '' }}
        </span>
      </div>
      <p class="mb-4 text-xs text-zinc-500">
        Click an icon to copy its call. Weather Icons (<code>wi-…</code>, SIL OFL 1.1) are filled shapes; outline icons are Lucide (ISC) and
        take an optional stroke width: <code v-text="'{{ icon &quot;battery&quot; 40 3 }}'" />. Icons inherit the text color.
      </p>
      <div class="grid grid-cols-[repeat(auto-fill,minmax(6.5rem,1fr))] gap-2">
        <button
          v-for="i in iconMatches.slice(0, maxIcons)"
          :key="i.name"
          type="button"
          class="flex flex-col items-center gap-1.5 rounded-md border bg-white px-1 py-3 text-center hover:border-zinc-400"
          :class="copied === i.name ? 'border-emerald-500 ring-1 ring-emerald-500' : 'border-zinc-200'"
          :title="i.tags?.join(', ') || i.name"
          @click="copyIcon(i.name)"
        >
          <img :src="`/icons/${i.name}.svg`" :alt="i.name" class="size-8" loading="lazy" />
          <span class="w-full truncate font-mono text-[10px] text-zinc-500">{{ i.name }}</span>
        </button>
      </div>
      <p v-if="copied" class="sticky bottom-0 mt-4 rounded-md bg-emerald-50 px-3 py-2 text-sm text-emerald-800">
        Copied <code class="font-mono" v-text="iconCall(copied)" />
      </p>
    </div>

    <!-- Snippets -->
    <div v-else class="grid min-h-0 flex-1 grid-cols-[14rem_1fr]">
      <aside class="overflow-y-auto border-r border-zinc-200 bg-white p-2">
        <button type="button" :class="[btn, 'mb-3 w-full justify-center']" @click="newSnippet"><Plus class="size-4" />New snippet</button>
        <button v-for="s in library.snippets" :key="s.id" type="button" :class="listItem(snippet !== 'new' && snippet?.id === s.id)" @click="selectSnippet(s)">
          <span class="truncate font-mono text-xs">{{ s.prefix }}</span>
          <Lock v-if="s.builtin" class="ml-auto size-3 shrink-0 opacity-50" />
        </button>
      </aside>
      <div v-if="!snippet" class="flex flex-col items-center justify-center gap-2 text-sm text-zinc-400">
        <Scissors class="size-8 text-zinc-300" />
        Snippets expand in the template editor: type the prefix outside <code v-text="'{{ }}'" /> and pick it.
      </div>
      <form v-else class="max-w-3xl space-y-3 overflow-y-auto p-6" @submit.prevent="saveSnippet">
        <p v-if="snippet !== 'new' && snippet.builtin" class="flex items-center gap-2 rounded-md bg-zinc-100 px-3 py-2 text-xs text-zinc-600">
          <Lock class="size-3.5" />
          Built-in snippets are read-only. Create your own with a different prefix.
        </p>
        <div class="grid gap-3 sm:grid-cols-[12rem_1fr]">
          <label class="block text-sm">
            <span class="mb-1 block text-zinc-600">Prefix</span>
            <input v-model="sdraft.prefix" :class="[input, 'font-mono']" :disabled="snippet !== 'new' && snippet.builtin" maxlength="40" />
          </label>
          <label class="block text-sm">
            <span class="mb-1 block text-zinc-600">Name</span>
            <input v-model="sdraft.name" :class="input" :disabled="snippet !== 'new' && snippet.builtin" maxlength="100" />
          </label>
        </div>
        <label class="block text-sm">
          <span class="mb-1 block text-zinc-600">Description</span>
          <input v-model="sdraft.description" :class="input" :disabled="snippet !== 'new' && snippet.builtin" maxlength="500" />
        </label>
        <label class="block text-sm">
          <span class="mb-1 block text-zinc-600">Body <span class="text-xs text-zinc-400">— $1, $2 … are tab stops, ${1:text} has a default, $0 is the final cursor</span></span>
          <textarea v-model="sdraft.body" rows="14" :class="[input, 'font-mono text-xs']" :disabled="snippet !== 'new' && snippet.builtin" spellcheck="false" />
        </label>
        <div v-if="snippet === 'new' || !snippet.builtin" class="flex gap-2">
          <button type="submit" :class="primary"><Save class="size-4" />{{ snippet === 'new' ? 'Create' : 'Save' }}</button>
          <button v-if="snippet !== 'new'" type="button" :class="[btn, 'text-red-600']" @click="removeSnippet"><Trash2 class="size-4" /></button>
        </div>
      </form>
    </div>
  </div>
</template>
