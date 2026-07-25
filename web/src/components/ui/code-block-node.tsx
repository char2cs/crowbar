'use client'

import * as React from 'react'

import { formatCodeBlock, isLangSupported } from '@platejs/code-block'
import { Command as CommandPrimitive } from 'cmdk'
import { BracesIcon, Check, CheckIcon, CopyIcon, SearchIcon } from 'lucide-react'
import { type TCodeBlockElement, type TCodeSyntaxLeaf, NodeApi } from 'platejs'
import { type PlateElementProps, type PlateLeafProps, PlateElement, PlateLeaf } from 'platejs/react'
import { useEditorRef, useElement, useReadOnly } from 'platejs/react'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

// This file is `components/ui` only by shadcn-registry convention — it is
// already Plate/markdown-specific (it imports `platejs` types and reads
// `element.lang` for the language combobox below), not a generic reusable
// primitive, so reaching into the markdown feature for the one extra
// language case (`mermaid`) keeps all of this element's per-language
// branching colocated rather than split across layers.
import { MermaidCodeBlock } from '@/features/editor/markdown/plate/mermaid-code-block'

// `@/components/ui/command` wraps this app's own base-ui `Autocomplete`
// (not `cmdk`), and `@/components/ui/popover` (base-ui) has no
// `onCloseAutoFocus` hook. This file talks to `cmdk` and
// `@radix-ui/react-popover` directly instead, mirroring the shape of the
// (skipped) registry `command.tsx`/`popover.tsx` so it stays self-contained.
import * as PopoverPrimitive from '@radix-ui/react-popover'

const Popover = PopoverPrimitive.Root
const PopoverTrigger = PopoverPrimitive.Trigger

function PopoverContent({
  className,
  sideOffset = 4,
  ...props
}: React.ComponentProps<typeof PopoverPrimitive.Content>) {
  return (
    <PopoverPrimitive.Portal>
      <PopoverPrimitive.Content
        className={cn(
          'z-50 rounded-md border bg-popover text-popover-foreground shadow-md outline-hidden',
          className,
        )}
        sideOffset={sideOffset}
        {...props}
      />
    </PopoverPrimitive.Portal>
  )
}

function Command({ className, ...props }: React.ComponentProps<typeof CommandPrimitive>) {
  return (
    <CommandPrimitive
      className={cn(
        'flex size-full flex-col overflow-hidden rounded-md bg-popover text-popover-foreground',
        className,
      )}
      {...props}
    />
  )
}

function CommandInput({
  className,
  ...props
}: React.ComponentProps<typeof CommandPrimitive.Input>) {
  return (
    <div className="flex items-center gap-2 border-b px-3" data-slot="command-input-wrapper">
      <SearchIcon className="size-4 shrink-0 opacity-50" />
      <CommandPrimitive.Input
        className={cn(
          'flex w-full rounded-md bg-transparent py-3 text-sm outline-hidden placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-50',
          className,
        )}
        {...props}
      />
    </div>
  )
}

function CommandList({ className, ...props }: React.ComponentProps<typeof CommandPrimitive.List>) {
  return (
    <CommandPrimitive.List
      className={cn('overflow-x-hidden overflow-y-auto', className)}
      {...props}
    />
  )
}

function CommandEmpty(props: React.ComponentProps<typeof CommandPrimitive.Empty>) {
  return <CommandPrimitive.Empty className="py-6 text-center text-sm" {...props} />
}

function CommandGroup({
  className,
  ...props
}: React.ComponentProps<typeof CommandPrimitive.Group>) {
  return (
    <CommandPrimitive.Group
      className={cn(
        'overflow-hidden p-1 text-foreground [&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:py-1.5 [&_[cmdk-group-heading]]:text-xs [&_[cmdk-group-heading]]:font-medium [&_[cmdk-group-heading]]:text-muted-foreground',
        className,
      )}
      {...props}
    />
  )
}

function CommandItem({ className, ...props }: React.ComponentProps<typeof CommandPrimitive.Item>) {
  return (
    <CommandPrimitive.Item
      className={cn(
        "relative flex cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-hidden select-none data-[disabled=true]:pointer-events-none data-[disabled=true]:opacity-50 data-[selected=true]:bg-accent data-[selected=true]:text-accent-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
        className,
      )}
      {...props}
    />
  )
}

type CodeBlockElementProps = PlateElementProps<TCodeBlockElement> & {
  showLanguageLabel?: boolean
}

const codeBlockLanguages: { label: string; value: string }[] = [
  { label: 'Auto', value: 'auto' },
  { label: 'Plain Text', value: 'plaintext' },
  { label: 'ABAP', value: 'abap' },
  { label: 'Agda', value: 'agda' },
  { label: 'Arduino', value: 'arduino' },
  { label: 'ASCII Art', value: 'ascii' },
  { label: 'Assembly', value: 'x86asm' },
  { label: 'Bash', value: 'bash' },
  { label: 'BASIC', value: 'basic' },
  { label: 'BNF', value: 'bnf' },
  { label: 'C', value: 'c' },
  { label: 'C#', value: 'csharp' },
  { label: 'C++', value: 'cpp' },
  { label: 'Clojure', value: 'clojure' },
  { label: 'CoffeeScript', value: 'coffeescript' },
  { label: 'Coq', value: 'coq' },
  { label: 'CSS', value: 'css' },
  { label: 'Dart', value: 'dart' },
  { label: 'Dhall', value: 'dhall' },
  { label: 'Diff', value: 'diff' },
  { label: 'Docker', value: 'dockerfile' },
  { label: 'EBNF', value: 'ebnf' },
  { label: 'Elixir', value: 'elixir' },
  { label: 'Elm', value: 'elm' },
  { label: 'Erlang', value: 'erlang' },
  { label: 'F#', value: 'fsharp' },
  { label: 'Flow', value: 'flow' },
  { label: 'Fortran', value: 'fortran' },
  { label: 'Gherkin', value: 'gherkin' },
  { label: 'GLSL', value: 'glsl' },
  { label: 'Go', value: 'go' },
  { label: 'GraphQL', value: 'graphql' },
  { label: 'Groovy', value: 'groovy' },
  { label: 'Haskell', value: 'haskell' },
  { label: 'HCL', value: 'hcl' },
  { label: 'HTML', value: 'html' },
  { label: 'Idris', value: 'idris' },
  { label: 'Java', value: 'java' },
  { label: 'JavaScript', value: 'javascript' },
  { label: 'JSON', value: 'json' },
  { label: 'Julia', value: 'julia' },
  { label: 'Kotlin', value: 'kotlin' },
  { label: 'LaTeX', value: 'latex' },
  { label: 'Less', value: 'less' },
  { label: 'Lisp', value: 'lisp' },
  { label: 'LiveScript', value: 'livescript' },
  { label: 'LLVM IR', value: 'llvm' },
  { label: 'Lua', value: 'lua' },
  { label: 'Makefile', value: 'makefile' },
  { label: 'Markdown', value: 'markdown' },
  { label: 'Markup', value: 'markup' },
  { label: 'MATLAB', value: 'matlab' },
  { label: 'Mathematica', value: 'mathematica' },
  { label: 'Mermaid', value: 'mermaid' },
  { label: 'Nix', value: 'nix' },
  { label: 'Notion Formula', value: 'notion' },
  { label: 'Objective-C', value: 'objectivec' },
  { label: 'OCaml', value: 'ocaml' },
  { label: 'Pascal', value: 'pascal' },
  { label: 'Perl', value: 'perl' },
  { label: 'PHP', value: 'php' },
  { label: 'PowerShell', value: 'powershell' },
  { label: 'Prolog', value: 'prolog' },
  { label: 'Protocol Buffers', value: 'protobuf' },
  { label: 'PureScript', value: 'purescript' },
  { label: 'Python', value: 'python' },
  { label: 'R', value: 'r' },
  { label: 'Racket', value: 'racket' },
  { label: 'Reason', value: 'reasonml' },
  { label: 'Ruby', value: 'ruby' },
  { label: 'Rust', value: 'rust' },
  { label: 'Sass', value: 'scss' },
  { label: 'Scala', value: 'scala' },
  { label: 'Scheme', value: 'scheme' },
  { label: 'SCSS', value: 'scss' },
  { label: 'Shell', value: 'shell' },
  { label: 'Smalltalk', value: 'smalltalk' },
  { label: 'Solidity', value: 'solidity' },
  { label: 'SQL', value: 'sql' },
  { label: 'Swift', value: 'swift' },
  { label: 'TOML', value: 'toml' },
  { label: 'TypeScript', value: 'typescript' },
  { label: 'VB.Net', value: 'vbnet' },
  { label: 'Verilog', value: 'verilog' },
  { label: 'VHDL', value: 'vhdl' },
  { label: 'Visual Basic', value: 'vbnet' },
  { label: 'WebAssembly', value: 'wasm' },
  { label: 'XML', value: 'xml' },
  { label: 'YAML', value: 'yaml' },
]

function getCodeBlockLanguageLabel(lang?: string | null) {
  const value = lang?.trim()

  if (!value) return null

  return codeBlockLanguages.find((language) => language.value === value)?.label ?? value
}

export function CodeBlockElement({ showLanguageLabel = true, ...props }: CodeBlockElementProps) {
  const { editor, element } = props
  const isMermaid = element.lang === 'mermaid'

  const codeBody = (
    <pre className="overflow-x-auto p-8 pr-4 font-mono text-sm leading-[normal] [tab-size:2] print:break-inside-avoid">
      <code>{props.children}</code>
    </pre>
  )

  return (
    <PlateElement
      className="py-1 **:[.hljs-addition]:bg-[#f0fff4] **:[.hljs-addition]:text-[#22863a] dark:**:[.hljs-addition]:bg-[#3c5743] dark:**:[.hljs-addition]:text-[#ceead5] **:[.hljs-attr,.hljs-attribute,.hljs-literal,.hljs-meta,.hljs-number,.hljs-operator,.hljs-selector-attr,.hljs-selector-class,.hljs-selector-id,.hljs-variable]:text-[#005cc5] dark:**:[.hljs-attr,.hljs-attribute,.hljs-literal,.hljs-meta,.hljs-number,.hljs-operator,.hljs-selector-attr,.hljs-selector-class,.hljs-selector-id,.hljs-variable]:text-[#6596cf] **:[.hljs-built\\\\_in,.hljs-symbol]:text-[#e36209] dark:**:[.hljs-built\\\\_in,.hljs-symbol]:text-[#c3854e] **:[.hljs-bullet]:text-[#735c0f] **:[.hljs-comment,.hljs-code,.hljs-formula]:text-[#6a737d] dark:**:[.hljs-comment,.hljs-code,.hljs-formula]:text-[#6a737d] **:[.hljs-deletion]:bg-[#ffeef0] **:[.hljs-deletion]:text-[#b31d28] dark:**:[.hljs-deletion]:bg-[#473235] dark:**:[.hljs-deletion]:text-[#e7c7cb] **:[.hljs-emphasis]:italic **:[.hljs-keyword,.hljs-doctag,.hljs-template-tag,.hljs-template-variable,.hljs-type,.hljs-variable.language\\\\_]:text-[#d73a49] dark:**:[.hljs-keyword,.hljs-doctag,.hljs-template-tag,.hljs-template-variable,.hljs-type,.hljs-variable.language\\\\_]:text-[#ee6960] **:[.hljs-name,.hljs-quote,.hljs-selector-tag,.hljs-selector-pseudo]:text-[#22863a] dark:**:[.hljs-name,.hljs-quote,.hljs-selector-tag,.hljs-selector-pseudo]:text-[#36a84f] **:[.hljs-regexp,.hljs-string,.hljs-meta_.hljs-string]:text-[#032f62] dark:**:[.hljs-regexp,.hljs-string,.hljs-meta_.hljs-string]:text-[#3593ff] **:[.hljs-section]:font-bold **:[.hljs-section]:text-[#005cc5] dark:**:[.hljs-section]:text-[#61a5f2] **:[.hljs-strong]:font-bold **:[.hljs-title,.hljs-title.class\\\\_,.hljs-title.class\\\\_.inherited\\\\_\\\\_,.hljs-title.function\\\\_]:text-[#6f42c1] dark:**:[.hljs-title,.hljs-title.class\\\\_,.hljs-title.class\\\\_.inherited\\\\_\\\\_,.hljs-title.function\\\\_]:text-[#a77bfa]"
      {...props}
    >
      <div className="relative rounded-md bg-muted/50">
        {isMermaid ? (
          // The fenced code block stays a plain `code_block` node in the
          // document model — this only changes how it RENDERS. `codeBody`
          // (the actual Plate/Slate-controlled text) is passed through
          // unchanged and stays mounted at all times; see
          // mermaid-code-block.tsx for why.
          //
          // Join the code LINES with `\n`: a code block is `code_block >
          // code_line[]`, and `NodeApi.string(element)` concatenates every
          // line's text with no separator — so `graph TD` and `  A --> B`
          // arrive as one line and mermaid fails to parse. Reconstruct the
          // real multi-line source per line instead.
          <MermaidCodeBlock code={element.children.map((line) => NodeApi.string(line)).join('\n')}>
            {codeBody}
          </MermaidCodeBlock>
        ) : (
          codeBody
        )}

        <div
          className="absolute top-1 right-1 z-10 flex select-none gap-0.5"
          contentEditable={false}
        >
          {isLangSupported(element.lang) && (
            <Button
              size="icon"
              variant="ghost"
              className="size-6 text-xs"
              onClick={() => formatCodeBlock(editor, { element })}
              title="Format code"
            >
              <BracesIcon className="!size-3.5 text-muted-foreground" />
            </Button>
          )}

          <CodeBlockCombobox showLanguageLabel={showLanguageLabel} />

          <CopyButton
            size="icon"
            variant="ghost"
            className="size-6 gap-1 text-muted-foreground text-xs"
            value={() => NodeApi.string(element)}
          />
        </div>
      </div>
    </PlateElement>
  )
}

function CodeBlockCombobox({ showLanguageLabel }: { showLanguageLabel: boolean }) {
  const [open, setOpen] = React.useState(false)
  const readOnly = useReadOnly()
  const editor = useEditorRef()
  const element = useElement<TCodeBlockElement>()
  const value = element.lang || 'plaintext'
  const [searchValue, setSearchValue] = React.useState('')

  const items = React.useMemo(
    () =>
      codeBlockLanguages.filter(
        (language) =>
          !searchValue || language.label.toLowerCase().includes(searchValue.toLowerCase()),
      ),
    [searchValue],
  )

  if (readOnly) {
    if (!showLanguageLabel) return null

    return <CodeBlockLanguageLabel lang={element.lang} />
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          size="sm"
          variant="ghost"
          className="h-6 select-none justify-between gap-1 px-2 text-muted-foreground text-xs"
          aria-expanded={open}
          role="combobox"
        >
          {getCodeBlockLanguageLabel(value) ?? 'Plain Text'}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[200px] p-0" onCloseAutoFocus={() => setSearchValue('')}>
        <Command shouldFilter={false}>
          <CommandInput
            className="h-9"
            value={searchValue}
            onValueChange={(value) => setSearchValue(value)}
            placeholder="Search language..."
          />
          <CommandEmpty>No language found.</CommandEmpty>

          <CommandList className="h-[344px] overflow-y-auto">
            <CommandGroup>
              {items.map((language) => (
                <CommandItem
                  key={language.label}
                  className="cursor-pointer"
                  value={language.value}
                  onSelect={(value) => {
                    editor.tf.setNodes<TCodeBlockElement>({ lang: value }, { at: element })
                    setSearchValue(value)
                    setOpen(false)
                  }}
                >
                  <Check className={cn(value === language.value ? 'opacity-100' : 'opacity-0')} />
                  {language.label}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}

function CodeBlockLanguageLabel({ lang }: { lang?: string | null }) {
  const label = getCodeBlockLanguageLabel(lang)

  if (!label) return null

  return (
    <span className="flex h-6 select-none items-center px-2 text-muted-foreground text-xs">
      {label}
    </span>
  )
}

function CopyButton({
  value,
  ...props
}: { value: (() => string) | string } & Omit<React.ComponentProps<typeof Button>, 'value'>) {
  const [hasCopied, setHasCopied] = React.useState(false)

  React.useEffect(() => {
    const timeoutId = window.setTimeout(() => {
      setHasCopied(false)
    }, 2000)

    return () => {
      window.clearTimeout(timeoutId)
    }
  }, [hasCopied])

  return (
    <Button
      onClick={() => {
        void navigator.clipboard.writeText(typeof value === 'function' ? value() : value)
        setHasCopied(true)
      }}
      {...props}
    >
      <span className="sr-only">Copy</span>
      {hasCopied ? <CheckIcon className="!size-3" /> : <CopyIcon className="!size-3" />}
    </Button>
  )
}

export function CodeLineElement(props: PlateElementProps) {
  return <PlateElement {...props} />
}

export function CodeSyntaxLeaf(props: PlateLeafProps<TCodeSyntaxLeaf>) {
  const tokenClassName = props.leaf.className as string

  return <PlateLeaf className={tokenClassName} {...props} />
}
