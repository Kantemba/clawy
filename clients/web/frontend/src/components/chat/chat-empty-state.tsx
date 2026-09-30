import { IconPlugConnectedX, IconRobotOff, IconStar } from "@tabler/icons-react"
import { Link } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { useAgentIdentity } from "@/hooks/use-agent-identity"

interface ChatEmptyStateProps {
  hasAvailableModels: boolean
  defaultModelName: string
  isConnected: boolean
}

export function ChatEmptyState({
  hasAvailableModels,
  defaultModelName,
  isConnected,
}: ChatEmptyStateProps) {
  const { t } = useTranslation()
  const { identity } = useAgentIdentity()

  if (!hasAvailableModels) {
    return (
      <div className="flex flex-col items-center justify-center py-20 opacity-70">
        <div className="bg-foreground/10 text-foreground mb-6 flex h-16 w-16 items-center justify-center rounded-2xl">
          <IconRobotOff className="h-8 w-8" />
        </div>
        <h3 className="mb-2 text-xl font-medium">
          {t("chat.empty.noConfiguredModel")}
        </h3>
        <p className="text-muted-foreground mb-4 text-center text-sm">
          {t("chat.empty.noConfiguredModelDescription")}
        </p>
        <Button asChild variant="outline" size="sm" className="px-4">
          <Link to="/models">{t("chat.empty.goToModels")}</Link>
        </Button>
      </div>
    )
  }

  if (!defaultModelName) {
    return (
      <div className="flex flex-col items-center justify-center py-20 opacity-70">
        <div className="bg-foreground/10 text-foreground mb-6 flex h-16 w-16 items-center justify-center rounded-2xl">
          <IconStar className="h-8 w-8" />
        </div>
        <h3 className="mb-2 text-xl font-medium">
          {t("chat.empty.noSelectedModel")}
        </h3>
        <p className="text-muted-foreground mb-4 text-center text-sm">
          {t("chat.empty.noSelectedModelDescription")}
        </p>
      </div>
    )
  }

  if (!isConnected) {
    return (
      <div className="flex flex-col items-center justify-center py-20 opacity-70">
        <div className="bg-foreground/10 text-foreground mb-6 flex h-16 w-16 items-center justify-center rounded-2xl">
          <IconPlugConnectedX className="h-8 w-8" />
        </div>
        <h3 className="mb-2 text-xl font-medium">
          {t("chat.empty.notRunning")}
        </h3>
        <p className="text-muted-foreground mb-4 text-center text-sm">
          {t("chat.empty.notRunningDescription")}
        </p>
      </div>
    )
  }

  return (
    <div className="flex flex-col items-center justify-center py-20 opacity-70">
      <div className="bg-foreground/10 text-foreground mb-6 flex h-16 w-16 items-center justify-center rounded-2xl">
        <span className="text-4xl" aria-hidden="true">
          {identity.avatar}
        </span>
      </div>
      <p className="text-muted-foreground mb-2 text-sm">
        {t("identity.meet", { name: identity.name })}
      </p>
      <h3 className="mb-2 max-w-xl text-center text-xl font-medium break-words">
        {identity.greeting || t("chat.welcome")}
      </h3>
      <p className="text-muted-foreground text-center text-sm">
        {identity.role || t("chat.welcomeDesc")}
      </p>
      <Button asChild variant="ghost" size="sm" className="mt-4">
        <Link to="/setup">{t("identity.edit")}</Link>
      </Button>
    </div>
  )
}
