import { Loader2Icon } from "lucide-react"
import { cn } from "@/lib/utils"

function Spinner({ className, ...props }: React.ComponentProps<"svg">) {
  return (
    <Loader2Icon
      role="status"
      aria-label="Loading"
      className={cn("size-4 animate-spin", className)}
      {...props}
    />
  )
}

interface LoadingSpinnerProps {
  label?: string;
  showLabel?: boolean;
  compact?: boolean;
  className?: string;
}

function LoadingSpinner({
  label = "Loading",
  showLabel = false,
  compact = false,
  className,
}: LoadingSpinnerProps) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 text-muted-foreground",
        compact ? "ui-text-xs" : "ui-text-sm",
        className,
      )}
      role="status"
      aria-live="polite"
      aria-label={label}
    >
      <Spinner className={compact ? "size-3" : "size-4"} aria-label={undefined} />
      {showLabel ? <span>{label}</span> : null}
    </span>
  )
}

export { Spinner, LoadingSpinner }
