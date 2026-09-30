import { IconLoader2, IconSearch, IconX } from "@tabler/icons-react"
import type { FormEvent, ReactNode } from "react"
import { useTranslation } from "react-i18next"

import { MarketSkillCard } from "@/components/agent/hub/market-skill-card"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"

import { useFindSkills } from "./use-find-skills"

/**
 * Registry search + install, embedded in the Skills page so skills can be
 * found and installed without leaving the list they end up in.
 */
export function FindSkillsDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const find = useFindSkills(open)

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    find.submit()
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="border-border/40 bg-card/95 flex max-h-[85vh] w-[min(940px,92vw)] flex-col gap-4 p-6 shadow-lg backdrop-blur-sm focus:outline-none sm:max-w-[940px] sm:rounded-2xl">
        <DialogHeader className="pr-8">
          <DialogTitle className="text-lg font-semibold tracking-tight">
            {t("pages.agent.skills.find_title")}
          </DialogTitle>
          <DialogDescription>
            {t("pages.agent.skills.find_description")}
          </DialogDescription>
        </DialogHeader>

        <form className="flex flex-col gap-2 sm:flex-row" onSubmit={handleSubmit}>
          <Input
            value={find.query}
            onChange={(event) => find.setQuery(event.target.value)}
            placeholder={t("pages.agent.skills.marketplace_search_placeholder")}
            className="h-10 flex-1"
            autoFocus
          />
          <Button
            type="submit"
            disabled={find.query.trim() === "" || find.isInitialLoading}
          >
            {find.isInitialLoading ? (
              <IconLoader2 className="size-4 animate-spin" />
            ) : (
              <IconSearch className="size-4" />
            )}
            {t("pages.agent.skills.marketplace_search_action")}
          </Button>
        </form>

        <div className="min-h-0 flex-1 overflow-auto pr-1">
          {!find.hasSearched ? (
            <StateBox icon={<IconSearch className="size-6" />}>
              {t("pages.agent.skills.find_idle")}
            </StateBox>
          ) : find.isInitialLoading ? (
            <StateBox icon={<IconLoader2 className="size-6 animate-spin" />}>
              {t("pages.agent.skills.marketplace_loading_results")}
            </StateBox>
          ) : find.error ? (
            <div className="border-destructive/20 bg-destructive/5 rounded-xl border px-4 py-3">
              <div className="text-destructive flex items-center gap-3 text-sm">
                <IconX className="size-5 shrink-0" />
                <span className="font-medium">
                  {find.error instanceof Error
                    ? find.error.message
                    : t("pages.agent.skills.marketplace_search_error")}
                </span>
              </div>
            </div>
          ) : find.results.length ? (
            <div className="space-y-4">
              <div className="border-border rounded-xl border bg-muted px-4 py-3 text-sm">
                <div className="font-semibold">
                  {t("pages.agent.skills.marketplace_notice_title")}
                </div>
                <div className="text-muted-foreground mt-1 leading-6">
                  {t("pages.agent.skills.marketplace_notice_body")}
                </div>
              </div>

              <div className="border-border/40 flex items-center justify-between border-b pb-3">
                <h3 className="text-foreground/85 text-sm font-semibold">
                  {t("pages.agent.skills.marketplace_results_title", {
                    query: find.submittedQuery,
                    count: find.results.length,
                  })}
                </h3>
                <span className="text-muted-foreground text-xs font-medium">
                  {t("pages.agent.skills.marketplace_results_hint")}
                </span>
              </div>

              <div className="grid gap-3 lg:grid-cols-2">
                {find.results.map((result) => (
                  <MarketSkillCard
                    key={`${result.registry_name}:${result.slug}`}
                    result={result}
                    installPending={find.isInstallPending(result)}
                    installedSkill={find.getInstalledSkill(
                      result.installed_name,
                    )}
                    onInstall={() => find.handleInstall(result)}
                    onViewInstalled={() => onOpenChange(false)}
                  />
                ))}
              </div>

              {find.hasNextPage ? (
                <div className="flex justify-center pt-1">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={find.loadMore}
                    disabled={find.isFetchingMore}
                  >
                    {find.isFetchingMore ? (
                      <IconLoader2 className="size-4 animate-spin" />
                    ) : null}
                    {t("pages.agent.skills.find_load_more")}
                  </Button>
                </div>
              ) : null}
            </div>
          ) : (
            <StateBox icon={<IconSearch className="size-6" />}>
              {t("pages.agent.skills.marketplace_empty_results", {
                query: find.submittedQuery,
              })}
            </StateBox>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}

function StateBox({
  icon,
  children,
}: {
  icon: ReactNode
  children: ReactNode
}) {
  return (
    <div className="border-border/40 bg-muted/10 flex min-h-[180px] flex-col items-center justify-center gap-3 rounded-xl border border-dashed px-6 text-center">
      <span className="text-muted-foreground/60">{icon}</span>
      <span className="text-muted-foreground max-w-[420px] text-sm font-medium">
        {children}
      </span>
    </div>
  )
}
