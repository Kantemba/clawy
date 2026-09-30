import { Navigate, createFileRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { ChatPage } from "@/components/chat/chat-page"
import { Button } from "@/components/ui/button"
import { useAgentIdentity } from "@/hooks/use-agent-identity"
import { useGateway } from "@/hooks/use-gateway"

function HomePage() {
  const { state, canStart } = useGateway()
  const { identity, isPending, error, refetch } = useAgentIdentity()
  const { t } = useTranslation()
  if (isPending)
    return (
      <p className="p-8" role="status">
        {t("labels.loading")}
      </p>
    )
  if (error)
    return (
      <div className="space-y-3 p-8">
        <p role="alert">{error.message}</p>
        <Button onClick={() => void refetch()}>
          {t("onboarding.checkAgain")}
        </Button>
      </div>
    )
  if (!identity.configured) return <Navigate to="/setup" replace />
  if (!canStart && (state === "stopped" || state === "error")) {
    return <Navigate to="/setup" replace />
  }
  return <ChatPage />
}

export const Route = createFileRoute("/")({
  component: HomePage,
})
