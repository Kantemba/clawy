import { useInfiniteQuery, useQuery } from "@tanstack/react-query"
import { useState } from "react"

import {
  type SkillRegistrySearchResult,
  type SkillSearchResponse,
  type SkillSupportItem,
  getSkills,
  searchSkills,
} from "@/api/skills"
import { useSkillInstall } from "@/hooks/use-skill-install"

export const FIND_SKILLS_PAGE_SIZE = 12

/**
 * Search + install state for the Find Skills dialog. Queries share the
 * `skills-marketplace` cache with the Hub so switching between the two never
 * re-downloads the same pages.
 */
export function useFindSkills(enabled: boolean) {
  const [query, setQuery] = useState("")
  const [submittedQuery, setSubmittedQuery] = useState("")

  const trimmedQuery = submittedQuery.trim()
  const searchEnabled = enabled && trimmedQuery !== ""

  const { data: skillsData } = useQuery({
    queryKey: ["skills"],
    queryFn: getSkills,
  })

  const {
    data,
    isPending,
    isFetching,
    isFetchingNextPage,
    error,
    hasNextPage,
    fetchNextPage,
    refetch,
  } = useInfiniteQuery({
    queryKey: ["skills-marketplace", submittedQuery],
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      searchSkills(
        submittedQuery,
        FIND_SKILLS_PAGE_SIZE,
        Number(pageParam) || 0,
      ),
    getNextPageParam: (lastPage: SkillSearchResponse) =>
      lastPage.has_more ? (lastPage.next_offset ?? undefined) : undefined,
    enabled: searchEnabled,
    staleTime: 5 * 60 * 1000,
  })

  const { handleInstall, isInstallPending } = useSkillInstall()

  const workspaceSkillsByName = new Map(
    (skillsData?.skills ?? [])
      .filter((skill) => skill.source === "workspace")
      .map((skill) => [skill.name, skill] as const),
  )

  const results =
    data?.pages.flatMap((page) => page.results) ??
    ([] as SkillRegistrySearchResult[])
  const isInitialLoading = searchEnabled && !data && (isPending || isFetching)

  return {
    query,
    submittedQuery: trimmedQuery,
    results,
    error,
    hasSearched: trimmedQuery !== "",
    hasNextPage: hasNextPage ?? false,
    isInitialLoading,
    isFetchingMore: Boolean(data) && isFetchingNextPage,
    setQuery,
    submit: () => {
      const nextQuery = query.trim()
      if (nextQuery === "") {
        return
      }
      if (nextQuery === trimmedQuery) {
        void refetch()
        return
      }
      setSubmittedQuery(nextQuery)
    },
    handleInstall,
    isInstallPending,
    loadMore: () => {
      void fetchNextPage()
    },
    getInstalledSkill: (installedName?: string): SkillSupportItem | null => {
      if (!installedName) {
        return null
      }
      return workspaceSkillsByName.get(installedName) ?? null
    },
  }
}
