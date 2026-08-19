import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const styles = readFileSync('src/styles.css', 'utf8')

describe('Explorer accessibility style contract', () => {
  it('keeps motion, increased-contrast, and forced-colors policies', () => {
    expect(styles).toContain('@media (prefers-reduced-motion: reduce)')
    expect(styles).toContain('@media (prefers-contrast: more)')
    expect(styles).toContain('@media (forced-colors: active)')
  })

  it('keeps text and focus tokens at WCAG AA contrast in both themes', () => {
    const light = variables(blocks(':root'))
    const dark = variables(blocks(":root[data-theme='dark']"))
    for (const [theme, palette] of Object.entries({ light, dark })) {
      for (const token of ['ink', 'muted', 'accent', 'good', 'warn', 'bad', 'focus']) {
        expect(contrast(palette.paper, palette[token]), `${theme} ${token}`).toBeGreaterThanOrEqual(4.5)
      }
    }
  })
})

function blocks(selector: string): string {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const matches = [...styles.matchAll(new RegExp(`${escaped}\\s*\\{([^}]+)\\}`, 'g'))]
  if (matches.length === 0) throw new Error(`missing CSS block: ${selector}`)
  return matches.map((match) => match[1]).join('\n')
}

function variables(css: string): Record<string, string> {
  return Object.fromEntries([...css.matchAll(/--([a-z-]+):\s*(#[0-9a-f]{6})/gi)].map((match) => [match[1], match[2]]))
}

function contrast(first: string, second: string): number {
  const values = [relativeLuminance(first), relativeLuminance(second)].sort((a, b) => b - a)
  return (values[0] + 0.05) / (values[1] + 0.05)
}

function relativeLuminance(hex: string): number {
  const channels = hex.match(/[0-9a-f]{2}/gi)?.map((part) => Number.parseInt(part, 16) / 255)
  if (!channels || channels.length !== 3) throw new Error(`invalid color: ${hex}`)
  const linear = channels.map((channel) => channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4)
  return 0.2126 * linear[0] + 0.7152 * linear[1] + 0.0722 * linear[2]
}
