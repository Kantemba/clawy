import dayjs from "dayjs"
import type { TFunction } from "i18next"

import type { Job, JobSchedule } from "@/api/jobs"

const MINUTE_MS = 60 * 1000
const HOUR_MS = 60 * MINUTE_MS
const DAY_MS = 24 * HOUR_MS

/** Formats a millisecond duration as a compact value such as `30s` or `2h`. */
export function formatInterval(ms: number): string {
  if (ms <= 0) {
    return "0s"
  }
  if (ms % DAY_MS === 0) {
    return `${ms / DAY_MS}d`
  }
  if (ms % HOUR_MS === 0) {
    return `${ms / HOUR_MS}h`
  }
  if (ms % MINUTE_MS === 0) {
    return `${ms / MINUTE_MS}m`
  }
  if (ms < 1000) {
    return `${ms}ms`
  }
  return `${Math.round(ms / 1000)}s`
}

/** Returns a human readable label for the job schedule. */
export function describeSchedule(schedule: JobSchedule, t: TFunction): string {
  switch (schedule.kind) {
    case "at":
      return schedule.atMs
        ? `${t("pages.jobs.schedule.once")} · ${formatTimestamp(schedule.atMs)}`
        : t("pages.jobs.schedule.once")
    case "every":
      return schedule.everyMs
        ? t("pages.jobs.schedule.every", {
            interval: formatInterval(schedule.everyMs),
          })
        : t("pages.jobs.schedule.unknown")
    case "cron":
      return schedule.expr
        ? t("pages.jobs.schedule.cron", { expr: schedule.expr })
        : t("pages.jobs.schedule.unknown")
    default:
      return t("pages.jobs.schedule.unknown")
  }
}

/** Formats a Unix millisecond timestamp, returning `null` when unset. */
export function formatTimestamp(ms?: number): string | null {
  if (!ms || ms <= 0) {
    return null
  }
  return dayjs(ms).format("LLL")
}

/** Sorts jobs: due first, then by upcoming run, then by name. */
export function compareJobs(left: Job, right: Job): number {
  if (left.status === "error" && right.status !== "error") {
    return -1
  }
  if (right.status === "error" && left.status !== "error") {
    return 1
  }

  const leftNext = left.state.nextRunAtMs ?? Number.POSITIVE_INFINITY
  const rightNext = right.state.nextRunAtMs ?? Number.POSITIVE_INFINITY
  if (leftNext !== rightNext) {
    return leftNext - rightNext
  }

  return left.name.localeCompare(right.name)
}
