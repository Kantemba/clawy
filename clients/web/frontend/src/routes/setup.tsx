import { Link, createFileRoute } from "@tanstack/react-router"
import { useTranslation } from "react-i18next"

import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { useGateway } from "@/hooks/use-gateway"
import { refreshGatewayState } from "@/store/gateway"

function SetupPage() {
  const { t } = useTranslation()
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
        <div className="mx-auto flex max-w-2xl flex-col gap-4">
          <p className="text-muted-foreground">{t("onboarding.description")}</p>
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
              {running ? (
                <Button asChild>
                  <Link to="/">{t("onboarding.openChat")}</Link>
                </Button>
              ) : (
                <Button disabled={!ready || busy} onClick={() => void start()}>
                  {busy ? t("labels.loading") : t("onboarding.start")}
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
