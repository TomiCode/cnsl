<script setup lang="ts">
import { computed, ref } from 'vue'
import { ImageUp, X } from 'lucide-vue-next'
import type { Schemas } from '@/api/client'
import { useDevicesStore, type Device } from '@/stores/devices'
import { imageUrl } from '@/lib/format'

const props = defineProps<{ device: Device }>()
const store = useDevicesStore()

const model = computed(() => store.modelOf(props.device))
const current = computed(() => imageUrl(props.device.image_filename))

const file = ref<File | null>(null)
const localPreview = ref<string | null>(null)
const fit = ref<Schemas['ImageFit']>('contain')
const dither = ref<Schemas['ImageDither']>('floyd-steinberg')
const busy = ref(false)
const error = ref<string | null>(null)
const dragging = ref(false)

function pick(f: File | undefined | null) {
  if (!f) return
  file.value = f
  if (localPreview.value) URL.revokeObjectURL(localPreview.value)
  localPreview.value = URL.createObjectURL(f)
}

async function upload() {
  if (!file.value) return
  busy.value = true
  error.value = null
  try {
    await store.setImage(props.device.id, file.value, fit.value, dither.value)
    file.value = null
    localPreview.value = null
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}

async function clear() {
  busy.value = true
  error.value = null
  try {
    await store.clearImage(props.device.id)
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="grid gap-6 lg:grid-cols-2">
    <div class="rounded-lg border border-zinc-200 bg-white p-4">
      <div class="mb-1 text-sm font-medium">Static image</div>
      <p class="mb-4 text-xs text-zinc-500">
        Shown until screens and playlists arrive. It is resized to
        <template v-if="model">{{ model.width }}×{{ model.height }}</template>
        and reduced to the panel's gray levels.
      </p>

      <label
        class="mb-4 flex cursor-pointer flex-col items-center gap-2 rounded-lg border-2 border-dashed px-4 py-8 text-sm text-zinc-500"
        :class="dragging ? 'border-zinc-500 bg-zinc-50' : 'border-zinc-300'"
        @dragover.prevent="dragging = true"
        @dragleave="dragging = false"
        @drop.prevent="dragging = false; pick($event.dataTransfer?.files[0])"
      >
        <ImageUp class="size-6" />
        <span>{{ file ? file.name : 'Drop an image or click to choose (PNG, JPEG, GIF)' }}</span>
        <input type="file" accept="image/png,image/jpeg,image/gif" class="hidden" @change="pick(($event.target as HTMLInputElement).files?.[0])" />
      </label>
      <img v-if="localPreview" :src="localPreview" alt="Selected image" class="mb-4 max-h-48 rounded border border-zinc-200" />

      <div class="mb-4 grid grid-cols-2 gap-4 text-sm">
        <fieldset>
          <legend class="mb-1 text-zinc-600">Fit</legend>
          <label class="flex items-center gap-2"><input v-model="fit" type="radio" value="contain" /> Contain (white bars)</label>
          <label class="flex items-center gap-2"><input v-model="fit" type="radio" value="cover" /> Cover (crop)</label>
        </fieldset>
        <fieldset>
          <legend class="mb-1 text-zinc-600">Dithering</legend>
          <label class="flex items-center gap-2"><input v-model="dither" type="radio" value="floyd-steinberg" /> Floyd–Steinberg</label>
          <label class="flex items-center gap-2"><input v-model="dither" type="radio" value="none" /> None</label>
        </fieldset>
      </div>

      <p v-if="error" class="mb-3 text-sm text-red-600">{{ error }}</p>
      <button
        type="button"
        class="rounded-md bg-zinc-900 px-3 py-1.5 text-sm text-white hover:bg-zinc-700 disabled:opacity-50"
        :disabled="!file || busy || device.model_id == null"
        @click="upload"
      >
        {{ busy ? 'Processing…' : 'Use this image' }}
      </button>
      <p v-if="device.model_id == null" class="mt-2 text-xs text-amber-700">Assign a model to the device first.</p>
    </div>

    <div class="rounded-lg border border-zinc-200 bg-white p-4">
      <div class="mb-3 flex items-center justify-between text-sm">
        <span class="font-medium">Assigned image (as the device gets it)</span>
        <button v-if="current" type="button" class="inline-flex items-center gap-1 text-zinc-500 hover:text-red-600" :disabled="busy" @click="clear">
          <X class="size-4" />
          Remove
        </button>
      </div>
      <img v-if="current" :src="current" alt="Assigned image" class="w-full rounded border border-zinc-200" />
      <div v-else class="flex aspect-[4/3] items-center justify-center rounded border border-dashed border-zinc-300 text-sm text-zinc-400">
        None — the device shows a placeholder frame
      </div>
    </div>
  </div>
</template>
