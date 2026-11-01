import type { Schemas } from '@/api/client'

export type Block = Schemas['Block']
export type BlockArg = Schemas['BlockArg']
type Component = Schemas['Component']

// blocksToHTML mirrors render.BlocksTemplate for "Convert to HTML"; the server generates the
// template that is actually rendered.
export function blocksToHTML(blocks: Block[], components: Component[]): string {
  return blocks
    .map((b, i) => {
      const inner = b.component ? componentCall(b, components) : (b.html ?? '')
      return `<div class="cnsl-block" data-cnsl-block="${i}">${inner}</div>\n`
    })
    .join('')
}

function componentCall(b: Block, components: Component[]): string {
  const c = components.find((x) => x.key === b.component)
  const parts = [`{{ component ${JSON.stringify(b.component)}`]
  for (const input of c?.inputs ?? []) {
    const arg = b.args?.[input.name]
    const expr = arg?.path ? arg.path : literal(arg?.value, input.kind)
    if (expr) parts.push(`${JSON.stringify(input.name)} ${expr}`)
  }
  return parts.join(' ') + ' }}'
}

function literal(v: unknown, kind: string): string {
  if (v === undefined || v === null || v === '') return ''
  if (typeof v === 'boolean' || typeof v === 'number') return String(v)
  return kind === 'number' ? String(v) : JSON.stringify(String(v))
}

// A component block with its data inputs filled in from expressions like ".Data.ha.sensor_x".
export function componentBlock(c: Component, paths: Record<string, string>): Block {
  const args: Record<string, BlockArg> = {}
  for (const [name, path] of Object.entries(paths)) args[name] = { path }
  return { component: c.key, args }
}
