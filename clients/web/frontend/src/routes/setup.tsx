import { Link, createFileRoute } from "@tanstack/react-router"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { IdentityEditor } from "@/components/agent/identity-editor"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { useAgentIdentity } from "@/hooks/use-agent-identity"
import { useGateway } from "@/hooks/use-gateway"
import { refreshGatewayState } from "@/store/gateway"

function SetupPage() {
  const { t } = useTranslation()
  const {
    identity,
    data,
    isPending,
    error: identityError,
    refetch,
  } = useAgentIdentity()
  const [identityDirty, setIdentityDirty] = useState(false)
  const identityReady = identity.configured && !identityDirty
  const { state, canStart, startReason, start, loading, error } = useGateway()
  const running = state === "running"
  const busy =
    loading ||
    state === "starting" ||
    state === "restarting" ||
    state === "stopping"
  const ready = state !== "unknown" && canStart

  return (
    <div className="flex h-full flex-col">
      <PageHeader title={t("onboarding.title")} />
      <div className="min-h-0 flex-1 overflow-y-auto p-4 md:p-8">
        <div className="mx-auto flex max-w-4xl flex-col gap-4">
          <p className="text-muted-foreground">{t("onboarding.description")}</p>
          <Card className="overflow-hidden">
            <CardHeader className="bg-muted/20 border-b">
              <CardTitle>{t("identity.title")}</CardTitle>
              <CardDescription>{t("identity.description")}</CardDescription>
            </CardHeader>
            <CardContent className="pt-6">
              {isPending ? (
                <p role="status">{t("labels.loading")}</p>
              ) : identityError ? (
                <div className="space-y-3">
                  <p role="alert" className="text-destructive text-sm">
                    {identityError.message}
                  </p>
                  <Button variant="outline" onClick={() => void refetch()}>
                    {t("onboarding.checkAgain")}
                  </Button>
                </div>
              ) : data ? (
                <IdentityEditor
                  key={JSON.stringify(data)}
                  profile={data}
                  onDirtyChange={setIdentityDirty}
                />
              ) : null}
              {identity.configured && !identityDirty && (
                <p className="text-muted-foreground mt-4 text-sm" role="status">
                  {t("identity.saved", { name: identity.name })}
                </p>
              )}
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>{t("onboarding.passwordTitle")}</CardTitle>
              <CardDescription>{t("onboarding.passwordDone")}</CardDescription>
            </CardHeader>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>{t("onboarding.modelTitle")}</CardTitle>
              <CardDescription>
                {t("onboarding.modelDescription")}
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-3">
              <div className="flex flex-wrap gap-2">
                <Button asChild>
                  <Link to="/models">{t("onboarding.configureModel")}</Link>
                </Button>
                <Button variant="outline" asChild>
                  <Link to="/credentials">
                    {t("onboarding.connectAccount")}
                  </Link>
                </Button>
              </div>
              <p className="text-muted-foreground text-sm" role="status">
                {state === "unknown"
                  ? t("onboarding.checking")
                  : ready || running
                    ? t("onboarding.modelReady")
                    : t("onboarding.modelBlocked", {
                        reason: startReason || t("onboarding.configureModel"),
                      })}
              </p>
              <Button
                variant="outline"
                onClick={() => void refreshGatewayState({ force: true })}
              >
                {t("onboarding.checkAgain")}
              </Button>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>{t("onboarding.channelsTitle")}</CardTitle>
              <CardDescription>
                {t("onboarding.channelsDescription")}
              </CardDescription>
            </CardHeader>
            <CardContent>
              <Button variant="outline" asChild>
                <Link to="/channels/$name" params={{ name: "telegram" }}>
                  {t("onboarding.configureChannels")}
                </Link>
              </Button>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>{t("onboarding.finishTitle")}</CardTitle>
              <CardDescription>
                {t("onboarding.finishDescription")}
              </CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-3">
              {!identityReady && (
                <p className="text-muted-foreground text-sm">
                  {t("identity.saveFirst")}
                </p>
              )}
              {running && identityReady ? (
                <Button asChild>
                  <Link to="/">{t("onboarding.openChat")}</Link>
                </Button>
              ) : (
                <Button
                  disabled={!identityReady || !ready || busy || running}
                  onClick={() => void start()}
                >
                  {busy
                    ? t("labels.loading")
                    : t("identity.bringToLife", { name: identity.name })}
                </Button>
              )}
              {error && (
                <p className="text-destructive text-sm" role="alert">
                  {error}
                </p>
              )}
              {state === "error" && (
                <Button variant="outline" asChild>
                  <Link to="/logs">{t("navigation.logs")}</Link>
                </Button>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  )
}

export const Route = createFileRoute("/setup")({ component: SetupPage })
