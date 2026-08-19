import { describe, expect, it, vi } from 'vitest'
import { recoverChunkLoad, type RetryStorage } from './chunk-reload'

class MemoryStorage implements RetryStorage {
  values = new Map<string, string>()
  getItem(name: string) { return this.values.get(name) ?? null }
  setItem(name: string, value: string) { this.values.set(name, value) }
  removeItem(name: string) { this.values.delete(name) }
}

describe('lazy chunk recovery', () => {
  it('clears the retry marker after a successful load', async () => {
    const storage = new MemoryStorage()
    storage.setItem('route', 'attempted')
    await expect(recoverChunkLoad(async () => 42, 'route', storage, vi.fn())).resolves.toBe(42)
    expect(storage.getItem('route')).toBeNull()
  })

  it('reloads once and then lets the route boundary handle a repeated failure', async () => {
    const storage = new MemoryStorage()
    const reload = vi.fn()
    const failure = () => Promise.reject(new Error('old chunk'))
    await expect(recoverChunkLoad(failure, 'route', storage, reload)).rejects.toThrow('old chunk')
    await expect(recoverChunkLoad(failure, 'route', storage, reload)).rejects.toThrow('old chunk')
    expect(reload).toHaveBeenCalledTimes(1)
    expect(storage.getItem('route')).toBe('attempted')
  })
})
