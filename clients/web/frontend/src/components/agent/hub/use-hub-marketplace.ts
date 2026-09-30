import { useInfiniteQuery, useQuery } from "@tanstack/react-query"
import { useNavigate } from "@tanstack/react-router"
import { type UIEvent, useEffect, useRef, useState } from "react"

import {
  type SkillSearchResponse,
  type SkillSupportItem,
  getSkills,
  searchSkills,
} from "@/api/skills"
import { useSkillInstall } from "@/hooks/use-skill-install"

const MARKET_SEARCH_LIMIT = 20

export function useHubMarketplace() {
  const navigate = useNavigate()
  const isLoadMoreLockedRef = useRef(false)

  const [marketQuery, setMarketQuery] = useState("")
  const [submittedMarketQuery, setSubmittedMarketQuery] = useState("")

  const { data: skillsData } = useQuery({
    queryKey: ["skills"],
    queryFn: getSkills,
  })

  const hasSubmittedQuery = submittedMarketQuery.trim() !== ""
  const isMarketSearchActive = hasSubmittedQuery

  const {
    data: marketSearchData,
    isPending: isMarketSearchPending,
    isFetching: isMarketSearchFetching,
    isFetchingNextPage,
    error: marketSearchError,
    hasNextPage,
    fetchNextPage,
    refetch: refetchMarketSearch,
  } = useInfiniteQuery({
    queryKey: ["skills-marketplace", submittedMarketQuery],
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      searchSkills(
        submittedMarketQuery,
        MARKET_SEARCH_LIMIT,
        Number(pageParam) || 0,
      ),
    getNextPageParam: (lastPage: SkillSearchResponse) =>
      lastPage.has_more ? (lastPage.next_offset ?? undefined) : undefined,
    enabled: isMarketSearchActive,
    staleTime: 5 * 60 * 1000,
    refetchOnMount: false,
    refetchOnWindowFocus: false,
  })

  const { handleInstall, isInstallPending } = useSkillInstall()

  const allSkills = skillsData?.skills ?? []
  const workspaceSkillsByName = new Map(
    allSkills
      .filter((skill) => skill.source === "workspace")
      .map((skill) => [skill.name, skill] as const),
  )
  const marketResults =
    marketSearchData?.pages.flatMap((page) => page.results) ?? []
  const hasMoreMarketResults = hasNextPage ?? false
  const isMarketSearchInitialLoading =
    isMarketSearchActive &&
    !marketSearchData &&
    (isMarketSearchPending || isMarketSearchFetching)
  const isMarketSearchLoadingMore =
    isMarketSearchActive && Boolean(marketSearchData) && isFetchingNextPage

  useEffect(() => {
    if (!isFetchingNextPage) {
      isLoadMoreLockedRef.current = false
    }
  }, [isFetchingNextPage])

  const handleSearchSubmit = () => {
    const nextQuery = marketQuery.trim()
    if (nextQuery === "") {
      return
    }

    isLoadMoreLockedRef.current = false
    if (nextQuery === submittedMarketQuery) {
      void refetchMarketSearch()
      return
    }

    setSubmittedMarketQuery(nextQuery)
  }

  const handleViewInstalled = () => {
    void navigate({ to: "/agent/skills" })
  }

  const handleScroll = (event: UIEvent<HTMLDivElement>) => {
    if (
      !isMarketSearchActive ||
      !hasMoreMarketResults ||
      isFetchingNextPage ||
      isLoadMoreLockedRef.current
    ) {
      return
    }

    const node = event.currentTarget
    const remaining = node.scrollHeight - node.scrollTop - node.clientHeight
    if (remaining > 240) {
      return
    }

    isLoadMoreLockedRef.current = true
    void fetchNextPage()
  }

  const getInstalledSkill = (
    installedName?: string,
  ): SkillSupportItem | null => {
    if (!installedName) {
      return null
    }
    return workspaceSkillsByName.get(installedName) ?? null
  }

  return {
    marketQuery,
    submittedMarketQuery,
    hasSubmittedQuery,
    marketResults,
    marketSearchError,
    isMarketSearchInitialLoading,
    isMarketSearchLoadingMore,
    setMarketQuery,
    handleSearchSubmit,
    handleInstall,
    handleViewInstalled,
    handleScroll,
    getInstalledSkill,
    isInstallPending,
  }
}
