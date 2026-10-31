<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { RouterLink, onBeforeRouteLeave, useRoute, useRouter } from 'vue-router'
import { ArrowLeft, CircleAlert, CircleCheck, Eye, Loader2, Play, Save, Trash2 } from 'lucide-vue-next'
import CodeEditor from '@/components/CodeEditor.vue'
import { api, type Schemas } from '@/api/client'
import { ApiError, unwrap, useDevicesStore } from '@/stores/devices'
import { useEventsStore } from '@/stores/events'
import { useScreensStore } from '@/stores/screens'
import { dateTime, duration, imageUrl, timeAgo } from '@/lib/format'
import type { TemplateError } from '@/lib/monaco'

const route = useRoute()
const router = useRouter()
const devices = useDevicesStore()
const screensStore = useScreensStore()
const id = computed(() => Number(route.params.id))

const screen = ref<Schemas['Screen'] | null>(null)
const functions = ref<string[]>([])
const draft = reactive({ name: '', model_id: 0, render_interval: 900, head: '', template: '' })
const tab = ref<'template' | 'head'>('template')
const editor = ref<InstanceType<typeof CodeEditor> | null>(null)

const preview = ref<Schemas['Preview'] | null>(null)
const previewMode = ref<'device' | 'raw'>('device')
const zoom = ref<'fit' | 'actual'>('fit')
const previewing = ref(false)
const saving = ref(false)
const errors = ref<TemplateError[]>([])
const message = ref<{ ok: boolean; text: string } | null>(null)
const renders = ref<Schemas['Render'][]>([])

const region = computed(() => screen.value?.regions[0])
const model = computed(() => devices.models.find((m) => m.id === draft.model_id))
const dirty = computed(() => {
  const s = screen.value
  if (!s) return false
  return (
    draft.name !== s.name ||
    draft.model_id !== s.model_id ||
    draft.render_interval !== s.render_interval ||
    draft.head !== s.head ||
    draft.template !== (region.value?.template ?? '')
  )
})

const intervals = [60, 300, 600, 900, 1800, 3600, 3 * 3600, 6 * 3600, 12 * 3600, 86400]

function reset(s: Schemas['Screen']) {
  screen.value = s
  draft.name = s.name
  draft.model_id = s.model_id
  draft.render_interval = s.render_interval
  draft.head = s.head
  draft.template = s.regions[0]?.template ?? ''
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
    functions.value = unwrap(sys).template_functions
    reset(unwrap(sc))
    await loadRenders()
    void runPreview()
  } catch (e) {
    message.value = { ok: false, text: (e as Error).message }
  }
})

const unsubscribe = useEventsStore().subscribe((e) => {
  if (e.type === 'screen.rendered' && e.screen_id === id.value) void loadRenders()
})
onBeforeUnmount(unsubscribe)

async function runPreview() {
  if (!screen.value || previewing.value) return
  previewing.value = true
  try {
    const key = region.value?.key ?? 'main'
    const res = unwrap(
      await api.POST('/screens/{id}/preview', {
        params: { path: { id: id.value } },
        body: { templates: { [key]: draft.template }, head: draft.head },
      }),
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
  if (!screen.value || !region.value || saving.value) return
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
        regions: [{ id: region.value.id, template: draft.template }],
      },
    })
    if (res.error) {
      const p = res.error as Schemas['TemplateProblem']
      if (p.template_errors?.length) {
        errors.value = p.template_errors
        tab.value = 'template'
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
  } catch (e) {
    message.value = { ok: false, text: (e as Error).message }
  }
}

async function remove() {
  if (!confirm(`Delete screen "${screen.value?.name}"? It is removed from all playlists.`)) return
  await screensStore.remove(id.value)
  draft.template = region.value?.template ?? '' // not dirty anymore
  await router.push('/screens')
}

onBeforeRouteLeave(() => !dirty.value || confirm('Leave without saving your changes?'))
const beforeUnload = (e: BeforeUnloadEvent) => {
  if (dirty.value) e.preventDefault()
}
window.addEventListener('beforeunload', beforeUnload)
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload))

function reveal(e: TemplateError) {
  tab.value = 'template'
  editor.value?.revealLine(e.line)
}

const btn = 'inline-flex items-center gap-1.5 rounded-md border border-zinc-200 bg-white px-3 py-1.5 text-sm hover:bg-zinc-50 disabled:opacity-50'
const input = 'rounded-md border border-zinc-300 bg-white px-2.5 py-1.5 text-sm focus:border-zinc-500 focus:outline-none'
</script>

<template>
  <div class="flex h-full flex-col">
    <!-- Header -->
    <header class="flex flex-wrap items-center gap-2 border-b border-zinc-200 bg-white px-4 py-2.5">
      <RouterLink to="/screens" class="rounded-md p-1.5 text-zinc-500 hover:bg-zinc-100" title="Back to screens">
        <ArrowLeft class="size-4" />
      </RouterLink>
      <input v-model="draft.name" :class="[input, 'w-48 font-medium']" maxlength="100" aria-label="Screen name" />
      <select v-model="draft.model_id" :class="input" aria-label="Device model">
        <option v-for="m in devices.models" :key="m.id" :value="m.id">{{ m.name }}</option>
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
      <!-- Editor -->
      <section class="flex min-h-0 flex-col border-zinc-200 lg:border-r">
        <div class="flex gap-1 border-b border-zinc-200 bg-zinc-50 px-2 text-sm">
          <button
            v-for="t in ['template', 'head'] as const"
            :key="t"
            type="button"
            class="-mb-px border-b-2 px-3 py-2"
            :class="tab === t ? 'border-zinc-900 font-medium' : 'border-transparent text-zinc-500 hover:text-zinc-900'"
            @click="tab = t"
          >
            {{ t === 'template' ? 'Template' : 'Head (CSS)' }}
          </button>
          <span class="ml-auto self-center text-xs text-zinc-400">Go html/template · trmnl-framework {{ screen.framework_version }}</span>
        </div>
        <div class="min-h-0 flex-1">
          <CodeEditor
            v-if="tab === 'template'"
            ref="editor"
            v-model="draft.template"
            :functions="functions"
            :errors="errors"
            @save="save"
            @preview="runPreview"
          />
          <CodeEditor v-else v-model="draft.head" :functions="functions" @save="save" @preview="runPreview" />
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
          <span v-if="preview" class="ml-auto text-xs text-zinc-400">{{ preview.duration_ms }} ms · {{ model?.width }}×{{ model?.height }}</span>
        </div>

        <div class="p-4" :class="{ 'overflow-auto': zoom === 'actual' }">
          <img
            v-if="preview"
            :src="previewMode === 'device' ? preview.device : preview.raw"
            alt="Screen preview"
            class="border border-zinc-300 bg-white shadow-sm"
            :class="zoom === 'fit' ? 'w-full' : 'max-w-none [image-rendering:pixelated]'"
          />
          <div v-else class="flex aspect-[4/3] items-center justify-center rounded border border-dashed border-zinc-300 text-sm text-zinc-400">
            {{ previewing ? 'Rendering…' : 'No preview yet' }}
          </div>

          <ul v-if="errors.length" class="mt-4 space-y-1">
            <li v-for="(e, i) in errors" :key="i">
              <button type="button" class="flex w-full items-start gap-2 rounded-md bg-red-50 px-3 py-2 text-left text-sm text-red-800 hover:bg-red-100" @click="reveal(e)">
                <CircleAlert class="mt-0.5 size-4 shrink-0" />
                <span><span class="font-mono text-xs">line {{ e.line }}{{ e.column ? `:${e.column}` : '' }}</span> {{ e.message }}</span>
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
