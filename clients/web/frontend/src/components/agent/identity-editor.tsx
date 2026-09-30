import { IconCheck, IconLoader2, IconSparkles } from "@tabler/icons-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import type { AgentIdentity } from "@/api/identity"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import { useAgentIdentity } from "@/hooks/use-agent-identity"

const avatars = ["🦞", "🤖", "🦊", "🐙", "🐱", "🌱", "✨", "🧠"]
const presets = [
  {
    key: "companion",
    personality:
      "Warm, empathetic, and encouraging. Talk naturally, ask thoughtful questions, and help me think things through.",
  },
  {
    key: "builder",
    personality:
      "Practical, direct, and resourceful. Focus on clear plans, working solutions, and actionable next steps.",
  },
  {
    key: "explorer",
    personality:
      "Curious, imaginative, and playful. Explore ideas from different angles and help me discover new possibilities.",
  },
]

export function IdentityEditor({
  profile,
  onDirtyChange,
}: {
  profile: AgentIdentity
  onDirtyChange: (dirty: boolean) => void
}) {
  const { t } = useTranslation()
  const { save } = useAgentIdentity()
  const [draft, setDraft] = useState(profile)
  const [saved, setSaved] = useState(false)
  const name = draft.name.trim() || t("identity.unnamed")

  function update(field: keyof AgentIdentity, value: string) {
    const next = { ...draft, [field]: value }
    setDraft(next)
    setSaved(false)
    save.reset()
    onDirtyChange(JSON.stringify(next) !== JSON.stringify(profile))
  }

  async function submit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    try {
      await save.mutateAsync(draft)
      onDirtyChange(false)
      setSaved(true)
    } catch {
      // The mutation exposes the error next to the submit button.
    }
  }

  return (
    <form
      onSubmit={(event) => void submit(event)}
      className="grid gap-6 md:grid-cols-[1fr_220px]"
    >
      <fieldset disabled={save.isPending} className="min-w-0 space-y-5">
        <div className="space-y-2">
          <Label htmlFor="agent-name">{t("identity.name")}</Label>
          <Input
            id="agent-name"
            value={draft.name}
            onChange={(e) => update("name", e.target.value)}
            placeholder={t("identity.namePlaceholder")}
            required
            maxLength={64}
            autoComplete="off"
          />
        </div>
        <div className="space-y-2">
          <Label>{t("identity.avatar")}</Label>
          <div
            className="flex flex-wrap gap-2"
            role="group"
            aria-label={t("identity.avatar")}
          >
            {avatars.map((avatar) => (
              <Button
                key={avatar}
                type="button"
                variant={draft.avatar === avatar ? "secondary" : "outline"}
                className="size-10 text-xl"
                aria-pressed={draft.avatar === avatar}
                aria-label={t("identity.chooseAvatar", { avatar })}
                onClick={() => update("avatar", avatar)}
              >
                {avatar}
              </Button>
            ))}
          </div>
          <Input
            aria-label={t("identity.customAvatar")}
            value={draft.avatar}
            onChange={(e) => update("avatar", e.target.value)}
            maxLength={16}
            className="w-24 text-center"
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="agent-role">{t("identity.role")}</Label>
          <Input
            id="agent-role"
            value={draft.role}
            onChange={(e) => update("role", e.target.value)}
            maxLength={200}
            placeholder={t("identity.rolePlaceholder")}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="agent-personality">{t("identity.personality")}</Label>
          <p className="text-muted-foreground text-xs">
            {t("identity.presetsHint")}
          </p>
          <div className="flex flex-wrap gap-2">
            {presets.map((preset) => (
              <Button
                key={preset.key}
                type="button"
                size="sm"
                variant="outline"
                onClick={() => update("personality", preset.personality)}
              >
                {t(`identity.presets.${preset.key}`)}
              </Button>
            ))}
          </div>
          <Textarea
            id="agent-personality"
            value={draft.personality}
            onChange={(e) => update("personality", e.target.value)}
            maxLength={4000}
            rows={4}
            placeholder={t("identity.personalityPlaceholder")}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="agent-instructions">
            {t("identity.instructions")}
          </Label>
          <Textarea
            id="agent-instructions"
            value={draft.instructions}
            onChange={(e) => update("instructions", e.target.value)}
            maxLength={8000}
            rows={4}
            placeholder={t("identity.instructionsPlaceholder")}
          />
          <p className="text-muted-foreground text-xs">
            {t("identity.instructionsHint")}
          </p>
        </div>
        <div className="space-y-2">
          <Label htmlFor="agent-greeting">{t("identity.greeting")}</Label>
          <Input
            id="agent-greeting"
            value={draft.greeting}
            onChange={(e) => update("greeting", e.target.value)}
            maxLength={300}
            placeholder={t("identity.greetingPlaceholder")}
          />
        </div>
        <Button
          type="submit"
          disabled={!draft.name.trim()}
          className="w-full gap-2 sm:w-auto"
        >
          {save.isPending ? (
            <IconLoader2 className="size-4 animate-spin" />
          ) : (
            <IconSparkles className="size-4" />
          )}
          {profile.configured
            ? t("identity.save")
            : t("identity.spawn", { name })}
        </Button>
        {save.error && (
          <p role="alert" className="text-destructive text-sm">
            {save.error.message}
          </p>
        )}
        {saved && (
          <p role="status" className="flex items-center gap-2 text-sm">
            <IconCheck className="size-4" />
            {t("identity.saved", { name })}
          </p>
        )}
      </fieldset>
      <aside className="bg-muted/30 h-fit rounded-2xl border p-5 md:sticky md:top-4">
        <p className="text-muted-foreground mb-6 text-xs font-medium tracking-widest uppercase">
          {t("identity.preview")}
        </p>
        <div
          className="bg-background mb-4 flex size-16 items-center justify-center rounded-2xl border text-4xl"
          aria-hidden="true"
        >
          {draft.avatar || "🦞"}
        </div>
        <h3 className="text-xl font-semibold break-words">{name}</h3>
        <p className="text-muted-foreground mt-1 text-sm break-words">
          {draft.role || t("identity.rolePlaceholder")}
        </p>
        <div className="bg-background mt-5 rounded-xl border p-3 text-sm break-words">
          {draft.greeting || t("identity.greetingPlaceholder")}
        </div>
        <p className="text-muted-foreground mt-4 text-xs">
          {t("identity.previewHint")}
        </p>
      </aside>
    </form>
  )
}
