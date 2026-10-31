<script setup lang="ts">
import type { Field } from '@/stores/connectors'

// Renders a connector config or binding params form from the server-provided field schema.
const props = defineProps<{
  fields: Field[]
  modelValue: Record<string, unknown>
  secretsSet?: string[]
}>()
const emit = defineEmits<{ 'update:modelValue': [value: Record<string, unknown>] }>()

function set(key: string, value: unknown) {
  emit('update:modelValue', { ...props.modelValue, [key]: value })
}

// Lists are edited as text, one item per line.
function listText(v: unknown) {
  return Array.isArray(v) ? v.join('\n') : typeof v === 'string' ? v : ''
}

const input = 'w-full rounded-md border border-zinc-300 bg-white px-2.5 py-1.5 text-sm focus:border-zinc-500 focus:outline-none'
</script>

<template>
  <div class="space-y-3">
    <label v-for="f in fields" :key="f.key" class="block text-sm">
      <span class="mb-1 flex items-center gap-1 text-zinc-600">
        {{ f.label }}<span v-if="f.required" class="text-red-500">*</span>
      </span>

      <textarea
        v-if="f.kind === 'list' || f.kind === 'text'"
        :value="listText(modelValue[f.key])"
        rows="4"
        :class="[input, 'font-mono text-xs']"
        @input="set(f.key, ($event.target as HTMLTextAreaElement).value)"
      />
      <textarea
        v-else-if="f.kind === 'secret' && f.key === 'headers'"
        :value="(modelValue[f.key] as string) ?? ''"
        rows="3"
        :placeholder="secretsSet?.includes(f.key) ? '•••••• (stored — leave empty to keep)' : ''"
        :class="[input, 'font-mono text-xs']"
        @input="set(f.key, ($event.target as HTMLTextAreaElement).value)"
      />
      <input
        v-else-if="f.kind === 'secret'"
        type="password"
        autocomplete="new-password"
        :value="(modelValue[f.key] as string) ?? ''"
        :placeholder="secretsSet?.includes(f.key) ? '•••••• (stored — leave empty to keep)' : ''"
        :class="input"
        @input="set(f.key, ($event.target as HTMLInputElement).value)"
      />
      <select
        v-else-if="f.kind === 'select'"
        :value="modelValue[f.key] ?? f.default"
        :class="input"
        @change="set(f.key, ($event.target as HTMLSelectElement).value)"
      >
        <option v-for="o in f.options ?? []" :key="o" :value="o">{{ o }}</option>
      </select>
      <input
        v-else-if="f.kind === 'bool'"
        type="checkbox"
        :checked="!!modelValue[f.key]"
        @change="set(f.key, ($event.target as HTMLInputElement).checked)"
      />
      <input
        v-else-if="f.kind === 'int' || f.kind === 'number'"
        type="number"
        :step="f.kind === 'int' ? 1 : 'any'"
        :value="modelValue[f.key] ?? ''"
        :class="input"
        @input="set(f.key, ($event.target as HTMLInputElement).value === '' ? undefined : Number(($event.target as HTMLInputElement).value))"
      />
      <input
        v-else
        :type="f.kind === 'url' ? 'url' : 'text'"
        :value="(modelValue[f.key] as string) ?? ''"
        :class="input"
        @input="set(f.key, ($event.target as HTMLInputElement).value)"
      />
      <span v-if="f.help" class="mt-1 block text-xs text-zinc-400">{{ f.help }}</span>
    </label>
  </div>
</template>
