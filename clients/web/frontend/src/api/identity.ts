import { launcherFetch } from "@/api/http"

export interface AgentIdentity {
  name: string
  avatar: string
  role: string
  personality: string
  instructions: string
  greeting: string
  configured: boolean
}

export const defaultIdentity: AgentIdentity = {
  name: "Clawy",
  avatar: "🦞",
  role: "Personal AI assistant",
  personality: "Calm, helpful, and practical.",
  instructions: "",
  greeting: "How can I help you today?",
  configured: false,
}

async function readIdentityResponse(
  response: Response,
): Promise<AgentIdentity> {
  if (!response.ok) {
    throw new Error((await response.text()) || "Unable to load agent identity")
  }
  return response.json() as Promise<AgentIdentity>
}

export async function getAgentIdentity(): Promise<AgentIdentity> {
  return readIdentityResponse(await launcherFetch("/api/agent/identity"))
}

export async function saveAgentIdentity(
  identity: AgentIdentity,
): Promise<AgentIdentity> {
  return readIdentityResponse(
    await launcherFetch("/api/agent/identity", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(identity),
    }),
  )
}
