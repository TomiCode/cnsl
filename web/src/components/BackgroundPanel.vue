<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { RouterLink } from 'vue-router'
import { Check } from 'lucide-vue-next'
import type { Schemas } from '@/api/client'
import { useAssetsStore } from '@/stores/assets'

type Background = Schemas['Background']

const props = defineProps<{ modelValue: Background; imageOnly?: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: Background] }>()

const assets = useAssetsStore()
onMounted(() => {
  if (!assets.loaded) void assets.load()
})

const bg = computed(() => props.modelValue)
const ids = computed(() => bg.value.asset_ids ?? [])

function set(patch: Partial<Background>) {
  emit('update:modelValue', { ...bg.value, ...patch })
}

function setKind(kind: Background['kind']) {
  let asset_ids = ids.value
  if (kind === 'image') asset_ids = asset_ids.slice(0, 1)
  if (kind === 'none') asset_ids = []
  set({ kind, asset_ids })
}

function toggle(id: number) {
  if (bg.value.kind === 'image' || bg.value.kind === 'none') {
    set({ kind: 'image', asset_ids: [id] })
    return
  }
  set({ asset_ids: ids.value.includes(id) ? ids.value.filter((x) => x !== id) : [...ids.value, id] })
}

const kinds = computed(() =>
  (props.imageOnly ? ['image', 'slideshow'] : ['none', 'image', 'slideshow']) as Background['kind'][],
)
const kindLabel: Record<Background['kind'], string> = { none: 'None', image: 'One image', slideshow: 'Slideshow' }
const input = 'rounded-md border border-zinc-300 bg-white px-2.5 py-1.5 text-sm focus:border-zinc-500 focus:outline-none'
</script>

<template>
  <div class="space-y-4 text-sm">
    <div class="flex overflow-hidden rounded-md border border-zinc-200 w-fit">
      <button
        v-for="k in kinds"
        :key="k"
        type="button"
        class="px-3 py-1.5"
        :class="bg.kind === k ? 'bg-zinc-900 text-white' : 'bg-white text-zinc-600 hover:bg-zinc-50'"
        @click="setKind(k)"
      >
        {{ kindLabel[k] }}
      </button>
    </div>

    <template v-if="bg.kind !== 'none'">
      <div class="flex flex-wrap gap-3">
        <label class="flex items-center gap-1.5 text-zinc-600">
          Fit
          <select :value="bg.fit" :class="input" @change="set({ fit: ($event.target as HTMLSelectElement).value as Schemas['ImageFit'] })">
            <option value="cover">Cover (crop)</option>
            <option value="contain">Contain</option>
          </select>
        </label>
        <label class="flex items-center gap-1.5 text-zinc-600">
          Dithering
          <select :value="bg.dither" :class="input" @change="set({ dither: ($event.target as HTMLSelectElement).value as Schemas['ImageDither'] })">
            <option value="floyd-steinberg">Floyd–Steinberg</option>
            <option value="atkinson">Atkinson</option>
            <option value="ordered">Ordered (Bayer)</option>
            <option value="none">None</option>
          </select>
        </label>
        <label v-if="bg.kind === 'slideshow'" class="flex items-center gap-1.5 text-zinc-600">
          Order
          <select :value="bg.order" :class="input" @change="set({ order: ($event.target as HTMLSelectElement).value as 'sequential' | 'shuffle' })">
            <option value="sequential">In order</option>
            <option value="shuffle">Shuffled</option>
          </select>
        </label>
      </div>
      <p v-if="bg.kind === 'slideshow'" class="text-xs text-zinc-500">
        The next image is used on each render (save, the screen's interval, or a device needing it). Ordered dithering avoids shimmer between images.
      </p>

      <div v-if="assets.loaded && !assets.assets.length" class="rounded-md border border-dashed border-zinc-300 p-4 text-zinc-500">
        No images yet. <RouterLink to="/images" class="underline">Upload some</RouterLink>.
      </div>
      <div class="grid gap-2 [grid-template-columns:repeat(auto-fill,minmax(6.5rem,1fr))]">
        <button
          v-for="a in assets.assets"
          :key="a.id"
          type="button"
          class="relative overflow-hidden rounded-md border text-left"
          :class="ids.includes(a.id) ? 'border-zinc-900 ring-2 ring-zinc-900' : 'border-zinc-200 hover:border-zinc-400'"
          :title="a.name"
          @click="toggle(a.id)"
        >
          <img :src="a.url" :alt="a.name" class="aspect-[4/3] w-full bg-zinc-100 object-cover" loading="lazy" />
          <span
            v-if="ids.includes(a.id)"
            class="absolute top-1 right-1 flex size-5 items-center justify-center rounded-full bg-zinc-900 text-[11px] text-white"
          >
            <template v-if="bg.kind === 'slideshow'">{{ ids.indexOf(a.id) + 1 }}</template>
            <Check v-else class="size-3" />
          </span>
          <span class="block truncate px-1.5 py-1 text-[11px] text-zinc-600">{{ a.name }}</span>
        </button>
      </div>
    </template>
  </div>
</template>
