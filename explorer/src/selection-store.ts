const key = 'nicos-catalog:explorer:selected'
const validID = /^[a-z0-9](?:[a-z0-9._-]{0,126}[a-z0-9])?$/

export interface SelectionStorage {
  getItem(name: string): string | null
  setItem(name: string, value: string): void
  removeItem(name: string): void
}

export function readSelection(storage: SelectionStorage = sessionStorage): string | undefined {
  const value = storage.getItem(key)
  if (!value) return undefined
  if (!validID.test(value)) {
    storage.removeItem(key)
    return undefined
  }
  return value
}

export function writeSelection(value: string | undefined, storage: SelectionStorage = sessionStorage): void {
  if (!value) {
    storage.removeItem(key)
    return
  }
  if (!validID.test(value)) throw new Error('The selected entity ID is invalid.')
  storage.setItem(key, value)
}
