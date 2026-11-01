import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api, type Schemas } from '@/api/client'
import { setComponents, setIcons, setSnippets } from '@/lib/monaco'
import { unwrap } from './devices'

export type Component = Schemas['Component']
export type ComponentInput = Schemas['ComponentInput']
export type ComponentParam = Schemas['ComponentParam']
export type Snippet = Schemas['Snippet']
export type SnippetInput = Schemas['SnippetInput']
export type IconInfo = Schemas['IconInfo']

export const useLibraryStore = defineStore('library', () => {
  const components = ref<Component[]>([])
  const snippets = ref<Snippet[]>([])
  const icons = ref<IconInfo[]>([])
  const loaded = ref(false)

  // The editor's completions follow the library.
  function sync() {
    setComponents(components.value)
    setSnippets(snippets.value)
  }

  async function load() {
    const [c, s, i] = await Promise.all([api.GET('/components'), api.GET('/snippets'), icons.value.length ? null : api.GET('/icons')])
    components.value = unwrap(c)
    snippets.value = unwrap(s)
    if (i) {
      icons.value = unwrap(i)
      setIcons(icons.value)
    }
    loaded.value = true
    sync()
  }

  async function ensure() {
    if (!loaded.value) await load()
  }

  async function createComponent(body: ComponentInput) {
    const res = await api.POST('/components', { body })
    if (!res.error) await load()
    return res
  }

  async function updateComponent(id: number, body: ComponentInput) {
    const res = await api.PUT('/components/{id}', { params: { path: { id } }, body })
    if (!res.error) await load()
    return res
  }

  async function duplicateComponent(id: number) {
    const c = unwrap(await api.POST('/components/{id}/duplicate', { params: { path: { id } } }))
    await load()
    return c
  }

  async function removeComponent(id: number) {
    const res = await api.DELETE('/components/{id}', { params: { path: { id } } })
    if (res.error) unwrap(res)
    await load()
  }

  async function saveSnippet(id: number | null, body: SnippetInput) {
    const s = unwrap(id ? await api.PUT('/snippets/{id}', { params: { path: { id } }, body }) : await api.POST('/snippets', { body }))
    await load()
    return s
  }

  async function removeSnippet(id: number) {
    const res = await api.DELETE('/snippets/{id}', { params: { path: { id } } })
    if (res.error) unwrap(res)
    await load()
  }

  return { components, snippets, icons, loaded, load, ensure, createComponent, updateComponent, duplicateComponent, removeComponent, saveSnippet, removeSnippet }
})

// callFor builds a {{ component … }} call. args maps data inputs to template expressions; other
// inputs keep their defaults. A lone first data input is passed positionally.
export function callFor(c: Component, args: Record<string, string>): string {
  const data = c.inputs.filter((i) => i.kind === 'data')
  const key = JSON.stringify(c.key)
  if (!data.length) return `{{ component ${key} }}`
  if (data.length === 1 && c.inputs[0]?.kind === 'data') return `{{ component ${key} ${args[data[0]!.name] ?? '.Data'} }}`
  return `{{ component ${key} ${data.map((i) => `${JSON.stringify(i.name)} ${args[i.name] ?? '.Data'}`).join(' ')} }}`
}
