import { describe, expect, it } from 'vitest'
import { readSelection, writeSelection, type SelectionStorage } from './selection-store'

class MemoryStorage implements SelectionStorage {
  values = new Map<string, string>()
  getItem(name: string) { return this.values.get(name) ?? null }
  setItem(name: string, value: string) { this.values.set(name, value) }
  removeItem(name: string) { this.values.delete(name) }
}

describe('selection storage', () => {
  it('stores only a valid selected ID and can clear it', () => {
    const storage = new MemoryStorage()
    writeSelection('service.api', storage)
    expect(readSelection(storage)).toBe('service.api')
    writeSelection(undefined, storage)
    expect(readSelection(storage)).toBeUndefined()
  })

  it('rejects writes and removes corrupt reads', () => {
    const storage = new MemoryStorage()
    expect(() => writeSelection('../private', storage)).toThrow('invalid')
    storage.values.set('nicos-catalog:explorer:selected', 'bad/id')
    expect(readSelection(storage)).toBeUndefined()
    expect(storage.values.size).toBe(0)
  })
})
