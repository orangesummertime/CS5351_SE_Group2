/** Server-computed freshness of a stored chat answer or citation (FR-05). */
export type VersionStatus = 'current' | 'expired' | 'unavailable'

export type ChatCitationView = {
  id?: string
  quote?: string
  status?: string
  reason?: string
  freshness?: VersionStatus
  locator?: string
  sourceVersion?: string
  contentRevision?: number
}

export type ChatMessageView = {
  role: 'user' | 'assistant'
  content: string
  model?: string
  sourceRevision?: number
  sourceHash?: string
  versionStatus?: VersionStatus
  citations?: ChatCitationView[]
}

const labels: Record<VersionStatus, string> = {
  current: '当前有效',
  expired: '已过期',
  unavailable: '来源不可用',
}

/** User-visible label for a version status. Unknown values are not treated as current. */
export function versionStatusLabel(status: string | undefined): string {
  if (status === 'current' || status === 'expired' || status === 'unavailable') {
    return labels[status]
  }
  return ''
}

/** NFR-05: only an explicit "current" status may be shown as currently valid. */
export function isDisplayedAsCurrentlyValid(status: string | undefined): boolean {
  return status === 'current'
}

/**
 * Freshness is the source of truth for "currently valid". A citation that C
 * marked verified at generation time is not currently valid once it has
 * expired or become unavailable.
 */
export function citationDisplayStatus(citation: ChatCitationView): string {
  if (citation.freshness) return citation.freshness
  return ''
}
