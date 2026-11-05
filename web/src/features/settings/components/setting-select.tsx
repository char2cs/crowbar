import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { SETTINGS_CONTROL_WIDTHS } from '@/features/settings/components/settings-control-widths'

interface SettingSelectProps {
  options: readonly { value: string; label: string }[]
  value: string
  onValueChange: (value: string | null) => void
  width?: keyof typeof SETTINGS_CONTROL_WIDTHS
  placeholder?: string
}

/** A settings row's dropdown: its options, labelled as they are listed. */
export function SettingSelect({
  options,
  value,
  onValueChange,
  width = 'default',
  placeholder,
}: SettingSelectProps) {
  return (
    <Select items={options} value={value} onValueChange={onValueChange}>
      <SelectTrigger size="sm" className={SETTINGS_CONTROL_WIDTHS[width]}>
        <SelectValue placeholder={placeholder} />
      </SelectTrigger>
      <SelectContent>
        {options.map((opt) => (
          <SelectItem key={opt.value} value={opt.value}>
            {opt.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}
