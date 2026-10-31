<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { CircleAlert, CircleCheck, FlaskConical, Loader2, Plug, Plus, Trash2 } from 'lucide-vue-next'
import SchemaForm from '@/components/SchemaForm.vue'
import { defaults, useConnectorsStore, type Connector } from '@/stores/connectors'
import { timeAgo } from '@/lib/format'
import type { Schemas } from '@/api/client'

const store = useConnectorsStore()
const error = ref<string | null>(null)

// Editor (create or edit)
const editing = ref<Connector | 'new' | null>(null)
const typeKey = ref('')
const name = ref('')
const config = ref<Record<string, unknown>>({})
const saving = ref(false)

// Test panel
const testParams = ref<Record<string, unknown>>({})
const testResult = ref<Schemas['ConnectorTestResult'] | null>(null)
const testing = ref(false)

const type = computed(() => store.typeOf(editing.value === 'new' ? typeKey.value : (editing.value?.type ?? '')))

onMounted(() => store.load().catch((e: Error) => (error.value = e.message)))

function startNew() {
  editing.value = 'new'
  typeKey.value = store.types[0]?.key ?? ''
  name.value = ''
  config.value = defaults(store.types[0]?.config_schema ?? [])
  testResult.value = null
}

function edit(c: Connector) {
  editing.value = c
  name.value = c.name
  config.value = { ...c.config }
  testParams.value = defaults(store.typeOf(c.type)?.params_schema ?? [])
  testResult.value = null
}

function changeType(key: string) {
  typeKey.value = key
  config.value = defaults(store.typeOf(key)?.config_schema ?? [])
  if (!name.value) name.value = store.typeOf(key)?.name ?? ''
}

async function save() {
  saving.value = true
  error.value = null
  try {
    if (editing.value === 'new') {
      const c = await store.create(typeKey.value, name.value || type.value?.name || typeKey.value, config.value)
      edit(c)
    } else if (editing.value) {
      edit(await store.update(editing.value.id, name.value, config.value))
    }
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    saving.value = false
  }
}

async function remove(c: Connector) {
  const note = c.binding_count ? ` It is used by ${c.binding_count} screen binding(s), which will be removed.` : ''
  if (!confirm(`Delete connector "${c.name}"?${note}`)) return
  try {
    await store.remove(c.id)
    if (editing.value !== 'new' && editing.value?.id === c.id) editing.value = null
  } catch (e) {
    error.value = (e as Error).message
  }
}

async function runTest() {
  if (!editing.value || editing.value === 'new') return
  testing.value = true
  try {
    testResult.value = await store.test(editing.value.id, testParams.value)
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    testing.value = false
  }
}

const input = 'w-full rounded-md border border-zinc-300 bg-white px-2.5 py-1.5 text-sm focus:border-zinc-500 focus:outline-none'
</script>

<template>
  <div class="mx-auto max-w-6xl px-8 py-8">
    <div class="mb-2 flex items-center justify-between">
      <h1 class="text-xl font-semibold">Connectors</h1>
      <button
        type="button"
        class="inline-flex items-center gap-1.5 rounded-md bg-zinc-900 px-3 py-1.5 text-sm text-white hover:bg-zinc-700"
        @click="startNew"
      >
        <Plus class="size-4" />
        New connector
      </button>
    </div>
    <p class="mb-6 text-sm text-zinc-500">
      Data sources for screens. A connector holds the connection (URL, token); screens bind it under an alias with their own settings
      in the editor's Data tab, and templates read it as <code class="rounded bg-zinc-100 px-1">.Data.alias</code>. Data is fetched and
      cached by the server.
    </p>

    <p v-if="error" class="mb-4 rounded-md bg-red-50 px-4 py-2 text-sm text-red-700">{{ error }}</p>

    <div class="grid gap-6 lg:grid-cols-[minmax(0,20rem)_1fr]">
      <!-- List -->
      <div class="space-y-2">
        <div
          v-if="store.loaded && !store.connectors.length"
          class="flex flex-col items-center gap-2 rounded-lg border border-dashed border-zinc-300 bg-white px-6 py-10 text-center text-sm text-zinc-500"
        >
          <Plug class="size-8 text-zinc-300" />
          No connectors yet.
        </div>
        <button
          v-for="c in store.connectors"
          :key="c.id"
          type="button"
          class="flex w-full items-center gap-3 rounded-lg border bg-white px-4 py-3 text-left"
          :class="editing !== 'new' && editing?.id === c.id ? 'border-zinc-900 ring-1 ring-zinc-900' : 'border-zinc-200 hover:border-zinc-400'"
          @click="edit(c)"
        >
          <div class="min-w-0 flex-1">
            <div class="truncate font-medium">{{ c.name }}</div>
            <div class="text-xs text-zinc-500">{{ store.typeOf(c.type)?.name ?? c.type }} · {{ c.binding_count }} binding{{ c.binding_count === 1 ? '' : 's' }}</div>
          </div>
          <span class="rounded p-1 text-zinc-400 hover:text-red-600" title="Delete" @click.stop="remove(c)"><Trash2 class="size-4" /></span>
        </button>
      </div>

      <!-- Editor + test -->
      <div v-if="editing" class="space-y-6">
        <form class="rounded-lg border border-zinc-200 bg-white p-4" @submit.prevent="save">
          <div class="mb-4 grid gap-3 sm:grid-cols-2">
            <label class="block text-sm">
              <span class="mb-1 block text-zinc-600">Type</span>
              <select v-if="editing === 'new'" :value="typeKey" :class="input" @change="changeType(($event.target as HTMLSelectElement).value)">
                <option v-for="t in store.types" :key="t.key" :value="t.key">{{ t.name }}</option>
              </select>
              <div v-else class="py-1.5">{{ type?.name }}</div>
            </label>
            <label class="block text-sm">
              <span class="mb-1 block text-zinc-600">Name</span>
              <input v-model="name" :class="input" :placeholder="type?.name" maxlength="100" />
            </label>
          </div>
          <p v-if="type" class="mb-4 text-xs text-zinc-500">{{ type.description }}</p>
          <SchemaForm
            v-if="type?.config_schema.length"
            v-model="config"
            :fields="type.config_schema"
            :secrets-set="editing === 'new' ? [] : editing.secrets_set"
          />
          <p v-else-if="type" class="text-sm text-zinc-400">No connection settings needed — everything is set per screen binding.</p>
          <div class="mt-4 flex items-center gap-3">
            <button type="submit" class="rounded-md bg-zinc-900 px-3 py-1.5 text-sm text-white hover:bg-zinc-700 disabled:opacity-50" :disabled="saving">
              {{ editing === 'new' ? 'Create' : 'Save' }}
            </button>
            <span v-if="editing !== 'new'" class="text-xs text-zinc-400">updated {{ timeAgo(editing.updated_at) }}</span>
          </div>
        </form>

        <div v-if="editing !== 'new' && type" class="rounded-lg border border-zinc-200 bg-white p-4">
          <div class="mb-3 flex items-center gap-2 text-sm font-medium">
            <FlaskConical class="size-4" />
            Test with screen settings
          </div>
          <SchemaForm v-model="testParams" :fields="type.params_schema" />
          <button
            type="button"
            class="mt-4 inline-flex items-center gap-1.5 rounded-md border border-zinc-200 px-3 py-1.5 text-sm hover:bg-zinc-50 disabled:opacity-50"
            :disabled="testing"
            @click="runTest"
          >
            <Loader2 v-if="testing" class="size-4 animate-spin" />
            Fetch now
          </button>
          <div v-if="testResult" class="mt-4">
            <div class="mb-2 flex items-center gap-1.5 text-sm" :class="testResult.ok ? 'text-emerald-700' : 'text-red-700'">
              <component :is="testResult.ok ? CircleCheck : CircleAlert" class="size-4" />
              {{ testResult.ok ? `OK in ${testResult.duration_ms} ms` : testResult.error }}
            </div>
            <pre v-if="testResult.ok" class="max-h-96 overflow-auto rounded-md bg-zinc-50 p-3 text-xs">{{ JSON.stringify(testResult.data, null, 2) }}</pre>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
