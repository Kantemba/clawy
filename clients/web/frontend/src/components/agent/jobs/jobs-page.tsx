import {
  IconAlertTriangle,
  IconCalendarClock,
  IconLoader2,
  IconRefresh,
} from "@tabler/icons-react"
import { useCallback, useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

import { type Job, getJobs } from "@/api/jobs"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"

import { JobCard } from "./job-card"
import { compareJobs, formatTimestamp } from "./job-utils"

const REFRESH_INTERVAL_MS = 15_000

export function JobsPage() {
  const { t } = useTranslation()
  const [jobs, setJobs] = useState<Job[]>([])
  const [cronEnabled, setCronEnabled] = useState(true)
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState("")

  const fetchJobs = useCallback(
    async (showSpinner = false) => {
      if (showSpinner) {
        setLoading(true)
      }
      try {
        const data = await getJobs()
        setJobs([...data.jobs].sort(compareJobs))
        setCronEnabled(data.cron_enabled)
        setLoadError("")
      } catch (error) {
        setLoadError(
          error instanceof Error ? error.message : t("pages.jobs.load_error"),
        )
      } finally {
        setLoading(false)
      }
    },
    [t],
  )

  useEffect(() => {
    void fetchJobs(true)
    const timer = setInterval(() => void fetchJobs(), REFRESH_INTERVAL_MS)
    return () => clearInterval(timer)
  }, [fetchJobs])

  const activeCount = jobs.filter((job) => job.enabled).length
  const errorCount = jobs.filter((job) => job.status === "error").length
  const nextRun = jobs
    .map((job) => job.state.nextRunAtMs)
    .filter((ms): ms is number => typeof ms === "number" && ms > 0)
    .sort((a, b) => a - b)[0]

  return (
    <div className="bg-background flex h-full flex-col">
      <PageHeader
        title={t("navigation.jobs")}
        children={
          <Button
            variant="outline"
            size="sm"
            onClick={() => void fetchJobs(true)}
            disabled={loading}
          >
            {loading ? (
              <IconLoader2 className="size-4 animate-spin" />
            ) : (
              <IconRefresh className="size-4" />
            )}
            {t("pages.jobs.refresh")}
          </Button>
        }
      />

      <div className="flex-1 overflow-auto px-6 py-6 pb-20">
        <div className="mx-auto w-full max-w-6xl space-y-6">
          <p className="text-muted-foreground text-sm">
            {t("pages.jobs.description")}
          </p>

          {!cronEnabled ? (
            <div className="flex items-start gap-3 rounded-xl border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm">
              <IconAlertTriangle className="mt-0.5 size-4 shrink-0 text-amber-600 dark:text-amber-400" />
              <div>
                <p className="font-medium text-amber-700 dark:text-amber-300">
                  {t("pages.jobs.cron_disabled_title")}
                </p>
                <p className="text-amber-700/80 dark:text-amber-300/80">
                  {t("pages.jobs.cron_disabled")}
                </p>
              </div>
            </div>
          ) : null}

          {loadError ? (
            <div className="border-destructive/30 bg-destructive/10 flex items-start gap-3 rounded-xl border px-4 py-3 text-sm">
              <IconAlertTriangle className="text-destructive mt-0.5 size-4 shrink-0" />
              <div className="flex-1">
                <p className="text-destructive font-medium">
                  {t("pages.jobs.load_error")}
                </p>
                <p className="text-destructive/80 break-words">{loadError}</p>
              </div>
              <Button
                variant="outline"
                size="sm"
                onClick={() => void fetchJobs(true)}
              >
                {t("pages.jobs.retry")}
              </Button>
            </div>
          ) : null}

          {!loading && !loadError && jobs.length > 0 ? (
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
              <Stat
                label={t("pages.jobs.stats.total")}
                value={String(jobs.length)}
              />
              <Stat
                label={t("pages.jobs.stats.active")}
                value={String(activeCount)}
              />
              <Stat
                label={t("pages.jobs.stats.errors")}
                value={String(errorCount)}
                highlight={errorCount > 0}
              />
              <Stat
                label={t("pages.jobs.stats.next")}
                value={formatTimestamp(nextRun) ?? t("pages.jobs.no_next_run")}
              />
            </div>
          ) : null}

          {loading && jobs.length === 0 ? (
            <div className="text-muted-foreground flex items-center justify-center gap-2 py-16 text-sm">
              <IconLoader2 className="size-4 animate-spin" />
              {t("pages.jobs.loading")}
            </div>
          ) : null}

          {!loading && !loadError && jobs.length === 0 ? (
            <div className="border-border/40 bg-muted/5 flex flex-col items-center justify-center gap-3 rounded-xl border border-dashed py-16 text-center">
              <div className="bg-muted mb-2 rounded-full p-4">
                <IconCalendarClock className="text-muted-foreground size-6" />
              </div>
              <h3 className="text-lg font-semibold tracking-tight">
                {t("pages.jobs.empty")}
              </h3>
              <p className="text-muted-foreground max-w-md text-sm">
                {t("pages.jobs.empty_hint")}
              </p>
            </div>
          ) : null}

          {!loading && jobs.length > 0 ? (
            <div className="grid gap-4 lg:grid-cols-2">
              {jobs.map((job) => (
                <JobCard key={job.id} job={job} t={t} />
              ))}
            </div>
          ) : null}
        </div>
      </div>
    </div>
  )
}

interface StatProps {
  label: string
  value: string
  highlight?: boolean
}

function Stat({ label, value, highlight = false }: StatProps) {
  return (
    <div className="border-border/40 bg-card/40 rounded-xl border px-4 py-3">
      <p className="text-muted-foreground text-xs">{label}</p>
      <p
        className={`mt-1 truncate text-lg font-semibold tracking-tight ${
          highlight ? "text-destructive" : "text-foreground/90"
        }`}
        title={value}
      >
        {value}
      </p>
    </div>
  )
}
