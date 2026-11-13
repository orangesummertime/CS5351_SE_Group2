import { useEffect, useMemo, useRef, useState } from 'react'
import { APIError, request } from '../../shared/api/client'
import type { SubtitleTrack } from '../../shared/api/types'
import type { AIBudgetReport, TranslationInspection, TranslationResult } from './types'
import { buildTranslationRequest, budgetReportFromErrorPayload, parseCallLimit } from './callBudget'
import { usePageVisibility } from '../../shared/hooks/usePageVisibility'
import { preferredSubtitleLanguage, subtitleTargetLabel, subtitleTrackMatchesTarget } from '../subtitles/subtitleLanguageMatch'
import type { SubtitleLanguage } from '../../shared/api/types'

export function useSubtitleTranslation(contentId: string, track?: SubtitleTrack, source?: 'title' | 'article', targetLanguage = 'zh-Hans', platformLanguages: readonly SubtitleLanguage[] = [], platformLanguagesReady = true) {
  const visible = usePageVisibility()
  const nativeAvailable = source !== 'article' && Boolean(preferredSubtitleLanguage(platformLanguages, targetLanguage))
  const alreadyTarget = useMemo(() => Boolean(track && source !== 'article' && subtitleTrackMatchesTarget(track, targetLanguage)), [track, source, targetLanguage])
  const scope = useMemo(() => track ? JSON.stringify([contentId, track.language, targetLanguage, track.cues]) : '', [contentId, track, targetLanguage])
  const liveScope = useRef(scope); liveScope.current = scope
  const [inspection, setInspection] = useState<TranslationInspection | null>(null)
  const [dataScope, setDataScope] = useState('')
  const [loading, setLoading] = useState(false), [pending, setPending] = useState(false)
  const [error, setError] = useState(''), [notice, setNotice] = useState(''), [show, setShow] = useState(true)
  const [callLimitInput, setCallLimitInput] = useState('')
  const [budget, setBudget] = useState<AIBudgetReport | undefined>(undefined)
  const reader = useRef<AbortController | null>(null), mutation = useRef<AbortController | null>(null)
  const refreshOnReturn = useRef<() => void>(() => {})
  refreshOnReturn.current = () => {
    if (track && !mutation.current && !reader.current) void reload()
  }
  async function reload() {
    if (!track || alreadyTarget) return
    reader.current?.abort(); const current = new AbortController(); reader.current = current
    const requestedScope = scope; if (dataScope !== requestedScope) setLoading(true)
    try {
      const value = await request<TranslationInspection>('/ai/subtitles/inspect', { method: 'POST', signal: current.signal, body: JSON.stringify({ contentId, track, source, targetLanguage, revision: 0 }) })
      if (!current.signal.aborted && liveScope.current === requestedScope) { setDataScope(requestedScope); setInspection(value); setError('') }
    } catch (cause) { if (!current.signal.aborted && liveScope.current === requestedScope) setError(cause instanceof Error ? cause.message : '本机译文暂不可用。') }
    finally {
      if (reader.current === current) reader.current = null
      if (!current.signal.aborted && liveScope.current === requestedScope) setLoading(false)
    }
  }
  useEffect(() => {
    setInspection(null); setError(''); setNotice(''); setPending(false); setLoading(false); setShow(true)
    const shouldLoad = track && platformLanguagesReady && !alreadyTarget && !nativeAvailable
    const timer = shouldLoad ? setTimeout(() => void reload(), 0) : undefined
    return () => {
      if (timer) clearTimeout(timer)
      reader.current?.abort(); reader.current = null
      mutation.current?.abort(); mutation.current = null
      setPending(false)
    }
  }, [scope, alreadyTarget, nativeAvailable])
  useEffect(() => {
    if (!scope) return
    let timer: ReturnType<typeof setTimeout> | undefined
    const refresh = () => {
      if (timer) clearTimeout(timer)
      if (document.visibilityState === 'hidden') return
      timer = setTimeout(() => {
        if (document.visibilityState !== 'hidden') refreshOnReturn.current()
      }, 80)
    }
    window.addEventListener('focus', refresh)
    document.addEventListener('visibilitychange', refresh)
    return () => {
      if (timer) clearTimeout(timer)
      window.removeEventListener('focus', refresh)
      document.removeEventListener('visibilitychange', refresh)
    }
  }, [scope])
  useEffect(() => {
    if (nativeAvailable || !visible || pending || loading || error || dataScope !== scope || (!inspection?.active && !inspection?.settings.translating)) return
    // A canceled request can outlive its browser connection while the server
    // finishes a batch. Inspect its lock until release; never restart the model.
    // Each inspection is a new object, so every answer re-arms the next check.
    const timer = setTimeout(() => void reload(), 1000)
    return () => clearTimeout(timer)
  }, [nativeAvailable, visible, pending, loading, error, dataScope, scope, inspection])
  // regenerate replaces the stored translation with one from the current model.
  async function translate(regenerate = false) {
    if (alreadyTarget || nativeAvailable || !track || !inspection || dataScope !== scope || pending || mutation.current || inspection.active || inspection.settings.translating) return
    const parsedLimit = parseCallLimit(callLimitInput)
    if (!parsedLimit.ok) { setError(parsedLimit.message); return }
    const current = new AbortController(); mutation.current = current
    const requestedScope = scope, snapshot = inspection
    setPending(true); setError(''); setNotice(''); setShow(true); setBudget(undefined)
    if (regenerate) setInspection({ ...snapshot, result: { ...snapshot.result, texts: [] } })
    let timer: ReturnType<typeof setTimeout> | undefined
    async function progress() {
      if (current.signal.aborted) return
      try {
        const value = await request<TranslationResult>(`/ai/subtitles/${snapshot.result.key}`, { signal: current.signal })
        if (!current.signal.aborted && liveScope.current === requestedScope) setInspection(previous => previous ? { ...previous, result: value } : previous)
      } catch { /* Translation response is authoritative; a missing first cache is expected. */ }
      if (!current.signal.aborted) timer = setTimeout(() => void progress(), 1000)
    }
    timer = setTimeout(() => void progress(), 700)
    try {
      const value = await request<TranslationResult>('/ai/subtitles/translate', { method: 'POST', signal: current.signal, body: JSON.stringify(buildTranslationRequest({ contentId, track, source, targetLanguage, revision: snapshot.settings.revision }, parsedLimit.value, regenerate)) })
      if (!current.signal.aborted && liveScope.current === requestedScope) { setInspection({ ...snapshot, result: value, active: false }); setNotice(`${subtitleTargetLabel(targetLanguage)}译文已保存到本机。`); setBudget(value.budget) }
    } catch (cause) {
      if (!current.signal.aborted && liveScope.current === requestedScope) {
        if (cause instanceof APIError) {
          const report = budgetReportFromErrorPayload(cause.payload)
          if (report) setBudget(report)
        }
        // Recover only already committed batches. Never retry the model call.
        try { const value = await request<TranslationResult>(`/ai/subtitles/${snapshot.result.key}`, { signal: current.signal }); if (!current.signal.aborted && liveScope.current === requestedScope) setInspection({ ...snapshot, result: value, active: false }) } catch { /* preserve confirmed data */ }
        if (!current.signal.aborted && liveScope.current === requestedScope) setError(cause instanceof Error ? cause.message : '翻译未完成，已完成部分仍保留。')
      }
    } finally {
      if (timer) clearTimeout(timer)
      current.abort() // Also abort an in-flight progress reader.
      if (mutation.current === current) { mutation.current = null; if (liveScope.current === requestedScope) setPending(false) }
    }
  }
  function cancel() {
    mutation.current?.abort(); mutation.current = null; setPending(false)
    setNotice('已取消翻译，已完成部分保留。'); void reload()
  }
  const callLimitError = (() => { const parsed = parseCallLimit(callLimitInput); return parsed.ok ? '' : parsed.message })()
  return { alreadyChinese: alreadyTarget, alreadyTarget, nativeAvailable, targetLanguage, inspection: !alreadyTarget && !nativeAvailable && dataScope === scope ? inspection : null, loading: loading && !nativeAvailable, pending: pending && !nativeAvailable, error: nativeAvailable ? '' : error, notice: nativeAvailable ? '' : notice, show: show && !alreadyTarget && !nativeAvailable, setShow, budget, callLimitInput, setCallLimitInput, callLimitError, reload, translate, cancel }
}
