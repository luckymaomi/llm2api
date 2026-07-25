import { DropdownMenu, Tooltip } from 'radix-ui'
import { Ellipsis } from 'lucide-react'
import type { ButtonHTMLAttributes, ReactNode } from 'react'

import { cn } from '@/lib/cn'

interface TableActionProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  label: string
  icon: ReactNode
  tone?: 'default' | 'positive' | 'warning' | 'danger'
  showTooltip?: boolean
}

export function TableAction({
  label,
  icon,
  tone = 'default',
  showTooltip = true,
  className,
  ...props
}: TableActionProps) {
  const button = (
    <button
      type="button"
      className={cn('table-action', `table-action--${tone}`, className)}
      aria-label={label}
      {...props}
    >
      {icon}
    </button>
  )
  if (!showTooltip) return button
  return (
    <Tooltip.Root>
      <Tooltip.Trigger asChild>{button}</Tooltip.Trigger>
      <Tooltip.Portal>
        <Tooltip.Content className="tooltip" sideOffset={7}>
          {label}
          <Tooltip.Arrow className="tooltip__arrow" />
        </Tooltip.Content>
      </Tooltip.Portal>
    </Tooltip.Root>
  )
}

export function RowActionMenu({
  children,
  label = '更多',
}: {
  children: ReactNode
  label?: string
}) {
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger asChild>
        <TableAction label={label} icon={<Ellipsis size={17} />} showTooltip={false} />
      </DropdownMenu.Trigger>
      <DropdownMenu.Portal>
        <DropdownMenu.Content className="row-action-menu" align="end" sideOffset={6}>
          {children}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

export function RowActionItem({
  children,
  icon,
  danger,
  disabled,
  onSelect,
}: {
  children: ReactNode
  icon: ReactNode
  danger?: boolean
  disabled?: boolean
  onSelect: () => void
}) {
  return (
    <DropdownMenu.Item
      className={cn('row-action-menu__item', danger && 'row-action-menu__item--danger')}
      disabled={Boolean(disabled)}
      onSelect={onSelect}
    >
      {icon}
      <span>{children}</span>
    </DropdownMenu.Item>
  )
}

export function RowActionSeparator() {
  return <DropdownMenu.Separator className="row-action-menu__separator" />
}
