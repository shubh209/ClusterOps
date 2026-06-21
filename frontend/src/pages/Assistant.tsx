import { useState } from 'react'
import { BrainCircuit, Terminal, Lightbulb, Target, ChevronDown, ChevronUp, Loader2 } from 'lucide-react'
import { useQuery } from '@/hooks/useQuery'
import { api } from '@/lib/api'
import { Card, CardHeader, CardTitle } from '@/components/shared/Card'
import { Badge } from '@/components/shared/Badge'
import { cn, alertSeverityBg, jobStatusBg, nodeStatusBg } from '@/lib/utils'
import type { AssistantAnalysis, Job, Node } from '@/types'

// ─── Analysis result display ──────────────────────────────────────────────────

function AnalysisCard({ analysis }: { analysis: AssistantAnalysis }) {
  const [stepsOpen, setStepsOpen] = useState(true)
  const [tipsOpen, setTipsOpen] = useState(false)

  return (
    <div className="space-y-4 animate-fade-in">
      {/* Headline */}
      <div className={cn(
        'rounded-lg border p-4',
        analysis.severity === 'critical' ? 'border-danger/40 bg-danger/8' :
        analysis.severity === 'warning'  ? 'border-warning/40 bg-warning/8' :
                                           'border-accent/40 bg-accent/8'
      )}>
        <div className="flex items-start justify-between gap-3 mb-2">
          <h2 className="text-sm font-semibold text-gray-100 leading-snug">{analysis.headline}</h2>
          <div className="flex items-center gap-2 shrink-0">
            <Badge className={cn(alertSeverityBg(analysis.severity))}>{analysis.severity}</Badge>
            <span className="text-[10px] text-muted font-mono">
              {Math.round(analysis.confidence * 100)}% confidence
            </span>
          </div>
        </div>
        <div className="flex items-center gap-2 mb-3">
          <Target className="h-3.5 w-3.5 text-muted" />
          <span className="text-xs font-semibold text-accent">{analysis.root_cause}</span>
        </div>
        <p className="text-xs text-gray-300 leading-relaxed">{analysis.summary}</p>
      </div>

      {/* Debugging steps */}
      {analysis.debugging_steps?.length > 0 && (
        <Card>
          <CardHeader>
            <button
              onClick={() => setStepsOpen(v => !v)}
              className="flex items-center gap-2 text-sm font-semibold text-gray-200 hover:text-accent transition-colors w-full text-left"
            >
              <Terminal className="h-4 w-4 text-accent" />
              Debugging Steps ({analysis.debugging_steps.length})
              {stepsOpen ? <ChevronUp className="h-3.5 w-3.5 ml-auto" /> : <ChevronDown className="h-3.5 w-3.5 ml-auto" />}
            </button>
          </CardHeader>
          {stepsOpen && (
            <div className="space-y-3">
              {analysis.debugging_steps.map((step) => (
                <div key={step.order} className="flex gap-3">
                  <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-accent/10 text-accent text-xs font-bold font-mono">
                    {step.order}
                  </div>
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-semibold text-gray-200 mb-1">{step.title}</p>
                    <p className="text-xs text-muted leading-relaxed">{step.description}</p>
                    {step.command && (
                      <div className="mt-2 rounded-md bg-surface border border-border px-3 py-2">
                        <pre className="text-[10px] font-mono text-gray-300 overflow-x-auto whitespace-pre-wrap break-all">
                          {step.command}
                        </pre>
                      </div>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </Card>
      )}

      {/* Prevention tips */}
      {analysis.prevention_tips?.length > 0 && (
        <Card>
          <CardHeader>
            <button
              onClick={() => setTipsOpen(v => !v)}
              className="flex items-center gap-2 text-sm font-semibold text-gray-200 hover:text-accent transition-colors w-full text-left"
            >
              <Lightbulb className="h-4 w-4 text-warning" />
              Prevention Tips ({analysis.prevention_tips.length})
              {tipsOpen ? <ChevronUp className="h-3.5 w-3.5 ml-auto" /> : <ChevronDown className="h-3.5 w-3.5 ml-auto" />}
            </button>
          </CardHeader>
          {tipsOpen && (
            <ul className="space-y-2">
              {analysis.prevention_tips.map((tip, i) => (
                <li key={i} className="flex items-start gap-2 text-xs text-muted">
                  <span className="text-warning mt-0.5">•</span>
                  <span className="leading-relaxed">{tip}</span>
                </li>
              ))}
            </ul>
          )}
        </Card>
      )}
    </div>
  )
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export function Assistant() {
  const [mode, setMode] = useState<'job' | 'node'>('job')
  const [selectedId, setSelectedId] = useState('')
  const [analysis, setAnalysis] = useState<AssistantAnalysis | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const jobsQuery = useQuery((s) => api.jobs.list(undefined, s), 'jobs-for-assistant')
  const nodesQuery = useQuery((s) => api.nodes.list(s), 'nodes-for-assistant')

  const jobs  = jobsQuery.status === 'success' ? jobsQuery.data : []
  const nodes = nodesQuery.status === 'success' ? nodesQuery.data : []

  async function analyze() {
    if (!selectedId) return
    setLoading(true)
    setError('')
    setAnalysis(null)
    try {
      const result = await api.assistant.analyze(
        mode === 'job' ? { job_id: selectedId } : { node_id: selectedId }
      )
      setAnalysis(result)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Analysis failed')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="p-6 space-y-6">
      <div>
        <h1 className="text-xl font-bold text-gray-100 flex items-center gap-2">
          <BrainCircuit className="h-5 w-5 text-accent" />
          AI Assistant
        </h1>
        <p className="text-xs text-muted mt-0.5">
          Rule-based failure analysis and debugging playbooks.
          Upgrade to Ollama (local LLM) for natural-language generation.
        </p>
      </div>

      {/* Target selector */}
      <Card>
        <div className="space-y-4">
          {/* Mode tabs */}
          <div className="flex gap-2">
            {(['job', 'node'] as const).map(m => (
              <button
                key={m}
                onClick={() => { setMode(m); setSelectedId(''); setAnalysis(null) }}
                className={cn(
                  'rounded-md px-4 py-2 text-sm font-medium transition-colors',
                  mode === m
                    ? 'bg-accent/10 text-accent border border-accent/30'
                    : 'text-muted border border-border hover:text-gray-200'
                )}
              >
                Analyze {m === 'job' ? 'Job' : 'Node'}
              </button>
            ))}
          </div>

          {/* Selector */}
          <div className="flex gap-2">
            <select
              value={selectedId}
              onChange={e => { setSelectedId(e.target.value); setAnalysis(null) }}
              className="flex-1 rounded-md border border-border bg-surface-2 px-3 py-2 text-sm text-gray-200 focus:outline-none focus:border-accent"
            >
              <option value="">Select a {mode}…</option>
              {mode === 'job'
                ? jobs.map((j: Job) => (
                    <option key={j.id} value={j.id}>
                      {j.name} ({j.status})
                    </option>
                  ))
                : nodes.map((n: Node) => (
                    <option key={n.id} value={n.id}>
                      {n.hostname.split('.')[0]} ({n.status})
                    </option>
                  ))
              }
            </select>
            <button
              onClick={analyze}
              disabled={!selectedId || loading}
              className={cn(
                'flex items-center gap-2 rounded-md px-5 py-2 text-sm font-semibold transition-colors',
                selectedId && !loading
                  ? 'bg-accent text-black hover:bg-accent/90'
                  : 'bg-surface-3 text-muted cursor-not-allowed'
              )}
            >
              {loading && <Loader2 className="h-4 w-4 animate-spin" />}
              Analyze
            </button>
          </div>

          {/* Quick-select failed jobs */}
          {mode === 'job' && jobs.filter((j: Job) => j.status === 'failed').length > 0 && (
            <div>
              <p className="text-[10px] text-muted mb-1.5">Recent failures — click to analyze:</p>
              <div className="flex flex-wrap gap-1.5">
                {jobs
                  .filter((j: Job) => j.status === 'failed')
                  .slice(0, 5)
                  .map((j: Job) => (
                    <button
                      key={j.id}
                      onClick={() => { setSelectedId(j.id); setAnalysis(null) }}
                      className={cn(
                        'rounded border px-2 py-1 text-[10px] font-mono transition-colors',
                        selectedId === j.id
                          ? 'border-accent bg-accent/10 text-accent'
                          : 'border-border text-muted hover:text-gray-200'
                      )}
                    >
                      {j.name.substring(0, 20)}…
                    </button>
                  ))
                }
              </div>
            </div>
          )}
        </div>
      </Card>

      {/* Error */}
      {error && (
        <div className="rounded-md border border-danger/30 bg-danger/10 p-3 text-xs text-danger">
          {error}
        </div>
      )}

      {/* Result */}
      {analysis && <AnalysisCard analysis={analysis} />}

      {/* Empty prompt */}
      {!analysis && !loading && !error && (
        <div className="flex flex-col items-center justify-center py-20 text-center">
          <BrainCircuit className="h-12 w-12 text-muted opacity-20 mb-4" />
          <p className="text-sm text-muted">Select a job or node and click Analyze</p>
          <p className="text-xs text-muted opacity-60 mt-1">
            The assistant will identify the root cause and generate a debugging playbook
          </p>
        </div>
      )}
    </div>
  )
}
