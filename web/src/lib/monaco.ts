import * as monaco from 'monaco-editor'
import EditorWorker from 'monaco-editor/editor/editor.worker?worker'

// Only the core editor worker is needed: the template language below is a Monarch grammar,
// not one of Monaco's worker-backed languages.
self.MonacoEnvironment = { getWorker: () => new EditorWorker() }

export const LANGUAGE = 'gotmpl-html'
export const THEME = 'cnsl'

const keywords = [
  'if', 'else', 'end', 'range', 'with', 'define', 'template', 'block', 'break', 'continue',
  'and', 'or', 'not', 'eq', 'ne', 'lt', 'le', 'gt', 'ge', 'len', 'index', 'slice',
  'print', 'printf', 'println', 'html', 'js', 'urlquery', 'call',
]

// Fields of the template context (internal/tmpl.Context).
const contextFields: { label: string; detail: string }[] = [
  { label: '.Now', detail: 'time.Time — current time in the household timezone' },
  { label: '.Screen.Name', detail: 'string' },
  { label: '.Screen.Width', detail: 'int — device pixels' },
  { label: '.Screen.Height', detail: 'int — device pixels' },
  { label: '.Screen.BitDepth', detail: 'int — 1, 2 or 4' },
  { label: '.Region.Key', detail: 'string' },
  { label: '.Region.Width', detail: 'int — device pixels' },
  { label: '.Region.Height', detail: 'int — device pixels' },
  { label: '.Data', detail: 'map — connector data by binding alias (phase 4)' },
  { label: '.Device.Name', detail: 'string — only when one device shows the screen' },
]

// Snippets and components come from the library (stores/library.ts keeps them current).
interface SnippetDef {
  prefix: string
  name: string
  description: string
  body: string
}
interface ComponentDef {
  key: string
  name: string
  description: string
  inputs: { name: string; kind: string; label?: string; help?: string }[]
}
let icons: { name: string; set: string }[] = []
export function setIcons(list: { name: string; set: string }[]) {
  icons = list
}
let snippets: SnippetDef[] = []
let components: ComponentDef[] = []
export function setSnippets(list: SnippetDef[]) {
  snippets = list
}
export function setComponents(list: ComponentDef[]) {
  components = list
}

let registered = false

// .Data paths from the screen's current binding data (see lib/datapaths.ts), updated by the editor.
let dataFields: { label: string; detail: string }[] = []
export function setDataFields(fields: { label: string; detail: string }[]) {
  dataFields = fields
}

export function setupMonaco(functions: string[]) {
  if (registered) return monaco
  registered = true

  monaco.languages.register({ id: LANGUAGE })
  monaco.languages.setLanguageConfiguration(LANGUAGE, {
    comments: { blockComment: ['<!--', '-->'] },
    brackets: [
      ['{{', '}}'],
      ['<', '>'],
      ['(', ')'],
    ],
    autoClosingPairs: [
      { open: '{{', close: '}}' },
      { open: '"', close: '"' },
      { open: '(', close: ')' },
    ],
  })
  monaco.languages.setMonarchTokensProvider(LANGUAGE, {
    keywords,
    functions,
    tokenizer: {
      root: [
        [/\{\{-?/, { token: 'delimiter.template', next: '@template' }],
        [/<!--/, 'comment', '@comment'],
        [/(<\/?)([\w-]+)/, ['delimiter.html', { token: 'tag', next: '@tag' }]],
        [/[^<{]+/, ''],
        [/[<{]/, ''],
      ],
      comment: [
        [/-->/, 'comment', '@pop'],
        [/[^-]+/, 'comment'],
        [/./, 'comment'],
      ],
      tag: [
        [/\{\{-?/, { token: 'delimiter.template', next: '@template' }],
        [/\/?>/, 'delimiter.html', '@pop'],
        [/"/, 'attribute.value', '@attrDouble'],
        [/'/, 'attribute.value', '@attrSingle'],
        [/[\w\-:@.]+/, 'attribute.name'],
        [/=/, 'delimiter'],
        [/\s+/, ''],
      ],
      attrDouble: [
        [/\{\{-?/, { token: 'delimiter.template', next: '@template' }],
        [/[^"{]+/, 'attribute.value'],
        [/"/, 'attribute.value', '@pop'],
        [/\{/, 'attribute.value'],
      ],
      attrSingle: [
        [/\{\{-?/, { token: 'delimiter.template', next: '@template' }],
        [/[^'{]+/, 'attribute.value'],
        [/'/, 'attribute.value', '@pop'],
        [/\{/, 'attribute.value'],
      ],
      template: [
        [/-?\}\}/, { token: 'delimiter.template', next: '@pop' }],
        [/\/\*/, 'comment', '@templateComment'],
        [/\$\w*/, 'variable'],
        [/(\.[A-Za-z_]\w*)+|\./, 'variable.field'],
        [/"([^"\\]|\\.)*"/, 'string'],
        [/`[^`]*`/, 'string'],
        [/-?\d+(\.\d+)?/, 'number'],
        [/:=|=|\|/, 'operator'],
        [/[A-Za-z_]\w*/, { cases: { '@keywords': 'keyword', '@functions': 'function', '@default': 'identifier' } }],
        [/[()]/, '@brackets'],
        [/\s+/, ''],
      ],
      templateComment: [
        [/\*\//, 'comment', '@pop'],
        [/./, 'comment'],
      ],
    },
  } as monaco.languages.IMonarchLanguage)

  monaco.editor.defineTheme(THEME, {
    base: 'vs',
    inherit: true,
    rules: [
      { token: 'delimiter.template', foreground: '7c3aed', fontStyle: 'bold' },
      { token: 'variable.field', foreground: '0369a1' },
      { token: 'variable', foreground: '0369a1', fontStyle: 'italic' },
      { token: 'function', foreground: 'b45309' },
      { token: 'keyword', foreground: '7c3aed' },
      { token: 'tag', foreground: '15803d' },
      { token: 'attribute.name', foreground: '4d7c0f' },
      { token: 'attribute.value', foreground: 'a16207' },
      { token: 'string', foreground: 'a16207' },
    ],
    colors: { 'editor.background': '#ffffff' },
  })

  monaco.languages.registerCompletionItemProvider(LANGUAGE, {
    triggerCharacters: ['.', '{', ' ', '"'],
    provideCompletionItems(model, position) {
      const before = model.getValueInRange({
        startLineNumber: 1,
        startColumn: 1,
        endLineNumber: position.lineNumber,
        endColumn: position.column,
      })
      const inAction = before.lastIndexOf('{{') > before.lastIndexOf('}}')
      const word = model.getWordUntilPosition(position)
      // Include a leading dot chain (".Screen.Na") in the replaced range.
      const line = model.getLineContent(position.lineNumber).slice(0, position.column - 1)
      const dotted = /(\.[\w.]*)$/.exec(line)?.[1]
      const range = {
        startLineNumber: position.lineNumber,
        endLineNumber: position.lineNumber,
        startColumn: dotted ? position.column - dotted.length : word.startColumn,
        endColumn: word.endColumn,
      }
      const K = monaco.languages.CompletionItemKind
      // {{ component "wea|  → component keys.
      const keyPrefix = /component\s+"([\w-]*)$/.exec(line)?.[1]
      if (inAction && keyPrefix !== undefined) {
        const keyRange = { ...range, startColumn: position.column - keyPrefix.length }
        return {
          suggestions: components.map((c) => ({
            label: c.key,
            kind: K.Module,
            detail: c.name,
            documentation: {
              value: [c.description, ...c.inputs.map((i, n) => `- \`${i.name}\` (${i.kind})${n === 0 ? ' — positional' : ''}${i.help ? `: ${i.help}` : ''}`)].join('\n'),
            },
            insertText: c.key,
            range: keyRange,
          })),
        }
      }
      // {{ icon "therm|  → icon names.
      const iconPrefix = /icon\s+"([\w:-]*)$/.exec(line)?.[1]
      if (inAction && iconPrefix !== undefined) {
        const iconRange = { ...range, startColumn: position.column - iconPrefix.length }
        return {
          suggestions: icons.map((i) => ({
            label: i.name,
            kind: K.Color,
            detail: i.set === 'wi' ? 'Weather Icons' : 'Lucide',
            insertText: i.name,
            range: iconRange,
          })),
        }
      }
      if (inAction) {
        return {
          suggestions: [
            ...dataFields.map((f) => ({ label: f.label, kind: K.Variable, detail: f.detail, insertText: f.label, range, sortText: '0' + f.label })),
            ...contextFields.map((f) => ({ label: f.label, kind: K.Field, detail: f.detail, insertText: f.label, range })),
            ...functions.map((f) => ({ label: f, kind: K.Function, detail: 'cnsl helper', insertText: f, range })),
            ...keywords.map((k) => ({ label: k, kind: K.Keyword, insertText: k, range })),
          ],
        }
      }
      return {
        suggestions: snippets.map((s) => ({
          label: s.prefix,
          kind: K.Snippet,
          detail: s.name,
          documentation: s.description,
          insertText: s.body,
          insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          range: { ...range, startColumn: word.startColumn },
        })),
      }
    },
  })
  return monaco
}

export interface TemplateError {
  region: string
  line: number
  column: number
  message: string
}

export function setErrorMarkers(model: monaco.editor.ITextModel, errors: TemplateError[]) {
  monaco.editor.setModelMarkers(
    model,
    'cnsl',
    errors.map((e) => {
      const line = Math.max(1, Math.min(e.line || 1, model.getLineCount()))
      return {
        severity: monaco.MarkerSeverity.Error,
        message: e.message,
        startLineNumber: line,
        startColumn: e.column || 1,
        endLineNumber: line,
        endColumn: e.column ? e.column + 1 : model.getLineMaxColumn(line),
      }
    }),
  )
}

export { monaco }
