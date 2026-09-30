import { useTranslation } from "react-i18next"

import type { OAuthProviderStatus } from "@/api/oauth"

interface ProviderStatusLineProps {
  status: OAuthProviderStatus["status"]
  authMethod?: string
}

export function ProviderStatusLine({
  status,
  authMethod,
}: ProviderStatusLineProps) {
  const { t } = useTranslation()

  const style =
    status === "connected"
      ? "bg-foreground/10 text-foreground"
      : status === "needs_refresh"
        ? "bg-foreground/10 text-foreground/70"
        : status === "expired"
          ? "bg-foreground/10 text-foreground line-through"
          : "bg-muted text-muted-foreground"

  return (
    <div className="flex items-center justify-between gap-2">
      <span className={`rounded px-2 py-1 text-xs font-medium ${style}`}>
        {status === "connected"
          ? t("credentials.status.connected")
          : status === "needs_refresh"
            ? t("credentials.status.needsRefresh")
            : status === "expired"
              ? t("credentials.status.expired")
              : t("credentials.status.notLoggedIn")}
      </span>
      {authMethod && (
        <span className="text-muted-foreground text-xs uppercase">
          {authMethod}
        </span>
      )}
    </div>
  )
}
