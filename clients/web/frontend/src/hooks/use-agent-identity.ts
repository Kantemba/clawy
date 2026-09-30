import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"

import {
  defaultIdentity,
  getAgentIdentity,
  saveAgentIdentity,
} from "@/api/identity"

const identityQueryKey = ["agent-identity"]

export function useAgentIdentity() {
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: identityQueryKey,
    queryFn: getAgentIdentity,
    staleTime: 30_000,
  })
  const save = useMutation({
    mutationFn: saveAgentIdentity,
    onSuccess: (profile) => {
      queryClient.setQueryData(identityQueryKey, profile)
    },
  })
  return { ...query, identity: query.data ?? defaultIdentity, save }
}
