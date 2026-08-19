import { lazy, type ComponentType, type LazyExoticComponent } from 'react'

export interface RetryStorage {
  getItem(name: string): string | null
  setItem(name: string, value: string): void
  removeItem(name: string): void
}

export async function recoverChunkLoad<T>(
  load: () => Promise<T>,
  key: string,
  storage: RetryStorage = sessionStorage,
  reload: () => void = () => location.reload(),
): Promise<T> {
  try {
    const module = await load()
    storage.removeItem(key)
    return module
  } catch (error) {
    if (storage.getItem(key) !== 'attempted') {
      storage.setItem(key, 'attempted')
      reload()
    }
    throw error
  }
}

export function lazyWithRetry<T extends ComponentType<object>>(
  key: string,
  load: () => Promise<{ default: T }>,
): LazyExoticComponent<T> {
  return lazy(() => recoverChunkLoad(load, `nicos-catalog:chunk:${key}`))
}
