import { launcherFetch } from "@/api/http"

export type JobStatus = "disabled" | "error" | "due" | "scheduled" | "ok"

export type JobScheduleKind = "at" | "every" | "cron" | string

export interface JobSchedule {
  kind: JobScheduleKind
  atMs?: number
  everyMs?: number
  expr?: string
  tz?: string
}

export interface JobPayload {
  kind: string
  message: string
  command?: string
  channel?: string
  to?: string
}

export interface JobState {
  nextRunAtMs?: number
  lastRunAtMs?: number
  lastStatus?: string
  lastError?: string
}

export interface Job {
  id: string
  name: string
  enabled: boolean
  schedule: JobSchedule
  payload: JobPayload
  state: JobState
  createdAtMs: number
  updatedAtMs: number
  deleteAfterRun: boolean
  status: JobStatus
}

export interface JobsResponse {
  cron_enabled: boolean
  jobs: Job[]
}

export async function getJobs(): Promise<JobsResponse> {
  const res = await launcherFetch("/api/jobs")
  if (!res.ok) {
    throw new Error(await extractErrorMessage(res))
  }
  return (await res.json()) as JobsResponse
}

async function extractErrorMessage(res: Response): Promise<string> {
  try {
    const raw = await res.text()
    return raw.trim() === ""
      ? `API error: ${res.status} ${res.statusText}`
      : raw
  } catch {
    return `API error: ${res.status} ${res.statusText}`
  }
}
