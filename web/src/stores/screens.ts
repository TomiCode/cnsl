import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api, type Schemas } from '@/api/client'
import { unwrap } from './devices'
import { useEventsStore } from './events'

export type ScreenSummary = Schemas['ScreenSummary']
export type Screen = Schemas['Screen']

export const useScreensStore = defineStore('screens', () => {
  const screens = ref<ScreenSummary[]>([])
  const loaded = ref(false)

  async function load() {
    screens.value = unwrap(await api.GET('/screens'))
    loaded.value = true
  }

  async function create(name: string, modelId: number, kind: Schemas['ScreenKind'] = 'template') {
    const sc = unwrap(await api.POST('/screens', { body: { name, model_id: modelId, kind } }))
    await load()
    return sc
  }

  async function remove(id: number) {
    const res = await api.DELETE('/screens/{id}', { params: { path: { id } } })
    if (res.error) unwrap(res)
    screens.value = screens.value.filter((s) => s.id !== id)
  }

  useEventsStore().subscribe((e) => {
    if (loaded.value && (e.type === 'screen.updated' || e.type === 'screen.rendered')) void load()
  })

  return { screens, loaded, load, create, remove }
})
