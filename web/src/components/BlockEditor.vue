<script setup lang="ts">
import { computed, ref } from 'vue'
import { ArrowDown, ArrowUp, Code, GripVertical, Plus, Trash2 } from 'lucide-vue-next'
import type { Schemas } from '@/api/client'
import type { Block } from '@/lib/blocks'

type Component = Schemas['Component']
type Input = Schemas['ComponentParam']

const props = defineProps<{
  modelValue: Block[]
  components: Component[]
  dataPaths: string[] // for completing data inputs
  highlight?: number | null // block picked on the preview
}>()
const emit = defineEmits<{
  'update:modelValue': [blocks: Block[]]
  add: [component: Component | null] // null = HTML snippet
}>()

const adding = ref(false)
const listId = `paths-${Math.random().toString(36).slice(2)}`
const byKey = computed(() => new Map(props.components.map((c) => [c.key, c])))

// Blocks are edited in place (the parent's draft is reactive); structural changes go through emit.
function update(next: Block[]) {
  emit('update:modelValue', next)
}
function move(i: number, to: number) {
  if (to < 0 || to >= props.modelValue.length) return
  const next = [...props.modelValue]
  const [b] = next.splice(i, 1)
  next.splice(to, 0, b!)
  update(next)
}
function remove(i: number) {
  update(props.modelValue.filter((_, j) => j !== i))
}

function arg(b: Block, name: string) {
  b.args ??= {}
  b.args[name] ??= {}
  return b.args[name]!
}
function setValue(b: Block, input: Input, raw: string | boolean) {
  const a = arg(b, input.name)
  if (raw === '' || raw === 'default') delete a.value
  else if (input.kind === 'number') a.value = Number(raw)
  else if (input.kind === 'bool') a.value = raw === 'true' || raw === true
  else a.value = raw
}
function boolState(b: Block, name: string) {
  const v = b.args?.[name]?.value
  return v === undefined ? 'default' : String(v)
}

// Drag to reorder (native drag and drop).
const dragging = ref<number | null>(null)
const over = ref<number | null>(null)
function drop(i: number) {
  if (dragging.value !== null && dragging.value !== i) move(dragging.value, i)
  dragging.value = over.value = null
}

function pick(c: Component | null) {
  adding.value = false
  emit('add', c)
}

const field = 'w-full rounded border border-zinc-300 bg-white px-2 py-1 text-xs focus:border-zinc-500 focus:outline-none'
defineExpose({
  scrollTo(i: number) {
    document.getElementById(`${listId}-block-${i}`)?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
  },
})
</script>

<template>
  <div class="space-y-2 p-3">
    <datalist :id="listId">
      <option v-for="p in dataPaths" :key="p" :value="p" />
    </datalist>

    <p v-if="!modelValue.length" class="rounded-md border border-dashed border-zinc-300 px-4 py-6 text-center text-sm text-zinc-500">
      No blocks yet. Add a sensor, the weather, a clock … — the region's arrangement places them.
    </p>

    <div
      v-for="(b, i) in modelValue"
      :id="`${listId}-block-${i}`"
      :key="i"
      class="rounded-md border bg-white"
      :class="[
        highlight === i ? 'border-sky-500 ring-1 ring-sky-500' : 'border-zinc-200',
        over === i && dragging !== i ? 'border-t-4 border-t-sky-400' : '',
      ]"
      @dragover.prevent="over = i"
      @drop.prevent="drop(i)"
    >
      <div class="flex items-center gap-1.5 border-b border-zinc-100 px-2 py-1.5 text-sm">
        <span
          class="cursor-grab text-zinc-400 hover:text-zinc-700"
          draggable="true"
          title="Drag to reorder"
          @dragstart="dragging = i"
          @dragend="dragging = over = null"
        >
          <GripVertical class="size-4" />
        </span>
        <Code v-if="!b.component" class="size-4 text-zinc-400" />
        <span class="font-medium">{{ b.component ? (byKey.get(b.component)?.name ?? b.component) : 'HTML snippet' }}</span>
        <span v-if="b.component && !byKey.get(b.component)" class="text-xs text-red-600">unknown component</span>
        <div class="ml-auto flex items-center">
          <button type="button" class="rounded p-1 text-zinc-400 hover:text-zinc-700 disabled:opacity-30" :disabled="i === 0" title="Move up" @click="move(i, i - 1)">
            <ArrowUp class="size-3.5" />
          </button>
          <button
            type="button"
            class="rounded p-1 text-zinc-400 hover:text-zinc-700 disabled:opacity-30"
            :disabled="i === modelValue.length - 1"
            title="Move down"
            @click="move(i, i + 1)"
          >
            <ArrowDown class="size-3.5" />
          </button>
          <button type="button" class="rounded p-1 text-zinc-400 hover:text-red-600" title="Remove block" @click="remove(i)">
            <Trash2 class="size-3.5" />
          </button>
        </div>
      </div>

      <!-- Component inputs -->
      <div v-if="b.component" class="grid grid-cols-[minmax(6rem,auto)_1fr] items-center gap-x-3 gap-y-1.5 px-3 py-2 text-xs">
        <template v-for="input in byKey.get(b.component)?.inputs ?? []" :key="input.name">
          <label :for="`${listId}-${i}-${input.name}`" class="text-zinc-600" :title="input.help">{{ input.label || input.name }}</label>
          <input
            v-if="input.kind === 'data'"
            :id="`${listId}-${i}-${input.name}`"
            :value="b.args?.[input.name]?.path ?? ''"
            :list="listId"
            :class="[field, 'font-mono']"
            placeholder=".Data.alias"
            @input="arg(b, input.name).path = ($event.target as HTMLInputElement).value"
          />
          <select
            v-else-if="input.kind === 'bool'"
            :id="`${listId}-${i}-${input.name}`"
            :value="boolState(b, input.name)"
            :class="field"
            @change="setValue(b, input, ($event.target as HTMLSelectElement).value)"
          >
            <option value="default">default ({{ input.default ? 'yes' : 'no' }})</option>
            <option value="true">yes</option>
            <option value="false">no</option>
          </select>
          <input
            v-else
            :id="`${listId}-${i}-${input.name}`"
            :type="input.kind === 'number' ? 'number' : 'text'"
            :value="b.args?.[input.name]?.value ?? ''"
            :placeholder="input.default !== undefined ? String(input.default) : ''"
            :class="field"
            @input="setValue(b, input, ($event.target as HTMLInputElement).value)"
          />
        </template>
      </div>

      <!-- HTML snippet -->
      <textarea
        v-else
        v-model="b.html"
        rows="4"
        spellcheck="false"
        :class="[field, 'm-0 block rounded-none rounded-b-md border-0 font-mono']"
        placeholder='<span class="title">{{ .Screen.Name }}</span>'
      />
    </div>

    <div class="relative">
      <button type="button" class="inline-flex items-center gap-1 text-sm text-zinc-600 hover:text-zinc-900" @click="adding = !adding">
        <Plus class="size-4" />
        Add block
      </button>
      <div v-if="adding" class="absolute left-0 z-20 mt-1 max-h-96 w-80 overflow-y-auto rounded-md border border-zinc-200 bg-white py-1 shadow-lg">
        <button v-for="c in components" :key="c.id" type="button" class="block w-full px-3 py-1.5 text-left hover:bg-zinc-50" @click="pick(c)">
          <span class="text-sm text-zinc-900">{{ c.name }}</span>
          <span class="block text-xs text-zinc-500">{{ c.description }}</span>
        </button>
        <button type="button" class="block w-full border-t border-zinc-100 px-3 py-1.5 text-left hover:bg-zinc-50" @click="pick(null)">
          <span class="text-sm text-zinc-900">HTML snippet</span>
          <span class="block text-xs text-zinc-500">Any markup or template code.</span>
        </button>
      </div>
    </div>
  </div>
</template>
