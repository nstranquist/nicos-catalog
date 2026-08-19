import { describe, expect, it } from 'vitest'
import { invalidCatalogParams, invalidGraphParams, parseCatalogSearch, parseGraphSearch } from './url-state'

describe('catalog URL state', () => {
  it('accepts the documented allowlist and removes empty values', () => {
    expect(parseCatalogSearch({ q: '  owner graph ', kind: 'service', sort: 'kind', direction: 'desc', selected: 'service.api', cursor: 'abc' })).toEqual({
      q: 'owner graph', kind: 'service', sort: 'kind', direction: 'desc', selected: 'service.api', cursor: 'abc',
    })
    expect(parseCatalogSearch({ q: ' ', sort: 'random', selected: '../bad', status: ['active', 'old'] })).toEqual({ status: 'active' })
  })

  it('reports unsupported, repeated, and invalid URL values without throwing', () => {
    const params = new URLSearchParams('q=good&q=again&sort=nope&secret=value&selected=bad%2Fid')
    expect(invalidCatalogParams(params)).toEqual(['q', 'secret', 'selected', 'sort'])
  })
})

describe('graph URL state', () => {
  it('parses bounded modes and depths', () => {
    expect(parseGraphSearch({ mode: 'neighborhood', group_by: 'surface', id: 'service.api', depth: 2 })).toEqual({ mode: 'neighborhood', group_by: 'surface', id: 'service.api', depth: 2 })
    expect(parseGraphSearch({ mode: 'full', group_by: 'owner', id: 'bad/id', depth: 3, group: '' })).toEqual({})
  })

  it('reports malformed graph parameters', () => {
    expect(invalidGraphParams(new URLSearchParams('mode=full&depth=9&x=1&group=a&group=b'))).toEqual(['depth', 'group', 'mode', 'x'])
  })
})
