import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api, type Schemas } from '@/api/client'

export const useSystemStore = defineStore('system', () => {
  const health = ref<Schemas['Health'] | null>(null)
  const reachable = ref(true)

  async function refreshHealth() {
    try {
      const { data, error } = await api.GET('/health')
      health.value = data ?? (error as Schemas['Health'] | undefined) ?? null
      reachable.value = true
    } catch {
      health.value = null
      reachable.value = false
    }
  }

  return { health, reachable, refreshHealth }
})
