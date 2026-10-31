import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api, type Schemas } from '@/api/client'
import { ApiError, unwrap } from './devices'

export type Asset = Schemas['Asset']

export const useAssetsStore = defineStore('assets', () => {
  const assets = ref<Asset[]>([])
  const loaded = ref(false)

  async function load() {
    assets.value = unwrap(await api.GET('/assets'))
    loaded.value = true
  }

  async function upload(file: File) {
    const form = new FormData()
    form.append('file', file)
    // openapi-fetch would JSON-encode the body; send multipart ourselves.
    const res = await fetch('/api/v1/assets', { method: 'POST', body: form })
    const data = await res.json()
    if (!res.ok) throw new ApiError((data as Schemas['Problem']).detail ?? 'Upload failed')
    const asset = data as Asset
    if (!assets.value.some((a) => a.id === asset.id)) assets.value.unshift(asset)
    return asset
  }

  async function remove(id: number) {
    const res = await api.DELETE('/assets/{id}', { params: { path: { id } } })
    if (res.error) unwrap(res)
    assets.value = assets.value.filter((a) => a.id !== id)
  }

  async function preview(id: number, modelId: number, fit: Schemas['ImageFit'], dither: Schemas['ImageDither']) {
    return unwrap(await api.POST('/assets/{id}/preview', { params: { path: { id } }, body: { model_id: modelId, fit, dither } })).device
  }

  function byId(id: number) {
    return assets.value.find((a) => a.id === id)
  }

  return { assets, loaded, load, upload, remove, preview, byId }
})
