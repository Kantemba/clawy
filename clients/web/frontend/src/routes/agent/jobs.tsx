import { createFileRoute } from "@tanstack/react-router"

import { JobsPage } from "@/components/agent/jobs/jobs-page"

export const Route = createFileRoute("/agent/jobs")({
  component: AgentJobsRoute,
})

function AgentJobsRoute() {
  return <JobsPage />
}
