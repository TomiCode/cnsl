<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'
import { LANGUAGE, THEME, monaco, setErrorMarkers, setupMonaco, type TemplateError } from '@/lib/monaco'

const props = defineProps<{
  modelValue: string
  functions: string[]
  errors?: TemplateError[]
}>()
const emit = defineEmits<{
  'update:modelValue': [value: string]
  save: []
  preview: []
}>()

const el = ref<HTMLElement | null>(null)
const editor = shallowRef<monaco.editor.IStandaloneCodeEditor | null>(null)

onMounted(() => {
  setupMonaco(props.functions)
  const ed = monaco.editor.create(el.value!, {
    value: props.modelValue,
    language: LANGUAGE,
    theme: THEME,
    automaticLayout: true,
    minimap: { enabled: false },
    fontSize: 13,
    tabSize: 2,
    wordWrap: 'on',
    scrollBeyondLastLine: false,
    renderWhitespace: 'boundary',
    fixedOverflowWidgets: true,
  })
  ed.onDidChangeModelContent(() => emit('update:modelValue', ed.getValue()))
  ed.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => emit('save'))
  ed.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter, () => emit('preview'))
  editor.value = ed
  if (props.errors) setErrorMarkers(ed.getModel()!, props.errors)
})

// External changes (e.g. switching regions or reloading) replace the content.
watch(
  () => props.modelValue,
  (v) => {
    const ed = editor.value
    if (ed && v !== ed.getValue()) ed.setValue(v)
  },
)
watch(
  () => props.errors,
  (errs) => {
    const model = editor.value?.getModel()
    if (model) setErrorMarkers(model, errs ?? [])
  },
  { deep: true },
)

onBeforeUnmount(() => editor.value?.dispose())

defineExpose({
  revealLine(line: number) {
    editor.value?.revealLineInCenter(line)
    editor.value?.setPosition({ lineNumber: line, column: 1 })
    editor.value?.focus()
  },
})
</script>

<template>
  <div ref="el" class="size-full min-h-0" />
</template>
