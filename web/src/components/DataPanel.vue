<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import { ChevronDown, ChevronRight, CircleAlert, Clock, Plus, RefreshCw, X } from 'lucide-vue-next'
import SchemaForm from '@/components/SchemaForm.vue'
import { api, type Schemas } from '@/api/client'
import { unwrap } from '@/stores/devices'
import { defaults, useConnectorsStore } from '@/stores/connectors'
import { timeAgo } from '@/lib/format'

type Binding = Schemas['Binding']
type BindingData = Schemas['BindingData']

const props = defineProps<{ screenId: number; regionKeys: string[]; data: Record<string, BindingData> }>()
const emit = defineEmits<{ saved: []; refresh: [] }>()

const connectors = useConnectorsStore()
const bindings = ref<Binding[]>([])
const saved = ref('')
const saving = ref(false)
const error = ref<string | null>(null)
const open = ref<Record<string, boolean>>({})

const dirty = computed(() => JSON.stringify(bindings.value) !== saved.value)

async function load() {
  // The API omits region_key for screen-wide bindings; the region select needs an explicit null.
  const list = unwrap(await api.GET('/screens/{id}/bindings', { params: { path: { id: props.screenId } } })).map((b) => ({
    ...b,
    region_key: b.region_key ?? null,
  }))
  bindings.value = list
  saved.value = JSON.stringify(list)
}

onMounted(async () => {
  try {
    await Promise.all([load(), connectors.loaded ? Promise.resolve() : connectors.load()])
  } catch (e) {
    error.value = (e as Error).message
  }
})
watch(() => props.screenId, load)

const aliasHints: Record<string, string> = { homeassistant: 'ha', openmeteo: 'weather', status: 'servers', http_json: 'api' }

function add() {
  const c = connectors.connectors[0]
  if (!c) return
  let alias = aliasHints[c.type] ?? 'data'
  for (let n = 2; bindings.value.some((b) => b.alias === alias); n++) alias = `${aliasHints[c.type] ?? 'data'}${n}`
  bindings.value.push({ connector_id: c.id, alias, params: defaults(connectors.typeOf(c.type)?.params_schema ?? []), ttl: 300, enabled: true, region_key: null })
}

function changeConnector(b: Binding, id: number) {
  const c = connectors.byId(id)
  b.connector_id = id
  b.params = defaults(connectors.typeOf(c?.type)?.params_schema ?? [])
}

async function save() {
  saving.value = true
  error.value = null
  try {
    const list = unwrap(
      await api.PUT('/screens/{id}/bindings', { params: { path: { id: props.screenId } }, body: bindings.value }),
    )
    bindings.value = list.map((b) => ({ ...b, region_key: b.region_key ?? null }))
    saved.value = JSON.stringify(bindings.value)
    emit('saved')
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    saving.value = false
  }
}

function paramsFields(b: Binding) {
  return connectors.typeOf(connectors.byId(b.connector_id)?.type)?.params_schema ?? []
}

const input = 'rounded-md border border-zinc-300 bg-white px-2.5 py-1.5 text-sm focus:border-zinc-500 focus:outline-none'
</script>

<template>
  <div class="space-y-4 text-sm">
    <p class="text-zinc-500">
      Bind connectors to this screen. Templates read the data as <code class="rounded bg-zinc-100 px-1">.Data.alias</code>; typing
      <code class="rounded bg-zinc-100 px-1">.Data.</code> in the editor suggests the fields.
      <template v-if="connectors.loaded && !connectors.connectors.length">
        First <RouterLink to="/connectors" class="underline">create a connector</RouterLink>.
      </template>
    </p>
    <p v-if="error" class="rounded-md bg-red-50 px-3 py-2 text-red-700">{{ error }}</p>

    <div v-for="(b, i) in bindings" :key="b.id ?? `new-${i}`" class="rounded-lg border border-zinc-200 bg-white">
      <div class="flex flex-wrap items-center gap-2 border-b border-zinc-100 px-3 py-2">
        <span class="text-zinc-400">.Data.</span>
        <input v-model="b.alias" :class="[input, 'w-28 font-mono']" aria-label="Alias" pattern="[a-z][a-z0-9_]*" />
        <select :value="b.connector_id" :class="input" aria-label="Connector" @change="changeConnector(b, Number(($event.target as HTMLSelectElement).value))">
          <option v-for="c in connectors.connectors" :key="c.id" :value="c.id">{{ c.name }}</option>
        </select>
        <select v-model="b.region_key" :class="input" aria-label="Region">
          <option :value="null">all regions</option>
          <option v-for="k in regionKeys" :key="k" :value="k">only {{ k }}</option>
        </select>
        <label class="flex items-center gap-1 text-zinc-500" title="Seconds before the data is fetched again">
          <Clock class="size-3.5" />
          <input v-model.number="b.ttl" type="number" min="30" max="86400" step="30" :class="[input, 'w-20']" />s
        </label>
        <label class="flex items-center gap-1 text-zinc-500"><input v-model="b.enabled" type="checkbox" /> on</label>
        <button type="button" class="ml-auto rounded p-1 text-zinc-400 hover:text-red-600" title="Remove" @click="bindings.splice(i, 1)"><X class="size-4" /></button>
      </div>
      <div class="p-3">
        <SchemaForm v-model="b.params" :fields="paramsFields(b)" />
      </div>
      <!-- Current data -->
      <div v-if="b.id && data[b.alias]" class="border-t border-zinc-100 px-3 py-2">
        <button type="button" class="flex w-full items-center gap-1.5 text-left text-xs text-zinc-600" @click="open[b.alias] = !open[b.alias]">
          <component :is="open[b.alias] ? ChevronDown : ChevronRight" class="size-3.5" />
          <span v-if="data[b.alias]!.error" class="flex items-center gap-1 text-red-700"><CircleAlert class="size-3.5" />{{ data[b.alias]!.error }}</span>
          <span v-else>data fetched {{ timeAgo(data[b.alias]!.fetched_at) }}</span>
          <span v-if="data[b.alias]!.stale" class="rounded bg-amber-100 px-1 text-amber-800">stale</span>
        </button>
        <pre v-if="open[b.alias]" class="mt-2 max-h-72 overflow-auto rounded-md bg-zinc-50 p-2 text-xs">{{ JSON.stringify(data[b.alias]!.data, null, 2) }}</pre>
      </div>
    </div>

    <div class="flex flex-wrap items-center gap-3">
      <button
        type="button"
        class="inline-flex items-center gap-1.5 text-zinc-600 hover:text-zinc-900 disabled:opacity-50"
        :disabled="!connectors.connectors.length"
        @click="add"
      >
        <Plus class="size-4" />
        Add binding
      </button>
      <button
        type="button"
        class="ml-auto rounded-md bg-zinc-900 px-3 py-1.5 text-white hover:bg-zinc-700 disabled:opacity-50"
        :disabled="!dirty || saving"
        @click="save"
      >
        Save bindings
      </button>
      <button type="button" class="inline-flex items-center gap-1 text-zinc-500 hover:text-zinc-900" title="Fetch data again" @click="emit('refresh')">
        <RefreshCw class="size-4" />
      </button>
    </div>
  </div>
</template>
