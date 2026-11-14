'use client'

import * as React from 'react'

import type { Emoji } from '@emoji-mart/data'
import { type EmojiCategoryList, type GridRow, EmojiSettings } from '@platejs/emoji'
import type { UseEmojiPickerType } from '@platejs/emoji/react'

import { cn } from '@/lib/utils'

const EmojiButton = React.memo(function EmojiButton({
  emoji,
  index,
  onMouseOver,
  onSelect,
}: {
  emoji: Emoji
  index: number
  onMouseOver: (emoji?: Emoji) => void
  onSelect: (emoji: Emoji) => void
}) {
  return (
    <button
      className="group relative flex size-9 cursor-pointer items-center justify-center border-none bg-transparent text-2xl leading-none"
      onClick={() => onSelect(emoji)}
      onMouseEnter={() => onMouseOver(emoji)}
      onMouseLeave={() => onMouseOver()}
      aria-label={emoji.skins[0].native}
      data-index={index}
      tabIndex={-1}
      type="button"
    >
      <div
        className="absolute inset-0 rounded-full opacity-0 group-hover:opacity-100"
        aria-hidden="true"
      />
      <span
        className="relative"
        style={{
          fontFamily:
            '"Apple Color Emoji", "Segoe UI Emoji", NotoColorEmoji, "Noto Color Emoji", "Segoe UI Symbol", "Android Emoji", EmojiSymbols',
        }}
        data-emoji-set="native"
      >
        {emoji.skins[0].native}
      </span>
    </button>
  )
})

const RowOfButtons = React.memo(function RowOfButtons({
  emojiLibrary,
  row,
  onMouseOver,
  onSelectEmoji,
}: {
  row: GridRow
} & Pick<UseEmojiPickerType, 'emojiLibrary' | 'onMouseOver' | 'onSelectEmoji'>) {
  return (
    <div key={row.id} className="flex" data-index={row.id}>
      {row.elements.map((emojiId, index) => (
        <EmojiButton
          key={emojiId}
          onMouseOver={onMouseOver}
          onSelect={onSelectEmoji}
          emoji={emojiLibrary.getEmoji(emojiId)}
          index={index}
        />
      ))}
    </div>
  )
})
/** The scrolling emoji grid: every category, or the search's results. */
export function EmojiPickerContent({
  emojiLibrary,
  i18n,
  isSearching = false,
  refs,
  searchResult,
  settings = EmojiSettings,
  visibleCategories,
  onMouseOver,
  onSelectEmoji,
}: Pick<
  UseEmojiPickerType,
  | 'emojiLibrary'
  | 'i18n'
  | 'isSearching'
  | 'onMouseOver'
  | 'onSelectEmoji'
  | 'refs'
  | 'searchResult'
  | 'settings'
  | 'visibleCategories'
>) {
  const getRowWidth = settings.perLine.value * settings.buttonSize.value

  const isCategoryVisible = React.useCallback(
    (categoryId: EmojiCategoryList) =>
      visibleCategories.has(categoryId) ? visibleCategories.get(categoryId) : false,
    [visibleCategories],
  )

  const EmojiList = React.useCallback(
    () =>
      emojiLibrary
        .getGrid()
        .sections()
        .map(({ id: categoryId }) => {
          const section = emojiLibrary.getGrid().section(categoryId)
          const { buttonSize } = settings

          return (
            <div
              key={categoryId}
              ref={section.root}
              style={{ width: getRowWidth }}
              data-id={categoryId}
            >
              <div className="-top-px sticky z-1 bg-popover/90 p-1 py-2 font-semibold text-sm backdrop-blur-xs">
                {i18n.categories[categoryId]}
              </div>
              <div
                className="relative flex flex-wrap"
                style={{ height: section.getRows().length * buttonSize.value }}
              >
                {isCategoryVisible(categoryId) &&
                  section
                    .getRows()
                    .map((row: GridRow) => (
                      <RowOfButtons
                        key={row.id}
                        onMouseOver={onMouseOver}
                        onSelectEmoji={onSelectEmoji}
                        emojiLibrary={emojiLibrary}
                        row={row}
                      />
                    ))}
              </div>
            </div>
          )
        }),
    [
      emojiLibrary,
      getRowWidth,
      i18n.categories,
      isCategoryVisible,
      onSelectEmoji,
      onMouseOver,
      settings,
    ],
  )

  const SearchList = React.useCallback(
    () => (
      <div style={{ width: getRowWidth }} data-id="search">
        <div className="-top-px sticky z-1 bg-popover/90 p-1 py-2 font-semibold text-card-foreground text-sm backdrop-blur-xs">
          {i18n.searchResult}
        </div>
        <div className="relative flex flex-wrap">
          {searchResult.map((emoji: Emoji, index: number) => (
            <EmojiButton
              key={emoji.id}
              onMouseOver={onMouseOver}
              onSelect={onSelectEmoji}
              emoji={emojiLibrary.getEmoji(emoji.id)}
              index={index}
            />
          ))}
        </div>
      </div>
    ),
    [emojiLibrary, getRowWidth, i18n.searchResult, searchResult, onSelectEmoji, onMouseOver],
  )

  return (
    <div
      ref={refs.current.contentRoot}
      className={cn(
        'h-full min-h-[50%] overflow-y-auto overflow-x-hidden px-2',
        '[&::-webkit-scrollbar]:w-4',
        '[&::-webkit-scrollbar-button]:hidden [&::-webkit-scrollbar-button]:size-0',
        '[&::-webkit-scrollbar-thumb]:min-h-11 [&::-webkit-scrollbar-thumb]:rounded-full [&::-webkit-scrollbar-thumb]:bg-muted [&::-webkit-scrollbar-thumb]:hover:bg-muted-foreground/25',
        '[&::-webkit-scrollbar-thumb]:border-4 [&::-webkit-scrollbar-thumb]:border-popover [&::-webkit-scrollbar-thumb]:border-solid [&::-webkit-scrollbar-thumb]:bg-clip-padding',
      )}
      data-id="scroll"
    >
      <div ref={refs.current.content} className="h-full">
        {isSearching ? SearchList() : EmojiList()}
      </div>
    </div>
  )
}
