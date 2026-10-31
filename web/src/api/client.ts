import createClient from 'openapi-fetch'
import type { components, paths } from './schema'

// Typed client generated from api/openapi.yaml (run `yarn gen:api` after changing the spec).
export const api = createClient<paths>({ baseUrl: '/api/v1' })

export type Schemas = components['schemas']
