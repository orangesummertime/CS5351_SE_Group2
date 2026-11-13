import { describe, expect, it } from 'vitest'
import { filterModels, getModelCapabilities, getModelProvider, getModelProviderStats, getVisibleModelRows, groupModelsByProvider, mergeModelLists, modelsForTask, setModelsEnabled, sortModelsWithEnabledFirst } from './modelPicker'

describe('model picker helpers', () => {
  it('groups by provider prefix and puts prefixless models in custom', () => {
    expect(groupModelsByProvider(['group/one', 'generic/gpt', 'custom', 'group/two'])).toEqual([
      { provider: 'group', models: ['group/one', 'group/two'] }, { provider: 'generic', models: ['generic/gpt'] }, { provider: '其他/自定义', models: ['custom'] },
    ])
    expect(getModelProvider('siliconflow/Qwen/Qwen3')).toBe('siliconflow')
  })
  it('filters case-insensitively and supports empty results', () => {
    expect(filterModels(['OpenRouter/Free', 'generic/gpt'], 'openrouter')).toEqual(['OpenRouter/Free'])
    expect(filterModels(['OpenRouter/Free', 'generic/gpt'], 'missing')).toEqual([])
  })
  it('merges available and enabled models without losing enabled-only entries', () => {
    expect(mergeModelLists(['remote/model'], ['saved/model', 'remote/model'])).toEqual(['remote/model', 'saved/model'])
  })
  it('sorts enabled models first without duplicates', () => {
    expect(sortModelsWithEnabledFirst(['b', 'a', 'b', 'c'], ['c', 'a'])).toEqual(['a', 'c', 'b'])
  })
  it('reports provider totals and enabled counts', () => {
    expect(getModelProviderStats(['generic/a', 'generic/b', 'custom'], ['generic/b'])).toEqual([
      { provider: 'generic', models: ['generic/a', 'generic/b'], enabledCount: 1 }, { provider: '其他/自定义', models: ['custom'], enabledCount: 0 },
    ])
    expect(getModelProviderStats(['generic/a', 'generic/b'], ['generic/a', 'generic/b'])).toEqual([
      { provider: 'generic', models: ['generic/a', 'generic/b'], enabledCount: 2 },
    ])
  })
  it('uses strict translation-only detection and conservative defaults', () => {
    expect(getModelCapabilities('doubao-translate/fixture')).toEqual(['translation-only'])
    expect(getModelCapabilities('bilibili/Index-Translate-35B-A3B')).toEqual(['translation-only'])
    expect(getModelCapabilities('model/r1')).toEqual(['text'])
    expect(getModelCapabilities('model', { capabilities: ['vision', 'text'] })).toEqual(['vision', 'text'])
  })
  it('filters enabled models for translation, summary, and chat', () => {
    const models = ['doubao-translate/fixture', 'bilibili/Index-Translate-35B-A3B', 'general-chat']
    expect(modelsForTask(models, models, 'translation')).toEqual(models)
    expect(modelsForTask(models, models, 'summary')).toEqual(['general-chat'])
    expect(modelsForTask(models, models, 'chat')).toEqual(['general-chat'])
  })
  it('batch toggles without duplicates or losing manual models', () => {
    expect(setModelsEnabled(['group/one', 'manual/model'], ['manual/model'], true)).toEqual(['manual/model', 'group/one'])
    expect(setModelsEnabled(['group/one'], ['manual/model', 'group/one'], false)).toEqual(['manual/model'])
  })
  it('renders no group rows while collapsed and caps expanded groups at 50 rows', () => {
    const models = Array.from({ length: 80 }, (_, index) => `provider/model-${index}`)
    expect(getVisibleModelRows(models, false)).toEqual([])
    expect(getVisibleModelRows(models, true)).toHaveLength(50)
    expect(getVisibleModelRows(models, true, 80)).toHaveLength(80)
  })
})
