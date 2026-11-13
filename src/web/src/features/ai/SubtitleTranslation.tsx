import { Button, Checkbox, Select } from '../../components/ui'
import { Languages } from 'lucide-react'
import { useRef } from 'react'
import type { useSubtitleTranslation } from './useSubtitleTranslation'
import { translationTargets } from '../subtitles/subtitleLanguageMatch'
import { CallLimitField } from './CallLimitField'
import './ai.css'
export function SubtitleTranslation({ translation, targetLabel = '简体中文', nativeTarget, targetLanguage = 'zh-Hans', onTargetLanguageChange, hasSourceTrack = true }: { translation: ReturnType<typeof useSubtitleTranslation>; targetLabel?: string; nativeTarget?: { label: string; language: string; source?: string }; targetLanguage?: string; onTargetLanguageChange?: (language: string) => void; hasSourceTrack?: boolean }) {
  const { inspection, loading, pending, error, notice, budget, callLimitInput, setCallLimitInput, callLimitError } = translation
  const done = inspection?.result.texts.length ?? 0, total = inspection?.result.total ?? 0
  const options = useRef<HTMLDetailsElement>(null)
  const targetSelector = <label className="subtitle-translation-target">翻译目标<Select aria-label="翻译目标语言" value={targetLanguage} onChange={event => onTargetLanguageChange?.(event.target.value)}>{translationTargets.map(([code, label]) => <option key={code} value={code}>{label}</option>)}</Select></label>
  const showAIControls = hasSourceTrack && !nativeTarget && !translation.alreadyTarget
  return <div className="subtitle-translation">
    <div className="subtitle-translation-bar">{targetSelector}
      {showAIControls && !pending && <details ref={options} className="subtitle-translation-options"><summary><Languages size={14} aria-hidden="true" />双语翻译{done > 0 && <span>{done === total ? '已保存' : `${done}/${total}`}</span>}</summary>
        <div className="subtitle-translation-body">
          {loading && !inspection ? <p role="status">正在读取本机译文…</p> : inspection && <>
            {inspection.settings.ready ? <p>{done < total ? `将这份字幕的 ${total} 段译为${targetLabel}。` : `本机已有完整的${targetLabel}译文。`}服务：<span className="ai-provider-address">{new URL(inspection.settings.baseUrl).host}</span>，模型：{inspection.settings.model}。</p> : <p>先到工具设置填写自己的翻译接口与模型。</p>}
            <CallLimitField id="translation-call-limit" value={callLimitInput} onChange={setCallLimitInput} error={callLimitError} budget={budget}/>
            <div className="subtitle-translation-actions">
              {done < total && <Button variant="secondary" className="button button-secondary" disabled={!inspection.settings.ready || loading || inspection.active || inspection.settings.translating || Boolean(callLimitError)} onClick={() => { if (options.current) options.current.open = false; void translation.translate() }}>{done ? '继续翻译' : '开始翻译整份字幕'}</Button>}
              {total > 0 && done === total && inspection.settings.ready && <Button variant="quiet" className="text-button" title={`现有译文由 ${inspection.result.model} 生成`} disabled={loading || inspection.active || inspection.settings.translating || Boolean(callLimitError)} onClick={() => { if (options.current) options.current.open = false; void translation.translate(true) }}>用当前模型重新翻译</Button>}
              <Button variant="quiet" className="text-button" disabled={loading} onClick={() => void translation.reload()}>重新读取设置与译文</Button>
            </div>
            {inspection.active && <p role="status">另一个页面正在翻译这份字幕，可重新读取进度。</p>}
          </>}
          {!inspection && !loading && <Button variant="quiet" className="text-button" onClick={() => void translation.reload()}>重新读取翻译设置</Button>}
        </div>
      </details>}
      {showAIControls && pending && <><span role="status">正在翻译，已完成 {done} / {total} 段</span><Button variant="quiet" className="text-button" onClick={translation.cancel}>取消翻译</Button></>}
      {showAIControls && done > 0 && <label className="subtitle-show-translation"><Checkbox type="checkbox" checked={translation.show} onChange={event => translation.setShow(event.target.checked)} />显示{targetLabel}译文</label>}
    </div>
    {showAIControls && notice && <p className="ai-translation-feedback" role="status">{notice}</p>}{showAIControls && error && <p className="ai-translation-feedback" role="alert">{error}</p>}
  </div>
}
