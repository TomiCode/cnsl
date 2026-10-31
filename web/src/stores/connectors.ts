import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api, type Schemas } from '@/api/client'
import { unwrap } from './devices'

export type ConnectorType = Schemas['ConnectorType']
export type Connector = Schemas['Connector']
export type Field = Schemas['ConnectorField']

export const useConnectorsStore = defineStore('connectors', () => {
  const types = ref<ConnectorType[]>([])
  const connectors = ref<Connector[]>([])
  const loaded = ref(false)

  async function load() {
    const [t, c] = await Promise.all([api.GET('/connector-types'), api.GET('/connectors')])
    types.value = unwrap(t)
    connectors.value = unwrap(c)
    loaded.value = true
  }

  async function create(type: string, name: string, config: Record<string, unknown>) {
    const c = unwrap(await api.POST('/connectors', { body: { type, name, config } }))
    await load()
    return c
  }

  async function update(id: number, name: string, config: Record<string, unknown>) {
    const c = unwrap(await api.PUT('/connectors/{id}', { params: { path: { id } }, body: { name, config } }))
    await load()
    return c
  }

  async function remove(id: number) {
    const res = await api.DELETE('/connectors/{id}', { params: { path: { id } } })
    if (res.error) unwrap(res)
    connectors.value = connectors.value.filter((c) => c.id !== id)
  }

  async function test(id: number, params: Record<string, unknown>) {
    return unwrap(await api.POST('/connectors/{id}/test', { params: { path: { id } }, body: { params } }))
  }

  function typeOf(key: string | undefined) {
    return types.value.find((t) => t.key === key)
  }

  function byId(id: number) {
    return connectors.value.find((c) => c.id === id)
  }

  return { types, connectors, loaded, load, create, update, remove, test, typeOf, byId }
})

// Default values for a schema (used when creating something new).
export function defaults(fields: Field[]): Record<string, unknown> {
  return Object.fromEntries(fields.filter((f) => f.default !== undefined).map((f) => [f.key, f.default]))
}
