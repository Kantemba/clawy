import { IconClock, IconTerminal2 } from "@tabler/icons-react"
import type { TFunction } from "i18next"

import type { Job } from "@/api/jobs"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"

import { JobStatusBadge } from "./job-status-badge"
import { describeSchedule, formatTimestamp } from "./job-utils"

interface JobCardProps {
  job: Job
  t: TFunction
}

export function JobCard({ job, t }: JobCardProps) {
  const nextRun = formatTimestamp(job.state.nextRunAtMs)
  const lastRun = formatTimestamp(job.state.lastRunAtMs)
  const summary = job.payload.message || job.payload.command || ""
  const target = [job.payload.channel, job.payload.to]
    .filter((part): part is string => Boolean(part))
    .join(" / ")

  return (
    <Card
      size="sm"
      className="border-border/40 bg-card/40 hover:border-border/80 hover:bg-card transition-all"
    >
      <CardHeader className="pb-2">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0 space-y-1">
            <CardTitle className="truncate text-base font-semibold tracking-tight">
              {job.name || job.id}
            </CardTitle>
            <CardDescription className="line-clamp-2 text-sm leading-relaxed">
              {summary || t("pages.jobs.no_summary")}
            </CardDescription>
          </div>
          <JobStatusBadge status={job.status} t={t} />
        </div>
      </CardHeader>

      <CardContent className="space-y-3">
        <dl className="grid grid-cols-1 gap-2 text-xs sm:grid-cols-2">
          <div className="flex items-start gap-2">
            <dt className="text-muted-foreground shrink-0">
              {t("pages.jobs.labels.schedule")}
            </dt>
            <dd className="text-foreground/90 font-medium">
              {describeSchedule(job.schedule, t)}
            </dd>
          </div>

          <div className="flex items-start gap-2">
            <dt className="text-muted-foreground shrink-0">
              {t("pages.jobs.labels.next_run")}
            </dt>
            <dd className="text-foreground/90 flex items-center gap-1 font-medium">
              <IconClock className="size-3.5 opacity-60" />
              {nextRun ?? t("pages.jobs.no_next_run")}
            </dd>
          </div>

          <div className="flex items-start gap-2">
            <dt className="text-muted-foreground shrink-0">
              {t("pages.jobs.labels.last_run")}
            </dt>
            <dd className="text-foreground/90 font-medium">
              {lastRun
                ? `${lastRun} · ${
                    job.state.lastStatus === "error"
                      ? t("pages.jobs.status.error")
                      : t("pages.jobs.status.ok")
                  }`
                : t("pages.jobs.never_ran")}
            </dd>
          </div>

          {target ? (
            <div className="flex items-start gap-2">
              <dt className="text-muted-foreground shrink-0">
                {t("pages.jobs.labels.target")}
              </dt>
              <dd className="text-foreground/90 flex items-center gap-1 font-medium">
                <IconTerminal2 className="size-3.5 opacity-60" />
                {target}
              </dd>
            </div>
          ) : null}
        </dl>

        {job.state.lastStatus === "error" && job.state.lastError ? (
          <p className="bg-destructive/10 text-destructive rounded-md px-3 py-2 text-xs break-words">
            {t("pages.jobs.last_error")}: {job.state.lastError}
          </p>
        ) : null}
      </CardContent>
    </Card>
  )
}
