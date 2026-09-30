import {
  IconCheck,
  IconLoader2,
  IconPlugConnected,
  IconX,
} from "@tabler/icons-react"
import { useCallback, useEffect, useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import type { ChannelConfig } from "@/api/channels"
import { pollChannelOAuth, startChannelOAuth } from "@/api/channels"
import { getSecretInputPlaceholder } from "@/components/channels/channel-config-fields"
import { Field, KeyInput } from "@/components/shared-form"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Input } from "@/components/ui/input"

type OAuthPhase =
  | "idle"
  | "starting"
  | "waiting"
  | "success"
  | "needs_token"
  | "expired"
  | "error"

const POLL_INTERVAL_MS = 1500
const MAX_POLL_ATTEMPTS = 200
const MAX_CONSECUTIVE_ERRORS = 10

interface ChannelOAuthConnectProps {
  channelName: "slack" | "discord"
  config: ChannelConfig
  onChange: (key: string, value: unknown) => void
  configuredSecrets: string[]
  onBindSuccess?: () => void
}

function asString(value: unknown): string {
  return typeof value === "string" ? value : ""
}

export function ChannelOAuthConnect({
  channelName,
  config,
  onChange,
  configuredSecrets,
  onBindSuccess,
}: ChannelOAuthConnectProps) {
  const { t } = useTranslation()

  const [phase, setPhase] = useState<OAuthPhase>("idle")
  const [message, setMessage] = useState("")
  const [accountID, setAccountID] = useState("")
  const [authURL, setAuthURL] = useState("")
  const [redirectHandled, setRedirectHandled] = useState(false)

  const pollTimerRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const pollGenerationRef = useRef(0)
  const watchedFlowRef = useRef("")

  const stopPolling = useCallback(() => {
    pollGenerationRef.current += 1
    if (pollTimerRef.current !== null) {
      clearInterval(pollTimerRef.current)
      pollTimerRef.current = null
    }
    watchedFlowRef.current = ""
  }, [])

  useEffect(() => () => stopPolling(), [stopPolling])

  const startPolling = useCallback(
    (flowID: string) => {
      stopPolling()
      const generation = pollGenerationRef.current
      watchedFlowRef.current = flowID
      let inFlight = false
      let attempts = 0
      let failures = 0

      pollTimerRef.current = setInterval(async () => {
        if (inFlight) return
        inFlight = true
        try {
          const resp = await pollChannelOAuth(channelName, flowID)
          if (generation !== pollGenerationRef.current) return
          failures = 0
          attempts += 1

          switch (resp.status) {
            case "success":
              stopPolling()
              setAccountID(resp.account_id ?? "")
              setPhase("success")
              onBindSuccess?.()
              return
            case "needs_token":
              stopPolling()
              setAccountID(resp.account_id ?? "")
              setPhase("needs_token")
              return
            case "expired":
              stopPolling()
              setPhase("expired")
              setMessage(t("channels.oauth.expired"))
              return
            case "error":
              stopPolling()
              setPhase("error")
              setMessage(resp.error || t("channels.oauth.errorGeneric"))
              return
            default:
              if (attempts >= MAX_POLL_ATTEMPTS) {
                stopPolling()
                setPhase("error")
                setMessage(t("channels.oauth.timeout"))
              }
          }
        } catch (e) {
          if (generation !== pollGenerationRef.current) return
          failures += 1
          if (failures >= MAX_CONSECUTIVE_ERRORS) {
            stopPolling()
            setPhase("error")
            setMessage(
              e instanceof Error ? e.message : t("channels.oauth.errorGeneric"),
            )
          }
        } finally {
          inFlight = false
        }
      }, POLL_INTERVAL_MS)
    },
    [channelName, onBindSuccess, stopPolling, t],
  )

  // If the provider redirected the main tab (popup blocked), resume watching
  // the flow from the query string instead of starting over.
  useEffect(() => {
    if (redirectHandled) return
    setRedirectHandled(true)

    const params = new URLSearchParams(window.location.search)
    const flowID = params.get("oauth_flow_id")
    if (!flowID) return

    window.history.replaceState({}, "", window.location.pathname)
    setPhase("waiting")
    startPolling(flowID)
  }, [redirectHandled, startPolling])

  const handleConnect = async () => {
    setPhase("starting")
    setMessage("")
    setAccountID("")
    stopPolling()

    try {
      const resp = await startChannelOAuth(channelName, {
        client_id: asString(config.client_id),
        client_secret: asString(config._client_secret),
        scopes: channelName === "slack" ? asString(config.oauth_scopes) : "",
        permissions:
          channelName === "discord" ? asString(config.permissions) : "",
      })
      if (!resp.auth_url) {
        throw new Error(t("channels.oauth.errorGeneric"))
      }

      const popup = window.open("", "_blank")
      if (!popup) {
        setPhase("error")
        setMessage(t("channels.oauth.popupBlocked"))
        setAuthURL(resp.auth_url)
        return
      }

      popup.location.href = resp.auth_url
      setAuthURL(resp.auth_url)
      setPhase("waiting")
      startPolling(resp.flow_id)
    } catch (e) {
      setPhase("error")
      setMessage(
        e instanceof Error ? e.message : t("channels.oauth.errorGeneric"),
      )
    }
  }

  const handleRetry = () => {
    stopPolling()
    setPhase("idle")
    setMessage("")
    setAccountID("")
    setAuthURL("")
  }

  const redirectURI =
    typeof window !== "undefined"
      ? `${window.location.origin}/channel/oauth/callback`
      : "/channel/oauth/callback"

  const renderStatus = () => {
    if (phase === "starting") {
      return (
        <div className="text-muted-foreground flex items-center gap-2 text-sm">
          <IconLoader2 className="animate-spin" size={16} />
          {t("channels.oauth.starting")}
        </div>
      )
    }

    if (phase === "waiting") {
      return (
        <div className="space-y-2">
          <div className="text-muted-foreground flex items-center gap-2 text-sm">
            <IconLoader2 className="animate-spin" size={16} />
            {t("channels.oauth.waiting")}
          </div>
          {authURL && (
            <a
              href={authURL}
              target="_blank"
              rel="noreferrer"
              className="text-primary text-sm underline underline-offset-4"
            >
              {t("channels.oauth.reopen")}
            </a>
          )}
        </div>
      )
    }

    if (phase === "success") {
      return (
        <div className="space-y-3">
          <div className="text-foreground flex items-center gap-2 text-sm font-medium">
            <span className="bg-foreground/10 flex h-8 w-8 items-center justify-center rounded-full">
              <IconCheck size={16} />
            </span>
            {t("channels.oauth.success")}
          </div>
          {accountID && (
            <p className="text-muted-foreground font-mono text-xs">
              {accountID}
            </p>
          )}
          <Button variant="outline" size="sm" onClick={handleRetry}>
            {t("channels.oauth.reconnect")}
          </Button>
        </div>
      )
    }

    if (phase === "needs_token") {
      return (
        <div className="space-y-3">
          <div className="text-foreground flex items-start gap-2 text-sm font-medium">
            <span className="bg-destructive/10 flex h-8 w-8 shrink-0 items-center justify-center rounded-full">
              <IconX size={16} className="text-destructive" />
            </span>
            {t("channels.oauth.needsTokenTitle")}
          </div>
          <p className="text-muted-foreground text-sm">
            {t("channels.oauth.needsToken")}
          </p>
          <Button variant="outline" size="sm" onClick={handleRetry}>
            {t("channels.oauth.retry")}
          </Button>
        </div>
      )
    }

    if (phase === "expired") {
      return (
        <div className="space-y-3">
          <p className="text-muted-foreground text-sm">
            {t("channels.oauth.expired")}
          </p>
          <Button variant="outline" size="sm" onClick={handleRetry}>
            {t("channels.oauth.retry")}
          </Button>
        </div>
      )
    }

    if (phase === "error") {
      return (
        <div className="space-y-3">
          <p className="text-destructive text-sm">
            {message || t("channels.oauth.errorGeneric")}
          </p>
          <div className="flex flex-wrap gap-2">
            {authURL && (
              <a href={authURL} target="_blank" rel="noreferrer">
                <Button variant="outline" size="sm">
                  {t("channels.oauth.reopen")}
                </Button>
              </a>
            )}
            <Button variant="outline" size="sm" onClick={handleRetry}>
              {t("channels.oauth.retry")}
            </Button>
          </div>
        </div>
      )
    }

    return (
      <Button onClick={() => void handleConnect()} className="gap-2">
        <IconPlugConnected size={16} />
        {channelName === "slack"
          ? t("channels.oauth.connectSlack")
          : t("channels.oauth.connectDiscord")}
      </Button>
    )
  }

  return (
    <Card className="shadow-sm">
      <CardHeader className="border-border/60 border-b px-6">
        <CardTitle className="text-foreground text-sm font-medium">
          {channelName === "slack"
            ? t("channels.oauth.titleSlack")
            : t("channels.oauth.titleDiscord")}
        </CardTitle>
        <CardDescription>
          {channelName === "slack"
            ? t("channels.oauth.descSlack")
            : t("channels.oauth.descDiscord")}
        </CardDescription>
      </CardHeader>
      <CardContent className="divide-border/60 divide-y px-6 py-0 [&>div]:py-5">
        <Field
          label={t("channels.oauth.clientId")}
          hint={t("channels.oauth.clientIdHint")}
        >
          <Input
            value={asString(config.client_id)}
            onChange={(e) => onChange("client_id", e.target.value)}
            placeholder={channelName === "slack" ? "111.222.333" : "bot app id"}
            autoComplete="off"
          />
        </Field>

        <Field
          label={t("channels.oauth.clientSecret")}
          hint={t("channels.oauth.clientSecretHint")}
        >
          <KeyInput
            value={asString(config._client_secret)}
            onChange={(v) => onChange("_client_secret", v)}
            placeholder={getSecretInputPlaceholder(
              configuredSecrets,
              "client_secret",
              t("channels.field.secretHintSet"),
              channelName === "slack" ? "xoxb-…-secret" : "client secret",
            )}
          />
        </Field>

        {channelName === "slack" ? (
          <Field
            label={t("channels.oauth.scopes")}
            hint={t("channels.oauth.scopesHint")}
          >
            <Input
              value={asString(config.oauth_scopes)}
              onChange={(e) => onChange("oauth_scopes", e.target.value)}
              placeholder="chat:write,files:write,reactions:write"
              autoComplete="off"
            />
          </Field>
        ) : (
          <Field
            label={t("channels.oauth.permissions")}
            hint={t("channels.oauth.permissionsHint")}
          >
            <Input
              value={asString(config.permissions)}
              onChange={(e) => onChange("permissions", e.target.value)}
              placeholder="274878024704"
              autoComplete="off"
            />
          </Field>
        )}

        <div className="space-y-2">
          <p className="text-foreground text-sm font-medium">
            {t("channels.oauth.redirectUri")}
          </p>
          <Input
            value={redirectURI}
            readOnly
            className="text-muted-foreground font-mono text-xs"
          />
          <p className="text-muted-foreground text-xs">
            {t("channels.oauth.redirectUriHint")}
          </p>
        </div>

        {channelName === "slack" && (
          <p className="text-muted-foreground text-xs">
            {t("channels.oauth.slackSocketModeNote")}
          </p>
        )}

        <div className="space-y-3">{renderStatus()}</div>
      </CardContent>
    </Card>
  )
}
