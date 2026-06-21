// Typed API client — all backend calls go through here.
// Uses fetch with abort signal support for React strict-mode compatibility.

import type {
  Alert,
  AssistantAnalysis,
  CapacityRow,
  ClusterGPUSummary,
  ClusterSummary,
  GPUTimeSeries,
  Job,
  Node,
} from '@/types'

const BASE = '/api/v1'

async function get<T>(path: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(`${BASE}${path}`, { signal })
  if (!res.ok) {
    const body = await res.text()
    throw new Error(`GET ${path} → ${res.status}: ${body}`)
  }
  return res.json() as Promise<T>
}

async function post<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const text = await res.text()
    throw new Error(`POST ${path} → ${res.status}: ${text}`)
  }
  return res.json() as Promise<T>
}

// ─── Cluster ─────────────────────────────────────────────────────────────────
export const api = {
  cluster: {
    summary: (signal?: AbortSignal) =>
      get<ClusterSummary>('/cluster/summary', signal),
  },

  nodes: {
    list: (signal?: AbortSignal) =>
      get<Node[]>('/nodes', signal),
    get: (id: string, signal?: AbortSignal) =>
      get<Node>(`/nodes/${id}`, signal),
    gpuSeries: (id: string, from?: string, to?: string, signal?: AbortSignal) => {
      const params = new URLSearchParams()
      if (from) params.set('from', from)
      if (to) params.set('to', to)
      const qs = params.toString() ? `?${params}` : ''
      return get<GPUTimeSeries[]>(`/nodes/${id}/gpu-series${qs}`, signal)
    },
  },

  jobs: {
    list: (params?: { status?: string; user_id?: string }, signal?: AbortSignal) => {
      const qs = params ? `?${new URLSearchParams(params as Record<string, string>)}` : ''
      return get<Job[]>(`/jobs${qs}`, signal)
    },
    get: (id: string, signal?: AbortSignal) =>
      get<Job>(`/jobs/${id}`, signal),
    alerts: (id: string, signal?: AbortSignal) =>
      get<Alert[]>(`/jobs/${id}/alerts`, signal),
  },

  metrics: {
    gpu: (signal?: AbortSignal) =>
      get<ClusterGPUSummary>('/metrics/gpu', signal),
    capacity: (signal?: AbortSignal) =>
      get<CapacityRow[]>('/metrics/capacity', signal),
  },

  alerts: {
    list: (activeOnly = false, signal?: AbortSignal) => {
      const qs = activeOnly ? '?active=true' : ''
      return get<Alert[]>(`/alerts${qs}`, signal)
    },
  },

  assistant: {
    analyze: (payload: { job_id?: string; node_id?: string }) =>
      post<AssistantAnalysis>('/assistant/analyze', payload),
  },
}
