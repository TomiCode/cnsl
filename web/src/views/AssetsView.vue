<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue'
import { ImageUp, Images, Loader2, Trash2, X } from 'lucide-vue-next'
import type { Schemas } from '@/api/client'
import { useAssetsStore, type Asset } from '@/stores/assets'
import { useDevicesStore } from '@/stores/devices'

const store = useAssetsStore()
const devices = useDevicesStore()

const error = ref<string | null>(null)
const uploading = ref(0)
const dragging = ref(false)

// Preview panel for one asset
const selected = ref<Asset | null>(null)
const opts = reactive({ model_id: 0, fit: 'cover' as Schemas['ImageFit'], dither: 'floyd-steinberg' as Schemas['ImageDither'] })
const preview = ref<string | null>(null)
const previewing = ref(false)

onMounted(async () => {
  try {
    await Promise.all([store.load(), devices.loaded ? Promise.resolve() : devices.load()])
    opts.model_id = devices.models[0]?.id ?? 0
  } catch (e) {
    error.value = (e as Error).message
  }
})

async function uploadFiles(files: FileList | File[] | null | undefined) {
  if (!files) return
  error.value = null
  for (const f of Array.from(files)) {
    uploading.value++
    try {
      await store.upload(f)
    } catch (e) {
      error.value = `${f.name}: ${(e as Error).message}`
    } finally {
      uploading.value--
    }
  }
}

async function remove(a: Asset) {
  if (!confirm(`Delete "${a.name}"?`)) return
  try {
    await store.remove(a.id)
    if (selected.value?.id === a.id) selected.value = null
  } catch (e) {
    error.value = (e as Error).message
  }
}

async function renderPreview() {
  if (!selected.value || !opts.model_id) return
  previewing.value = true
  try {
    preview.value = await store.preview(selected.value.id, opts.model_id, opts.fit, opts.dither)
  } catch (e) {
    error.value = (e as Error).message
  } finally {
    previewing.value = false
  }
}
watch([selected, () => ({ ...opts })], renderPreview, { deep: true })

function size(bytes: number) {
  return bytes > 1 << 20 ? `${(bytes / (1 << 20)).toFixed(1)} MB` : `${Math.round(bytes / 1024)} KB`
}

const input = 'rounded-md border border-zinc-300 bg-white px-2.5 py-1.5 text-sm focus:border-zinc-500 focus:outline-none'
</script>

<template>
  <div class="mx-auto max-w-6xl px-8 py-8">
    <h1 class="mb-2 text-xl font-semibold">Images</h1>
    <p class="mb-6 text-sm text-zinc-500">
      Uploaded images for screen backgrounds, slideshows and image screens. They are stored as uploaded and prepared per device model
      (resized, gray-curved, dithered) when a screen uses them.
    </p>

    <p v-if="error" class="mb-4 rounded-md bg-red-50 px-4 py-2 text-sm text-red-700">{{ error }}</p>

    <label
      class="mb-6 flex cursor-pointer flex-col items-center gap-2 rounded-lg border-2 border-dashed bg-white px-4 py-8 text-sm text-zinc-500"
      :class="dragging ? 'border-zinc-500 bg-zinc-50' : 'border-zinc-300'"
      @dragover.prevent="dragging = true"
      @dragleave="dragging = false"
      @drop.prevent="dragging = false; uploadFiles($event.dataTransfer?.files)"
    >
      <component :is="uploading ? Loader2 : ImageUp" class="size-6" :class="{ 'animate-spin': uploading }" />
      <span>{{ uploading ? `Uploading ${uploading}…` : 'Drop images here or click to choose (PNG, JPEG, GIF, up to 25 MB)' }}</span>
      <input type="file" accept="image/png,image/jpeg,image/gif" multiple class="hidden" @change="uploadFiles(($event.target as HTMLInputElement).files)" />
    </label>

    <div
      v-if="store.loaded && !store.assets.length"
      class="flex flex-col items-center gap-2 rounded-lg border border-dashed border-zinc-300 bg-white px-6 py-12 text-center text-sm text-zinc-500"
    >
      <Images class="size-8 text-zinc-300" />
      <p>No images yet.</p>
    </div>

    <div class="grid gap-6" :class="{ 'lg:grid-cols-[1fr_minmax(0,28rem)]': selected }">
      <div class="grid content-start gap-3 [grid-template-columns:repeat(auto-fill,minmax(9rem,1fr))]">
        <div
          v-for="a in store.assets"
          :key="a.id"
          class="group relative cursor-pointer overflow-hidden rounded-lg border bg-white"
          :class="selected?.id === a.id ? 'border-zinc-900 ring-1 ring-zinc-900' : 'border-zinc-200 hover:border-zinc-400'"
          @click="selected = a"
        >
          <img :src="a.url" :alt="a.name" class="aspect-[4/3] w-full bg-zinc-100 object-cover" loading="lazy" />
          <div class="px-2 py-1.5">
            <div class="truncate text-xs font-medium">{{ a.name }}</div>
            <div class="text-[11px] text-zinc-400">{{ a.width }}×{{ a.height }} · {{ size(a.size) }}</div>
          </div>
          <button
            type="button"
            class="absolute top-1.5 right-1.5 hidden rounded-md bg-white/90 p-1 text-zinc-500 shadow group-hover:block hover:text-red-600"
            title="Delete"
            @click.stop="remove(a)"
          >
            <Trash2 class="size-4" />
          </button>
        </div>
      </div>

      <aside v-if="selected" class="h-fit rounded-lg border border-zinc-200 bg-white p-4 text-sm lg:sticky lg:top-4">
        <div class="mb-3 flex items-center justify-between">
          <span class="truncate font-medium">{{ selected.name }}</span>
          <button type="button" class="text-zinc-400 hover:text-zinc-900" title="Close" @click="selected = null"><X class="size-4" /></button>
        </div>
        <div class="mb-3 flex flex-wrap gap-2">
          <select v-model="opts.model_id" :class="input" aria-label="Device model">
            <option v-for="m in devices.models" :key="m.id" :value="m.id">{{ m.name }}</option>
          </select>
          <select v-model="opts.fit" :class="input" aria-label="Fit">
            <option value="cover">Cover (crop)</option>
            <option value="contain">Contain</option>
          </select>
          <select v-model="opts.dither" :class="input" aria-label="Dithering">
            <option value="floyd-steinberg">Floyd–Steinberg</option>
            <option value="atkinson">Atkinson</option>
            <option value="ordered">Ordered (Bayer)</option>
            <option value="none">No dithering</option>
          </select>
        </div>
        <div class="relative">
          <img v-if="preview" :src="preview" alt="Device preview" class="w-full rounded border border-zinc-200" />
          <div v-else class="flex aspect-[4/3] items-center justify-center rounded border border-dashed border-zinc-300 text-zinc-400">Rendering…</div>
          <Loader2 v-if="previewing" class="absolute top-2 right-2 size-4 animate-spin text-zinc-500" />
        </div>
        <p class="mt-2 text-xs text-zinc-400">As the device gets it, with the model's gray curve. Floyd–Steinberg suits photos; ordered stays stable between slideshow renders.</p>
      </aside>
    </div>
  </div>
</template>
