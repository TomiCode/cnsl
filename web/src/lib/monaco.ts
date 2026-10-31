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

// Built-in snippets until the snippet library lands (phase 5).
const snippets: { label: string; detail: string; body: string }[] = [
  {
    label: 'view',
    detail: 'Full-screen view with layout and title bar',
    body: [
      '<div class="view view--full">',
      '  <div class="layout layout--col layout--center gap--large">',
      '    $0',
      '  </div>',
      '  <div class="title_bar">',
      '    <span class="title">${1:{{ .Screen.Name }}}</span>',
      '  </div>',
      '</div>',
    ].join('\n'),
  },
  { label: 'title_bar', detail: 'Framework title bar', body: '<div class="title_bar">\n  <span class="title">$1</span>\n  <span class="instance">$2</span>\n</div>' },
  { label: 'item', detail: 'Framework item (meta + content)', body: '<div class="item">\n  <div class="meta"></div>\n  <div class="content">\n    <span class="title title--small">$1</span>\n    <span class="description">$2</span>\n  </div>\n</div>' },
  { label: 'value', detail: 'Big value with label', body: '<span class="value value--xlarge value--tnums">$1</span>\n<span class="label">$2</span>' },
  { label: 'columns', detail: 'Two columns', body: '<div class="columns">\n  <div class="column">$1</div>\n  <div class="column">$2</div>\n</div>' },
  { label: 'range', detail: 'Go template range', body: '{{ range ${1:.Data.items} }}\n  $0\n{{ end }}' },
  { label: 'if', detail: 'Go template if', body: '{{ if ${1:condition} }}\n  $0\n{{ end }}' },
  { label: 'date', detail: 'Format the current time', body: '{{ date "${1:Monday, 2 January}" .Now }}' },
]

let registered = false

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
    triggerCharacters: ['.', '{', ' '],
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
      if (inAction) {
        return {
          suggestions: [
            ...contextFields.map((f) => ({ label: f.label, kind: K.Field, detail: f.detail, insertText: f.label, range })),
            ...functions.map((f) => ({ label: f, kind: K.Function, detail: 'cnsl helper', insertText: f, range })),
            ...keywords.map((k) => ({ label: k, kind: K.Keyword, insertText: k, range })),
          ],
        }
      }
      return {
        suggestions: snippets.map((s) => ({
          label: s.label,
          kind: K.Snippet,
          detail: s.detail,
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
