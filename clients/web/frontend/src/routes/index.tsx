import { Navigate, createFileRoute } from "@tanstack/react-router"

import { ChatPage } from "@/components/chat/chat-page"
import { useGateway } from "@/hooks/use-gateway"

function HomePage() {
  const { state, canStart } = useGateway()
  if (!canStart && (state === "stopped" || state === "error")) {
    return <Navigate to="/setup" replace />
  }
  return <ChatPage />
}

export const Route = createFileRoute("/")({
  component: HomePage,
})
