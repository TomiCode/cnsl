import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { api, type Schemas } from '@/api/client'
import { useEventsStore } from './events'

export type Device = Schemas['Device']
export type DeviceModel = Schemas['DeviceModel']
export type DeviceUpdate = Schemas['DeviceUpdate']

export class ApiError extends Error {}

function problemMessage(error: unknown): string {
  const p = error as Schemas['Problem'] | undefined
  if (p?.errors?.length) return p.errors.map((e) => (e.field ? `${e.field}: ${e.message}` : e.message)).join('; ')
  return p?.detail ?? p?.title ?? 'Request failed'
}

// unwrap turns an openapi-fetch result into data or a thrown ApiError.
export function unwrap<T>(result: { data?: T; error?: unknown }): T {
  if (result.error !== undefined) throw new ApiError(problemMessage(result.error))
  return result.data as T
}

export const useDevicesStore = defineStore('devices', () => {
  const devices = ref<Device[]>([])
  const models = ref<DeviceModel[]>([])
  const loaded = ref(false)

  const pending = computed(() => devices.value.filter((d) => d.status === 'pending'))
  const registered = computed(() => devices.value.filter((d) => d.status !== 'pending'))

  async function load() {
    const [list, modelList] = await Promise.all([api.GET('/devices'), api.GET('/device-models')])
    devices.value = unwrap(list)
    models.value = unwrap(modelList)
    loaded.value = true
  }

  function byId(id: number) {
    return devices.value.find((d) => d.id === id)
  }

  function modelOf(device: Device) {
    return models.value.find((m) => m.id === device.model_id)
  }

  function replace(device: Device) {
    const i = devices.value.findIndex((d) => d.id === device.id)
    if (i >= 0) devices.value[i] = device
    else devices.value.push(device)
    return device
  }

  async function refreshOne(id: number) {
    const res = await api.GET('/devices/{id}', { params: { path: { id } } })
    if (res.response.status === 404) {
      devices.value = devices.value.filter((d) => d.id !== id)
      return
    }
    replace(unwrap(res))
  }

  async function update(id: number, body: DeviceUpdate) {
    return replace(unwrap(await api.PATCH('/devices/{id}', { params: { path: { id } }, body })))
  }

  async function approve(id: number) {
    return replace(unwrap(await api.POST('/devices/{id}/approve', { params: { path: { id } } })))
  }

  async function disable(id: number) {
    return replace(unwrap(await api.POST('/devices/{id}/disable', { params: { path: { id } } })))
  }

  async function remove(id: number) {
    const res = await api.DELETE('/devices/{id}', { params: { path: { id } } })
    if (res.error) throw new ApiError(problemMessage(res.error))
    devices.value = devices.value.filter((d) => d.id !== id)
  }

  async function setPlaylist(id: number, playlistId: number | null) {
    return replace(unwrap(await api.PUT('/devices/{id}/playlist', { params: { path: { id } }, body: { playlist_id: playlistId } })))
  }

  async function startCalibration(id: number) {
    return replace(unwrap(await api.POST('/devices/{id}/calibration', { params: { path: { id } } })))
  }

  async function stopCalibration(id: number) {
    return replace(unwrap(await api.DELETE('/devices/{id}/calibration', { params: { path: { id } } })))
  }

  async function setToneCurve(modelId: number, curve: Schemas['ToneCurve']) {
    const m = unwrap(await api.PUT('/device-models/{id}/tone-curve', { params: { path: { id: modelId } }, body: curve }))
    const i = models.value.findIndex((x) => x.id === m.id)
    if (i >= 0) models.value[i] = m
    return m
  }

  // Keep the list live: refetch the affected device on every server event.
  useEventsStore().subscribe((e) => {
    if (!loaded.value) return
    if (e.type === 'model.updated') {
      void load()
    } else if (!e.type.startsWith('device.')) {
      return
    } else if (e.device_id == null) {
      void load() // reconnect: we may have missed events
    } else if (e.type === 'device.deleted') {
      devices.value = devices.value.filter((d) => d.id !== e.device_id)
    } else if (e.type !== 'device.logs') {
      void refreshOne(e.device_id)
    }
  })

  return {
    devices,
    models,
    loaded,
    pending,
    registered,
    load,
    byId,
    modelOf,
    refreshOne,
    update,
    approve,
    disable,
    remove,
    setPlaylist,
    startCalibration,
    stopCalibration,
    setToneCurve,
  }
})
