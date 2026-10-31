import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api, type Schemas } from '@/api/client'
import { unwrap } from './devices'
import { useEventsStore } from './events'

export type Playlist = Schemas['Playlist']
export type PlaylistInput = Schemas['PlaylistInput']

export const usePlaylistsStore = defineStore('playlists', () => {
  const playlists = ref<Playlist[]>([])
  const loaded = ref(false)

  async function load() {
    playlists.value = unwrap(await api.GET('/playlists'))
    loaded.value = true
  }

  async function create(body: PlaylistInput) {
    const p = unwrap(await api.POST('/playlists', { body }))
    await load()
    return p
  }

  async function update(id: number, body: PlaylistInput) {
    const p = unwrap(await api.PUT('/playlists/{id}', { params: { path: { id } }, body }))
    await load()
    return p
  }

  async function remove(id: number) {
    const res = await api.DELETE('/playlists/{id}', { params: { path: { id } } })
    if (res.error) unwrap(res)
    playlists.value = playlists.value.filter((p) => p.id !== id)
  }

  useEventsStore().subscribe((e) => {
    if (loaded.value && (e.type === 'playlist.updated' || e.type === 'device.updated')) void load()
  })

  return { playlists, loaded, load, create, update, remove }
})
