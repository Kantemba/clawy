import {
  IconCircleCheck,
  IconCircleDashed,
  IconCircleX,
  IconClockExclamation,
  IconPlayerPause,
} from "@tabler/icons-react"
import type { TFunction } from "i18next"
import type { ComponentType } from "react"

import type { JobStatus } from "@/api/jobs"
import { cn } from "@/lib/utils"

interface JobStatusBadgeProps {
  status: JobStatus
  t: TFunction
}

const STATUS_STYLES: Record<
  JobStatus,
  { className: string; icon: ComponentType<{ className?: string }> }
> = {
  ok: {
    className:
      "bg-emerald-500/10 text-emerald-600 dark:bg-emerald-500/15 dark:text-emerald-400",
    icon: IconCircleCheck,
  },
  due: {
    className:
      "bg-sky-500/10 text-sky-600 dark:bg-sky-500/15 dark:text-sky-400",
    icon: IconClockExclamation,
  },
  scheduled: {
    className: "bg-muted text-muted-foreground",
    icon: IconCircleDashed,
  },
  error: {
    className:
      "bg-destructive/10 text-destructive dark:bg-destructive/20 dark:text-destructive",
    icon: IconCircleX,
  },
  disabled: {
    className:
      "bg-muted text-muted-foreground/80 dark:bg-muted-foreground/20 dark:text-muted-foreground",
    icon: IconPlayerPause,
  },
}

export function JobStatusBadge({ status, t }: JobStatusBadgeProps) {
  const style = STATUS_STYLES[status] ?? STATUS_STYLES.scheduled
  const Icon = style.icon

  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center gap-1 rounded-full px-2.5 py-0.5 text-[11px] font-medium tracking-wide",
        style.className,
      )}
    >
      <Icon className="size-3" />
      {t(`pages.jobs.status.${status}`, status)}
    </span>
  )
}
