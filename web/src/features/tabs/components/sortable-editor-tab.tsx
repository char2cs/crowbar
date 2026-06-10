import { useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import type { ReactNode } from 'react'

interface SortableEditorTabProps {
  id: string
  children: ReactNode
  tabRef: (element: HTMLDivElement | null) => void
}

function SortableEditorTab({ id, children, tabRef }: SortableEditorTabProps) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id,
  })

  return (
    <div
      ref={(element) => {
        setNodeRef(element)
        tabRef(element)
      }}
      style={{
        transform: CSS.Transform.toString(transform),
        transition,
      }}
      className={isDragging ? 'relative z-10 opacity-40' : 'relative'}
      {...attributes}
      {...listeners}
    >
      {children}
    </div>
  )
}

export default SortableEditorTab
