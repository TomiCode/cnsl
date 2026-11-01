<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { Cpu, Loader2, Pencil, Trash2, TriangleAlert, Upload } from 'lucide-vue-next'
import { api, type Schemas } from '@/api/client'
import { unwrap, useDevicesStore } from '@/stores/devices'
import { useEventsStore } from '@/stores/events'
import { dateTime } from '@/lib/format'

type Firmware = Schemas['Firmware']

const devices = useDevicesStore()
const firmware = ref<Firmware[]>([])
const error = ref<string | null>(null)
const message = ref<string | null>(null)

// Models that take updates (the virtual model has no chip).
const models = computed(() => devices.models.filter((m) => m.chip))

async function load() {
  firmware.value = unwrap(await api.GET('/firmware'))
}

onMounted(async () => {
  try {
    await Promise.all([load(), devices.load()])
  } catch (e) {
    error.value = (e as Error).message
  }
})
const unsubscribe = useEventsStore().subscribe((e) => {
  if (e.type === 'model.updated') void load()
})
onBeforeUnmount(unsubscribe)

// Upload
const fileInput = ref<HTMLInputElement | null>(null)
const file = ref<File | null>(null)
const version = ref('')
const notes = ref('')
const uploading = ref(false)

function pick(e: Event) {
  file.value = (e.target as HTMLInputElement).files?.[0] ?? null
  // Same guess as the server: a version in the file name.
  const guess = file.value?.name.match(/\d+\.\d+\.\d+/)?.[0]
  if (guess && !version.value) version.value = guess
}

async function upload() {
  if (!file.value) return
  uploading.value = true
  error.value = null
  message.value = null
  try {
    const form = new FormData()
    form.append('file', file.value)
    if (version.value.trim()) form.append('version', version.value.trim())
    if (notes.value.trim()) form.append('notes', notes.value.trim())
    const res = await fetch('/api/v1/firmware', { method: 'POST', body: form })
    const body = await res.json()
    if (!res.ok) throw new Error(body.detail ?? body.title ?? `upload failed (${res.status})`)
    message.value = `Uploaded ${body.version} for ${chipName(body.chip)}`
    file.value = null
    version.value = ''
    notes.value = ''
    if (fileInput.value) fileInput.value.value = ''
    await load()
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    uploading.value = false
  }
}

async function setTarget(modelId: number, value: string) {
  error.value = null
  message.value = null
  const m = devices.models.find((x) => x.id === modelId)
  if (value && !confirm(`Offer this firmware to every ${m?.name} that has auto-update turned on? A wrong image can only be fixed by flashing over USB.`)) {
    await devices.load() // reset the select
    return
  }
  try {
    unwrap(await api.PUT('/device-models/{id}/firmware', { params: { path: { id: modelId } }, body: { firmware_id: value ? Number(value) : null } }))
    await Promise.all([devices.load(), load()])
  } catch (e) {
    error.value = (e as Error).message
  }
}

async function rename(f: Firmware) {
  const v = prompt('Version the device reports after installing this firmware (FW-Version):', f.version)
  if (!v || v.trim() === f.version) return
  try {
    unwrap(await api.PATCH('/firmware/{id}', { params: { path: { id: f.id } }, body: { version: v.trim(), notes: f.notes } }))
    await load()
  } catch (e) {
    error.value = (e as Error).message
  }
}

async function remove(f: Firmware) {
  const note = f.model_ids.length ? ' It is the target of a model, which will then have none.' : ''
  if (!confirm(`Delete firmware ${f.version} (${f.filename})?${note}`)) return
  try {
    const res = await api.DELETE('/firmware/{id}', { params: { path: { id: f.id } } })
    if (res.error) unwrap(res)
    await Promise.all([load(), devices.load()])
  } catch (e) {
    error.value = (e as Error).message
  }
}

function chipName(chip: string) {
  return chip.startsWith('esp32') && chip.length > 5 ? `ESP32-${chip.slice(5).toUpperCase()}` : chip.toUpperCase()
}
const size = (n: number) => (n < 1024 * 1024 ? `${Math.ceil(n / 1024)} KB` : `${(n / 1024 / 1024).toFixed(2)} MB`)
const modelName = (id: number) => devices.models.find((m) => m.id === id)?.name ?? `model ${id}`
const devicesOf = (modelId: number) => devices.devices.filter((d) => d.model_id === modelId && d.status !== 'pending')

const input = 'rounded-md border border-zinc-300 bg-white px-2.5 py-1.5 text-sm focus:border-zinc-500 focus:outline-none'
</script>

<template>
  <div class="mx-auto max-w-5xl space-y-6 px-8 py-8">
    <div>
      <h1 class="text-xl font-semibold">Firmware</h1>
      <p class="mt-2 text-sm text-zinc-500">
        Upload firmware, pick a target per device model, and turn on auto-update per device (device page). A device is offered the target
        whenever the version it reports differs, at most 3 times a day apart.
      </p>
    </div>

    <p class="flex gap-2 rounded-md bg-amber-50 px-4 py-3 text-sm text-amber-900">
      <TriangleAlert class="mt-0.5 size-4 shrink-0" />
      <span>
        TRMNL firmware installs any app image it is given — there is no signature check, and the TRMNL X can't roll back. cnsl only accepts
        complete app images (<code>firmware.bin</code>, not the merged/factory file) for the model's chip, but a broken build still means
        re-flashing over USB.
      </span>
    </p>
    <p v-if="error" class="rounded-md bg-red-50 px-4 py-2 text-sm text-red-700">{{ error }}</p>
    <p v-if="message" class="rounded-md bg-emerald-50 px-4 py-2 text-sm text-emerald-800">{{ message }}</p>

    <!-- Upload -->
    <form class="rounded-lg border border-zinc-200 bg-white p-4" @submit.prevent="upload">
      <div class="mb-3 text-sm font-medium">Upload</div>
      <div class="flex flex-wrap items-end gap-3">
        <label class="block text-sm">
          <span class="mb-1 block text-zinc-600">App image (.bin)</span>
          <input ref="fileInput" type="file" accept=".bin,application/octet-stream" class="text-sm" @change="pick" />
        </label>
        <label class="block text-sm">
          <span class="mb-1 block text-zinc-600">Version it reports</span>
          <input v-model="version" :class="[input, 'w-32 font-mono']" placeholder="1.8.19" maxlength="40" />
        </label>
        <label class="block min-w-48 flex-1 text-sm">
          <span class="mb-1 block text-zinc-600">Notes</span>
          <input v-model="notes" :class="[input, 'w-full']" maxlength="1000" placeholder="optional" />
        </label>
        <button
          type="submit"
          class="inline-flex items-center gap-1.5 rounded-md bg-zinc-900 px-3 py-1.5 text-sm text-white hover:bg-zinc-700 disabled:opacity-50"
          :disabled="!file || uploading"
        >
          <component :is="uploading ? Loader2 : Upload" class="size-4" :class="{ 'animate-spin': uploading }" />
          Upload
        </button>
      </div>
      <p class="mt-2 text-xs text-zinc-400">
        The version must match what the firmware reports in its <code>FW-Version</code> header (e.g. <code>FW_VERSION_STRING</code> in
        trmnl-firmware's <code>config.h</code>), otherwise cnsl can't tell the update worked.
      </p>
    </form>

    <!-- Targets per model -->
    <div class="rounded-lg border border-zinc-200 bg-white p-4">
      <div class="mb-3 text-sm font-medium">Targets</div>
      <div v-for="m in models" :key="m.id" class="flex flex-wrap items-center gap-3 border-t border-zinc-100 py-2 text-sm first:border-t-0">
        <span class="w-40 font-medium">{{ m.name }}</span>
        <span class="w-20 text-xs text-zinc-500">{{ chipName(m.chip!) }}</span>
        <select :value="m.firmware_id ?? ''" :class="input" @change="setTarget(m.id, ($event.target as HTMLSelectElement).value)">
          <option value="">No target (no updates)</option>
          <option v-for="f in firmware.filter((f) => f.chip === m.chip)" :key="f.id" :value="f.id">{{ f.version }} — {{ f.filename }}</option>
        </select>
        <span class="text-xs text-zinc-500">
          {{ devicesOf(m.id).filter((d) => d.auto_update).length }} of {{ devicesOf(m.id).length }} device(s) with auto-update
        </span>
      </div>
      <p v-if="!models.length" class="text-sm text-zinc-400">No device model takes firmware updates.</p>
    </div>

    <!-- Images -->
    <div class="rounded-lg border border-zinc-200 bg-white">
      <div v-if="!firmware.length" class="flex flex-col items-center gap-2 px-6 py-10 text-sm text-zinc-500">
        <Cpu class="size-8 text-zinc-300" />
        No firmware uploaded yet.
      </div>
      <table v-else class="w-full text-sm">
        <thead class="border-b border-zinc-100 text-left text-xs text-zinc-400">
          <tr>
            <th class="px-4 py-2 font-normal">Version</th>
            <th class="px-4 py-2 font-normal">Chip</th>
            <th class="px-4 py-2 font-normal">Build</th>
            <th class="px-4 py-2 font-normal">File</th>
            <th class="px-4 py-2 font-normal">Target of</th>
            <th />
          </tr>
        </thead>
        <tbody class="divide-y divide-zinc-100">
          <tr v-for="f in firmware" :key="f.id" class="align-top">
            <td class="px-4 py-2">
              <span class="font-mono">{{ f.version }}</span>
              <button type="button" class="ml-1 text-zinc-400 hover:text-zinc-700" title="Change the version label" @click="rename(f)">
                <Pencil class="inline size-3.5" />
              </button>
              <div v-if="f.notes" class="text-xs text-zinc-500">{{ f.notes }}</div>
            </td>
            <td class="px-4 py-2">{{ chipName(f.chip) }}</td>
            <td class="px-4 py-2 text-xs text-zinc-500">
              {{ f.project || '—' }}<br />
              {{ f.built }} · IDF {{ f.idf }}
            </td>
            <td class="px-4 py-2 text-xs text-zinc-500">
              {{ f.filename }}<br />
              {{ size(f.size) }} · <span class="font-mono" :title="f.sha256">{{ f.sha256.slice(0, 12) }}</span> · {{ dateTime(f.created_at) }}
            </td>
            <td class="px-4 py-2 text-xs">{{ f.model_ids.map(modelName).join(', ') || '—' }}</td>
            <td class="px-4 py-2 text-right">
              <button type="button" class="rounded p-1 text-zinc-400 hover:text-red-600" title="Delete" @click="remove(f)"><Trash2 class="size-4" /></button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
