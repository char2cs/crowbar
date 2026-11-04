'use client'

import * as React from 'react'

import { type EmojiCategoryList, type EmojiIconList, EmojiSettings } from '@platejs/emoji'
import type { UseEmojiPickerType } from '@platejs/emoji/react'
import { Popover } from '@base-ui/react/popover'
import {
  BasketballIcon,
  ClockIcon,
  CompassIcon,
  FlagIcon,
  HamburgerIcon,
  LeafIcon,
  LightbulbIcon,
  MagnifyingGlassIcon,
  MusicNoteIcon,
  SmileyIcon,
  StarIcon,
  XIcon,
  type Icon,
} from '@phosphor-icons/react'

import { Button } from '@/components/ui/button'
import { WithTooltip } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

import { EmojiPickerContent } from './emoji-picker-grid'

export function EmojiPopover({
  children,
  control,
  isOpen,
  setIsOpen,
}: {
  children: React.ReactNode
  control: React.ReactNode
  isOpen: boolean
  setIsOpen: (open: boolean) => void
}) {
  return (
    <Popover.Root open={isOpen} onOpenChange={setIsOpen}>
      {/* `render`: the control is a <Button> (a real <button>); the trigger
          merges onto it rather than wrapping it in a second <button>. */}
      <Popover.Trigger render={control as React.ReactElement} />

      <Popover.Portal>
        <Popover.Positioner className="z-100">
          <Popover.Popup>{children}</Popover.Popup>
        </Popover.Positioner>
      </Popover.Portal>
    </Popover.Root>
  )
}

export function EmojiPicker({
  clearSearch,
  emoji,
  emojiLibrary,
  focusedCategory,
  hasFound,
  i18n,
  icons = {
    categories: emojiCategoryIcons,
    search: emojiSearchIcons,
  },
  isSearching,
  refs,
  searchResult,
  searchValue,
  setSearch,
  settings = EmojiSettings,
  visibleCategories,
  handleCategoryClick,
  onMouseOver,
  onSelectEmoji,
}: Omit<UseEmojiPickerType, 'icons'> & {
  icons?: EmojiIconList<React.ReactElement>
}) {
  return (
    <div
      className={cn(
        'flex flex-col rounded-xl bg-popover text-popover-foreground',
        'h-[23rem] w-80 border shadow-md',
      )}
    >
      <EmojiPickerNavigation
        onClick={handleCategoryClick}
        emojiLibrary={emojiLibrary}
        focusedCategory={focusedCategory}
        i18n={i18n}
        icons={icons}
      />
      <EmojiPickerSearchBar i18n={i18n} searchValue={searchValue} setSearch={setSearch}>
        <EmojiPickerSearchAndClear
          clearSearch={clearSearch}
          i18n={i18n}
          searchValue={searchValue}
        />
      </EmojiPickerSearchBar>
      <EmojiPickerContent
        onMouseOver={onMouseOver}
        onSelectEmoji={onSelectEmoji}
        emojiLibrary={emojiLibrary}
        i18n={i18n}
        isSearching={isSearching}
        refs={refs}
        searchResult={searchResult}
        settings={settings}
        visibleCategories={visibleCategories}
      />
      <EmojiPickerPreview emoji={emoji} hasFound={hasFound} i18n={i18n} isSearching={isSearching} />
    </div>
  )
}
function EmojiPickerSearchBar({
  children,
  i18n,
  searchValue,
  setSearch,
}: {
  children: React.ReactNode
} & Pick<UseEmojiPickerType, 'i18n' | 'searchValue' | 'setSearch'>) {
  return (
    <div className="flex items-center px-2">
      <div className="relative flex grow items-center">
        <input
          className="block w-full appearance-none rounded-full border-0 bg-muted px-10 py-2 text-sm outline-none placeholder:text-muted-foreground focus-visible:outline-none"
          value={searchValue}
          onChange={(event) => setSearch(event.target.value)}
          placeholder={i18n.search}
          aria-label="Search"
          autoComplete="off"
          type="text"
          // react-doctor-disable-next-line no-autofocus -- not a page-load autofocus: this input only mounts inside the emoji popover, which the user opens deliberately from the callout emoji button. Moving focus into the search box is the expected result of that action (identical to a command palette), and it is the popover's only meaningful control. Dropping it would leave keyboard users with focus stranded on the trigger with no way to type a query.
          autoFocus
        />
        {children}
      </div>
    </div>
  )
}

function EmojiPickerSearchAndClear({
  clearSearch,
  i18n,
  searchValue,
}: Pick<UseEmojiPickerType, 'clearSearch' | 'i18n' | 'searchValue'>) {
  return (
    <div className="flex items-center text-foreground">
      <div
        className={cn(
          '-translate-y-1/2 absolute top-1/2 left-2.5 z-10 flex size-5 items-center justify-center text-foreground',
        )}
      >
        {emojiSearchIcons.loupe}
      </div>
      {searchValue && (
        <Button
          size="icon"
          variant="ghost"
          className={cn(
            '-translate-y-1/2 absolute top-1/2 right-0.5 flex size-8 cursor-pointer items-center justify-center rounded-full border-none bg-transparent text-popover-foreground hover:bg-transparent',
          )}
          onClick={clearSearch}
          title={i18n.clear}
          aria-label="Clear"
          type="button"
        >
          {emojiSearchIcons.delete}
        </Button>
      )}
    </div>
  )
}

function EmojiPreview({ emoji }: Pick<UseEmojiPickerType, 'emoji'>) {
  return (
    <div className="flex h-14 max-h-14 min-h-14 items-center border-muted border-t p-2">
      <div className="flex items-center justify-center text-2xl">{emoji?.skins[0].native}</div>
      <div className="overflow-hidden pl-2">
        <div className="truncate font-semibold text-sm">{emoji?.name}</div>
        <div className="truncate text-sm">{`:${emoji?.id}:`}</div>
      </div>
    </div>
  )
}

function NoEmoji({ i18n }: Pick<UseEmojiPickerType, 'i18n'>) {
  return (
    <div className="flex h-14 max-h-14 min-h-14 items-center border-muted border-t p-2">
      <div className="flex items-center justify-center text-2xl">😢</div>
      <div className="overflow-hidden pl-2">
        <div className="truncate font-bold text-sm">{i18n.searchNoResultsTitle}</div>
        <div className="truncate text-sm">{i18n.searchNoResultsSubtitle}</div>
      </div>
    </div>
  )
}

function PickAnEmoji({ i18n }: Pick<UseEmojiPickerType, 'i18n'>) {
  return (
    <div className="flex h-14 max-h-14 min-h-14 items-center border-muted border-t p-2">
      <div className="flex items-center justify-center text-2xl">☝️</div>
      <div className="overflow-hidden pl-2">
        <div className="truncate font-semibold text-sm">{i18n.pick}</div>
      </div>
    </div>
  )
}

function EmojiPickerPreview({
  emoji,
  hasFound = true,
  i18n,
  isSearching = false,
  ...props
}: Pick<UseEmojiPickerType, 'emoji' | 'hasFound' | 'i18n' | 'isSearching'>) {
  const showPickEmoji = !emoji && (!isSearching || hasFound)
  const showNoEmoji = isSearching && !hasFound
  const showPreview = emoji && !showNoEmoji

  return (
    <>
      {showPreview && <EmojiPreview emoji={emoji} {...props} />}
      {showPickEmoji && <PickAnEmoji i18n={i18n} {...props} />}
      {showNoEmoji && <NoEmoji i18n={i18n} {...props} />}
    </>
  )
}

function EmojiPickerNavigation({
  emojiLibrary,
  focusedCategory,
  i18n,
  icons,
  onClick,
}: {
  onClick: (id: EmojiCategoryList) => void
} & Pick<UseEmojiPickerType, 'emojiLibrary' | 'focusedCategory' | 'i18n' | 'icons'>) {
  return (
    <nav id="emoji-nav" className="mb-2.5 border-0 border-b border-b-border border-solid p-1.5">
      <div className="relative flex items-center justify-evenly">
        {emojiLibrary
          .getGrid()
          .sections()
          .map(({ id }) => (
            <WithTooltip
              key={id}
              side="bottom"
              delay={500}
              content={i18n.categories[id]}
              trigger={
                <Button
                  size="sm"
                  variant="ghost"
                  className={cn(
                    'h-fit rounded-full fill-current p-1.5 text-muted-foreground hover:bg-muted hover:text-muted-foreground',
                    id === focusedCategory &&
                      'pointer-events-none bg-accent fill-current text-accent-foreground',
                  )}
                  onClick={() => {
                    onClick(id)
                  }}
                  aria-label={i18n.categories[id]}
                  type="button"
                >
                  <span className="inline-flex size-5 items-center justify-center">
                    {icons.categories[id].outline}
                  </span>
                </Button>
              }
            />
          ))}
      </div>
    </nav>
  )
}

// The picker draws `outline` only; the type asks for a `solid` too.
const categoryIcon = (CategoryIcon: Icon) => {
  const icon = <CategoryIcon className="size-full" />
  return { outline: icon, solid: icon }
}

const emojiCategoryIcons: Record<
  EmojiCategoryList,
  { outline: React.ReactElement; solid: React.ReactElement }
> = {
  activity: categoryIcon(BasketballIcon),
  custom: categoryIcon(StarIcon),
  flags: categoryIcon(FlagIcon),
  foods: categoryIcon(HamburgerIcon),
  frequent: categoryIcon(ClockIcon),
  nature: categoryIcon(LeafIcon),
  objects: categoryIcon(LightbulbIcon),
  people: categoryIcon(SmileyIcon),
  places: categoryIcon(CompassIcon),
  symbols: categoryIcon(MusicNoteIcon),
}

const emojiSearchIcons = {
  delete: <XIcon className="size-4 text-current" />,
  loupe: <MagnifyingGlassIcon className="size-4 text-current" />,
}
