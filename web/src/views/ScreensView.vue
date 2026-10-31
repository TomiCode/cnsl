<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { CircleAlert, PanelsTopLeft, Plus } from 'lucide-vue-next'
import { useScreensStore } from '@/stores/screens'
import { useDevicesStore } from '@/stores/devices'
import { imageUrl, timeAgo } from '@/lib/format'

const screens = useScreensStore()
const devices = useDevicesStore()
const router = useRouter()

const creating = ref(false)
const name = ref('')
const modelId = ref<number | null>(null)
const error = ref<string | null>(null)

onMounted(async () => {
  try {
    await Promise.all([screens.load(), devices.loaded ? Promise.resolve() : devices.load()])
    modelId.value = devices.models[0]?.id ?? null
  } catch (e) {
    error.value = (e as Error).message
  }
})

async function create() {
  if (!name.value.trim() || modelId.value == null) return
  try {
    const sc = await screens.create(name.value.trim(), modelId.value)
    await router.push(`/screens/${sc.id}`)
  } catch (e) {
    error.value = (e as Error).message
  }
}

function modelName(id: number) {
  return devices.models.find((m) => m.id === id)?.name ?? '—'
}
function aspect(id: number) {
  const m = devices.models.find((m) => m.id === id)
  return m ? `${m.width} / ${m.height}` : '4 / 3'
}

const input = 'rounded-md border border-zinc-300 bg-white px-3 py-1.5 text-sm focus:border-zinc-500 focus:outline-none'
</script>

<template>
  <div class="mx-auto max-w-6xl px-8 py-8">
    <div class="mb-6 flex items-center justify-between">
      <h1 class="text-xl font-semibold">Screens</h1>
      <button
        v-if="!creating"
        type="button"
        class="inline-flex items-center gap-1.5 rounded-md bg-zinc-900 px-3 py-1.5 text-sm text-white hover:bg-zinc-700"
        @click="creating = true"
      >
        <Plus class="size-4" />
        New screen
      </button>
    </div>

    <p v-if="error" class="mb-4 rounded-md bg-red-50 px-4 py-2 text-sm text-red-700">{{ error }}</p>

    <form v-if="creating" class="mb-6 flex flex-wrap items-end gap-3 rounded-lg border border-zinc-200 bg-white p-4" @submit.prevent="create">
      <label class="text-sm">
        <span class="mb-1 block text-zinc-600">Name</span>
        <input v-model="name" :class="input" placeholder="Kitchen dashboard" autofocus maxlength="100" />
      </label>
      <label class="text-sm">
        <span class="mb-1 block text-zinc-600">For model</span>
        <select v-model="modelId" :class="input">
          <option v-for="m in devices.models" :key="m.id" :value="m.id">{{ m.name }} ({{ m.width }}×{{ m.height }})</option>
        </select>
      </label>
      <button type="submit" class="rounded-md bg-zinc-900 px-3 py-1.5 text-sm text-white hover:bg-zinc-700 disabled:opacity-50" :disabled="!name.trim()">
        Create
      </button>
      <button type="button" class="text-sm text-zinc-500 hover:text-zinc-900" @click="creating = false">Cancel</button>
    </form>

    <div
      v-if="screens.loaded && !screens.screens.length"
      class="flex flex-col items-center gap-2 rounded-lg border border-dashed border-zinc-300 bg-white px-6 py-12 text-center text-sm text-zinc-500"
    >
      <PanelsTopLeft class="size-8 text-zinc-300" />
      <p>No screens yet. A screen is an HTML template rendered into an image for one device model.</p>
    </div>

    <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
      <RouterLink
        v-for="sc in screens.screens"
        :key="sc.id"
        :to="`/screens/${sc.id}`"
        class="group overflow-hidden rounded-lg border border-zinc-200 bg-white hover:border-zinc-400"
      >
        <div class="border-b border-zinc-100 bg-zinc-50 p-3">
          <img
            v-if="sc.latest_image"
            :src="imageUrl(sc.latest_image)!"
            :alt="`${sc.name} preview`"
            class="w-full rounded-sm border border-zinc-200 bg-white"
            :style="{ aspectRatio: aspect(sc.model_id) }"
          />
          <div v-else class="flex items-center justify-center rounded-sm border border-dashed border-zinc-300 text-xs text-zinc-400" :style="{ aspectRatio: aspect(sc.model_id) }">
            not rendered yet
          </div>
        </div>
        <div class="px-4 py-3">
          <div class="font-medium group-hover:underline">{{ sc.name }}</div>
          <div class="flex items-center gap-1.5 text-xs text-zinc-500">
            {{ modelName(sc.model_id) }} · rendered {{ timeAgo(sc.latest_render?.created_at) }}
            <CircleAlert v-if="sc.latest_render?.status === 'error'" class="size-3.5 text-red-600" title="Last render failed" />
          </div>
        </div>
      </RouterLink>
    </div>
  </div>
</template>
