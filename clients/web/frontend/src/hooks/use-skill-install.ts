import { useMutation, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { toast } from "sonner"

import {
  type InstallSkillResponse,
  type SkillRegistrySearchResult,
  installSkill,
} from "@/api/skills"

/**
 * Shared registry-install mutation used by every place that can install a
 * skill (the Hub marketplace and the Find Skills dialog on the Skills page).
 * A successful install refreshes the installed-skill list and any in-flight
 * marketplace searches so cards flip to their "Installed" state.
 */
export function useSkillInstall() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()

  const installMutation = useMutation({
    mutationFn: installSkill,
    onSuccess: (response: InstallSkillResponse) => {
      toast.success(
        t("pages.agent.skills.install_success", {
          name: response.skill?.name ?? response.slug,
        }),
      )
      void queryClient.invalidateQueries({ queryKey: ["skills"] })
      void queryClient.invalidateQueries({ queryKey: ["skills-marketplace"] })
    },
    onError: (err) => {
      toast.error(
        err instanceof Error
          ? err.message
          : t("pages.agent.skills.install_error"),
      )
    },
  })

  const pendingKey =
    installMutation.isPending && installMutation.variables
      ? `${installMutation.variables.registry}:${installMutation.variables.slug}`
      : null

  return {
    handleInstall: (result: SkillRegistrySearchResult) => {
      installMutation.mutate({
        slug: result.slug,
        registry: result.registry_name,
        version: result.version || undefined,
      })
    },
    isInstallPending: (result: SkillRegistrySearchResult) =>
      pendingKey === `${result.registry_name}:${result.slug}`,
    isInstalling: installMutation.isPending,
  }
}
