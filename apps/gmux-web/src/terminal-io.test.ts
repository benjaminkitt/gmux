import { describe, expect, it, vi } from 'vitest'
import { createTerminalIO } from './terminal-io'

function makeHarness() {
  const writes: Array<string> = []
  const resizes: Array<{ cols: number, rows: number }> = []
  const pending: Array<() => void> = []

  const io = createTerminalIO({
    write(data, callback) {
      writes.push(typeof data === 'string' ? data : new TextDecoder().decode(data))
      pending.push(() => callback?.())
    },
    resize(cols, rows) {
      resizes.push({ cols, rows })
    },
  })

  return {
    io,
    writes,
    resizes,
    flushOne() {
      const cb = pending.shift()
      if (!cb) throw new Error('no pending write callback')
      cb()
    },
    flushAll() {
      while (pending.length) pending.shift()?.()
    },
  }
}

const enc = (s: string) => new TextEncoder().encode(s)

describe('createTerminalIO', () => {
  it('serializes writes one at a time', () => {
    const h = makeHarness()
    h.io.reset(1)

    h.io.enqueue(enc('a'), 1)
    h.io.enqueue(enc('b'), 1)
    h.io.enqueue(enc('c'), 1)

    expect(h.writes).toEqual(['a'])

    h.flushOne()
    expect(h.writes).toEqual(['a', 'b'])

    h.flushOne()
    expect(h.writes).toEqual(['a', 'b', 'c'])
  })

  it('waits for queued writes before resizing', () => {
    const h = makeHarness()
    h.io.reset(1)

    h.io.enqueue(enc('hello'), 1)
    h.io.requestResize({ cols: 120, rows: 40 }, 1)

    expect(h.resizes).toEqual([])
    h.flushOne()
    expect(h.resizes).toEqual([{ cols: 120, rows: 40 }])
  })

  it('coalesces to the latest pending resize', () => {
    const h = makeHarness()
    h.io.reset(1)

    h.io.enqueue(enc('hello'), 1)
    h.io.requestResize({ cols: 100, rows: 30 }, 1)
    h.io.requestResize({ cols: 140, rows: 50 }, 1)

    h.flushOne()
    expect(h.resizes).toEqual([{ cols: 140, rows: 50 }])
  })

  it('drops stale queued writes and resizes after epoch reset', () => {
    const h = makeHarness()
    const onWritten = vi.fn()

    h.io.reset(1)
    h.io.enqueue(enc('stale'), 1, onWritten)
    h.io.requestResize({ cols: 90, rows: 20 }, 1)
    h.io.reset(2)
    h.io.enqueue(enc('fresh'), 2)

    expect(h.writes).toEqual(['stale', 'fresh'])
    h.flushOne()

    expect(onWritten).not.toHaveBeenCalled()
    expect(h.writes).toEqual(['stale', 'fresh'])
    expect(h.resizes).toEqual([])
  })

  it('runs completion callback after the final chunk in enqueueMany', () => {
    const h = makeHarness()
    const done = vi.fn()
    h.io.reset(1)

    h.io.enqueueMany([enc('a'), enc('b'), enc('c')], 1, done)
    h.flushAll()

    expect(h.writes).toEqual(['a', 'b', 'c'])
    expect(done).toHaveBeenCalledTimes(1)
  })
})
