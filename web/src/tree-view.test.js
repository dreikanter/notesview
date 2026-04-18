import { describe, it, expect, beforeEach, vi } from 'vitest'
import { TreeView } from './tree-view.js'

function makeLoader(tree) {
  return vi.fn(async (path) => (tree[path] || []).slice())
}

const twoRoot = {
  '': [
    { name: 'a', path: 'a', isDir: true },
    { name: 'readme.md', path: 'readme.md', isDir: false },
  ],
}

describe('TreeView construction', () => {
  let container
  beforeEach(() => {
    document.body.innerHTML = '<div id="host"></div>'
    container = document.getElementById('host')
  })

  it('wipes the container and mounts a tv-root with role="tree"', async () => {
    container.innerHTML = '<p>stale</p>'
    const tv = new TreeView(container, { loader: makeLoader(twoRoot) })
    await tv.ready
    expect(container.querySelector('p')).toBeNull()
    expect(container.querySelector('.tv-root')).toBeTruthy()
    expect(container.querySelector('.tv-root').getAttribute('role')).toBe('tree')
  })

  it('calls loader with rootPath on construction and renders returned children', async () => {
    const loader = makeLoader(twoRoot)
    const tv = new TreeView(container, { loader })
    await tv.ready
    expect(loader).toHaveBeenCalledWith('')
    const items = container.querySelectorAll('[role="treeitem"]')
    expect(items.length).toBe(2)
    expect(items[0].getAttribute('data-path')).toBe('a')
    expect(items[0].classList.contains('tv-item--dir')).toBe(true)
    expect(items[1].getAttribute('data-path')).toBe('readme.md')
    expect(items[1].classList.contains('tv-item--file')).toBe(true)
  })

  it('uses rootPath option when provided', async () => {
    const loader = makeLoader({ 'sub': [{ name: 'x.md', path: 'sub/x.md', isDir: false }] })
    const tv = new TreeView(container, { loader, rootPath: 'sub' })
    await tv.ready
    expect(loader).toHaveBeenCalledWith('sub')
    expect(container.querySelector('[data-path="sub/x.md"]')).toBeTruthy()
  })

  it('honors classPrefix option', async () => {
    const tv = new TreeView(container, {
      loader: makeLoader(twoRoot),
      classPrefix: 'x-',
    })
    await tv.ready
    expect(container.querySelector('.x-root')).toBeTruthy()
    expect(container.querySelector('.tv-root')).toBeNull()
    const item = container.querySelector('[role="treeitem"]')
    expect(item.classList.contains('x-item')).toBe(true)
    expect(item.classList.contains('x-item--dir')).toBe(true)
  })
})

const nested = {
  '': [
    { name: 'a', path: 'a', isDir: true },
    { name: 'b', path: 'b', isDir: true },
    { name: 'readme.md', path: 'readme.md', isDir: false },
  ],
  'a': [
    { name: 'inner.md', path: 'a/inner.md', isDir: false },
  ],
  'b': [
    { name: 'deep', path: 'b/deep', isDir: true },
  ],
  'b/deep': [
    { name: 'x.md', path: 'b/deep/x.md', isDir: false },
  ],
}

describe('TreeView expand/collapse', () => {
  let container
  beforeEach(() => {
    document.body.innerHTML = '<div id="host"></div>'
    container = document.getElementById('host')
  })

  it('expand(path) loads children, inserts rows, flips aria-expanded', async () => {
    const loader = makeLoader(nested)
    const tv = new TreeView(container, { loader })
    await tv.ready
    await tv.expand('a')
    expect(loader).toHaveBeenCalledWith('a')
    const row = container.querySelector('[data-path="a"]')
    expect(row.getAttribute('aria-expanded')).toBe('true')
    expect(container.querySelector('[data-path="a/inner.md"]')).toBeTruthy()
  })

  it('expand(path) is idempotent — no duplicate loader call, no duplicate rows', async () => {
    const loader = makeLoader(nested)
    const tv = new TreeView(container, { loader })
    await tv.ready
    await tv.expand('a')
    await tv.expand('a')
    const callsForA = loader.mock.calls.filter((c) => c[0] === 'a').length
    expect(callsForA).toBe(1)
    expect(container.querySelectorAll('[data-path="a/inner.md"]').length).toBe(1)
  })

  it('concurrent expand(path) calls share one loader call', async () => {
    const loader = makeLoader(nested)
    const tv = new TreeView(container, { loader })
    await tv.ready
    await Promise.all([tv.expand('a'), tv.expand('a')])
    const callsForA = loader.mock.calls.filter((c) => c[0] === 'a').length
    expect(callsForA).toBe(1)
  })

  it('collapse(path) removes DOM subtree but preserves model', async () => {
    const loader = makeLoader(nested)
    const tv = new TreeView(container, { loader })
    await tv.ready
    await tv.expand('a')
    tv.collapse('a')
    expect(container.querySelector('[data-path="a/inner.md"]')).toBeNull()
    expect(container.querySelector('[data-path="a"]').getAttribute('aria-expanded')).toBe('false')
    // Model retained: re-expand does not re-fetch
    await tv.expand('a')
    const callsForA = loader.mock.calls.filter((c) => c[0] === 'a').length
    expect(callsForA).toBe(1)
    expect(container.querySelector('[data-path="a/inner.md"]')).toBeTruthy()
  })

  it('toggle(path) expands if collapsed, collapses if expanded', async () => {
    const loader = makeLoader(nested)
    const tv = new TreeView(container, { loader })
    await tv.ready
    await tv.toggle('a')
    expect(container.querySelector('[data-path="a/inner.md"]')).toBeTruthy()
    await tv.toggle('a')
    expect(container.querySelector('[data-path="a/inner.md"]')).toBeNull()
  })

  it('emits tree:toggle on expand and collapse', async () => {
    const loader = makeLoader(nested)
    const tv = new TreeView(container, { loader })
    await tv.ready
    const events = []
    container.addEventListener('tree:toggle', (e) => events.push(e.detail))
    await tv.expand('a')
    tv.collapse('a')
    expect(events).toEqual([
      { path: 'a', expanded: true },
      { path: 'a', expanded: false },
    ])
  })

  it('expand(path) emits tree:error when loader rejects and leaves state unchanged', async () => {
    const loader = vi.fn(async (path) => {
      if (path === '') return nested['']
      throw new Error('nope')
    })
    const tv = new TreeView(container, { loader })
    await tv.ready
    const errors = []
    container.addEventListener('tree:error', (e) => errors.push(e.detail))
    await tv.expand('a')  // no .catch — promise resolves even on loader rejection
    expect(errors.length).toBe(1)
    expect(errors[0].path).toBe('a')
    expect(errors[0].error.message).toBe('nope')
    expect(container.querySelector('[data-path="a"]').getAttribute('aria-expanded')).toBe('false')
  })

  it('loadingPaths is cleared after a failed expand', async () => {
    const loader = vi.fn(async (path) => {
      if (path === '') return nested['']
      throw new Error('boom')
    })
    const tv = new TreeView(container, { loader })
    await tv.ready
    await tv.expand('a')
    expect(tv.loadingPaths.size).toBe(0)
  })
})

describe('TreeView select', () => {
  let container
  beforeEach(() => {
    document.body.innerHTML = '<div id="host"></div>'
    container = document.getElementById('host')
  })

  it('select(path) flips aria-selected and emits tree:select', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    const events = []
    container.addEventListener('tree:select', (e) => events.push(e.detail))
    tv.select('readme.md', { source: 'api' })
    const li = container.querySelector('[data-path="readme.md"]')
    expect(li.getAttribute('aria-selected')).toBe('true')
    expect(events).toEqual([{ path: 'readme.md', node: expect.any(Object), source: 'api' }])
  })

  it('select(path) clears previous selection', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    tv.select('a')
    tv.select('readme.md')
    expect(container.querySelector('[data-path="a"]').getAttribute('aria-selected')).toBe('false')
    expect(container.querySelector('[data-path="readme.md"]').getAttribute('aria-selected')).toBe('true')
  })

  it('select with source=silent does not emit tree:select', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    const events = []
    container.addEventListener('tree:select', (e) => events.push(e.detail))
    tv.select('readme.md', { source: 'silent' })
    expect(events.length).toBe(0)
    expect(container.querySelector('[data-path="readme.md"]').getAttribute('aria-selected')).toBe('true')
  })

  it('select sets keyboard focus (tabindex=0 on selected, -1 elsewhere)', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    tv.select('readme.md')
    const selected = container.querySelector('[data-path="readme.md"]')
    const others = container.querySelectorAll('[role="treeitem"]:not([data-path="readme.md"])')
    expect(selected.getAttribute('tabindex')).toBe('0')
    for (const o of others) expect(o.getAttribute('tabindex')).toBe('-1')
  })

  it('select(null) clears selection and tabindex rests on first node', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    tv.select('readme.md')
    tv.select(null)
    expect(container.querySelectorAll('[aria-selected="true"]').length).toBe(0)
    const first = container.querySelector('[role="treeitem"]')
    expect(first.getAttribute('tabindex')).toBe('0')
  })
})

describe('TreeView refresh / reconciliation', () => {
  let container
  beforeEach(() => {
    document.body.innerHTML = '<div id="host"></div>'
    container = document.getElementById('host')
  })

  it('refresh adds new children from loader', async () => {
    const state = { '': [{ name: 'a.md', path: 'a.md', isDir: false }] }
    const loader = vi.fn(async (p) => (state[p] || []).slice())
    const tv = new TreeView(container, { loader })
    await tv.ready
    state[''].push({ name: 'b.md', path: 'b.md', isDir: false })
    await tv.refresh('')
    expect(container.querySelector('[data-path="b.md"]')).toBeTruthy()
  })

  it('refresh removes deleted children and drops their model', async () => {
    const state = {
      '': [{ name: 'x.md', path: 'x.md', isDir: false }, { name: 'y.md', path: 'y.md', isDir: false }],
    }
    const loader = vi.fn(async (p) => (state[p] || []).slice())
    const tv = new TreeView(container, { loader })
    await tv.ready
    state[''] = [{ name: 'x.md', path: 'x.md', isDir: false }]
    await tv.refresh('')
    expect(container.querySelector('[data-path="y.md"]')).toBeNull()
    expect(tv.nodesByPath.has('y.md')).toBe(false)
  })

  it('refresh preserves unchanged-and-expanded descendants', async () => {
    const state = {
      '': [
        { name: 'a', path: 'a', isDir: true },
        { name: 'b', path: 'b', isDir: true },
      ],
      'a': [{ name: 'a.md', path: 'a/a.md', isDir: false }],
      'b': [{ name: 'b.md', path: 'b/b.md', isDir: false }],
    }
    const loader = vi.fn(async (p) => (state[p] || []).slice())
    const tv = new TreeView(container, { loader })
    await tv.ready
    await tv.expand('a')
    await tv.expand('b')

    state[''].unshift({ name: 'c', path: 'c', isDir: true })
    state['c'] = []
    await tv.refresh('')

    expect(container.querySelector('[data-path="c"]')).toBeTruthy()
    expect(container.querySelector('[data-path="a/a.md"]')).toBeTruthy()
    expect(container.querySelector('[data-path="b/b.md"]')).toBeTruthy()
    expect(tv.expandedPaths.has('a')).toBe(true)
    expect(tv.expandedPaths.has('b')).toBe(true)
  })

  it('refresh clears selection if selected node is removed', async () => {
    const state = {
      '': [
        { name: 'x.md', path: 'x.md', isDir: false },
        { name: 'y.md', path: 'y.md', isDir: false },
      ],
    }
    const loader = vi.fn(async (p) => (state[p] || []).slice())
    const tv = new TreeView(container, { loader })
    await tv.ready
    tv.select('y.md')
    const events = []
    container.addEventListener('tree:select', (e) => events.push(e.detail))
    state[''] = [{ name: 'x.md', path: 'x.md', isDir: false }]
    await tv.refresh('')
    expect(tv.selectedPath).toBeNull()
    expect(events.some((e) => e.path === null)).toBe(true)
  })

  it('refresh replaces a node whose isDir flipped', async () => {
    const state = { '': [{ name: 'thing', path: 'thing', isDir: true }], 'thing': [] }
    const loader = vi.fn(async (p) => (state[p] || []).slice())
    const tv = new TreeView(container, { loader })
    await tv.ready
    await tv.expand('thing')
    expect(tv.expandedPaths.has('thing')).toBe(true)

    state[''] = [{ name: 'thing', path: 'thing', isDir: false }]
    await tv.refresh('')
    const row = container.querySelector('[data-path="thing"]')
    expect(row.classList.contains('tv-item--file')).toBe(true)
    expect(tv.expandedPaths.has('thing')).toBe(false)
  })
})

describe('TreeView refresh-during-load queue', () => {
  let container
  beforeEach(() => {
    document.body.innerHTML = '<div id="host"></div>'
    container = document.getElementById('host')
  })

  it('refresh while expand is in flight fires exactly one follow-up fetch', async () => {
    let resolveFirst
    const firstPromise = new Promise((r) => { resolveFirst = r })
    const calls = []
    const loader = vi.fn(async (path) => {
      calls.push(path)
      if (path === '') return [{ name: 'a', path: 'a', isDir: true }]
      if (path === 'a' && calls.filter((c) => c === 'a').length === 1) {
        await firstPromise
        return [{ name: 'x.md', path: 'a/x.md', isDir: false }]
      }
      return [{ name: 'y.md', path: 'a/y.md', isDir: false }]
    })
    const tv = new TreeView(container, { loader })
    await tv.ready

    const expandP = tv.expand('a')
    // While the first load is blocked, ask for a refresh
    const refreshP = tv.refresh('a')
    resolveFirst()
    await Promise.all([expandP, refreshP])
    // Allow the queued follow-up to complete
    await new Promise((r) => setTimeout(r, 0))

    expect(calls.filter((c) => c === 'a').length).toBe(2)
    expect(container.querySelector('[data-path="a/y.md"]')).toBeTruthy()
  })

  it('multiple refreshes during a single in-flight load coalesce to one follow-up', async () => {
    let resolveFirst
    const firstPromise = new Promise((r) => { resolveFirst = r })
    const calls = []
    const loader = vi.fn(async (path) => {
      calls.push(path)
      if (path === '') return [{ name: 'a', path: 'a', isDir: true }]
      if (calls.filter((c) => c === 'a').length === 1) {
        await firstPromise
        return [{ name: 'x.md', path: 'a/x.md', isDir: false }]
      }
      return [{ name: 'z.md', path: 'a/z.md', isDir: false }]
    })
    const tv = new TreeView(container, { loader })
    await tv.ready
    const p = tv.expand('a')
    tv.refresh('a')
    tv.refresh('a')
    tv.refresh('a')
    resolveFirst()
    await p
    await new Promise((r) => setTimeout(r, 0))
    expect(calls.filter((c) => c === 'a').length).toBe(2)
  })
})

describe('TreeView persistence', () => {
  let container
  beforeEach(() => {
    document.body.innerHTML = '<div id="host"></div>'
    container = document.getElementById('host')
    localStorage.clear()
  })

  it('persists expanded + selected on change', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested), persistKey: 'tv' })
    await tv.ready
    await tv.expand('a')
    tv.select('readme.md')
    const saved = JSON.parse(localStorage.getItem('tv'))
    expect(saved.version).toBe(1)
    expect(saved.expanded).toEqual(['a'])
    expect(saved.selected).toBe('readme.md')
  })

  it('bootstrap restores expanded + selected from localStorage', async () => {
    localStorage.setItem('tv', JSON.stringify({ version: 1, expanded: ['a'], selected: 'readme.md' }))
    const tv = new TreeView(container, { loader: makeLoader(nested), persistKey: 'tv' })
    await tv.ready
    expect(container.querySelector('[data-path="a/inner.md"]')).toBeTruthy()
    expect(container.querySelector('[data-path="readme.md"]').getAttribute('aria-selected')).toBe('true')
  })

  it('invalid JSON in storage is ignored', async () => {
    localStorage.setItem('tv', '{not json')
    const tv = new TreeView(container, { loader: makeLoader(nested), persistKey: 'tv' })
    await tv.ready
    expect(tv.expandedPaths.size).toBe(0)
  })

  it('stale expanded paths are pruned and re-persisted', async () => {
    localStorage.setItem('tv', JSON.stringify({ version: 1, expanded: ['a', 'ghost'], selected: null }))
    const tv = new TreeView(container, { loader: makeLoader(nested), persistKey: 'tv' })
    await tv.ready
    const saved = JSON.parse(localStorage.getItem('tv'))
    expect(saved.expanded).toEqual(['a'])
  })

  it('initial.selectedPath wins over persisted selected and merges expansion with ancestors', async () => {
    localStorage.setItem('tv', JSON.stringify({ version: 1, expanded: ['a'], selected: 'readme.md' }))
    const tv = new TreeView(container, {
      loader: makeLoader(nested),
      persistKey: 'tv',
      initial: { selectedPath: 'b/deep/x.md' },
    })
    await tv.ready
    expect(tv.expandedPaths.has('a')).toBe(true)
    expect(tv.expandedPaths.has('b')).toBe(true)
    expect(tv.expandedPaths.has('b/deep')).toBe(true)
    expect(tv.selectedPath).toBe('b/deep/x.md')
    expect(container.querySelector('[data-path="b/deep/x.md"]').getAttribute('aria-selected')).toBe('true')
  })

  it('initial.selectedPath applied silently does not emit tree:select', async () => {
    const tv = new TreeView(container, {
      loader: makeLoader(nested),
      initial: { selectedPath: 'readme.md' },
    })
    const events = []
    container.addEventListener('tree:select', (e) => events.push(e.detail))
    await tv.ready
    expect(events.length).toBe(0)
    expect(tv.selectedPath).toBe('readme.md')
  })

  it('wrong version is dropped', async () => {
    localStorage.setItem('tv', JSON.stringify({ version: 99, expanded: ['a'], selected: 'x' }))
    const tv = new TreeView(container, { loader: makeLoader(nested), persistKey: 'tv' })
    await tv.ready
    expect(tv.expandedPaths.size).toBe(0)
    expect(tv.selectedPath).toBeNull()
  })
})

describe('TreeView keyboard — arrows, Home, End', () => {
  let container
  beforeEach(() => {
    document.body.innerHTML = '<div id="host"></div>'
    container = document.getElementById('host')
  })

  function press(el, key) {
    const ev = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true })
    el.dispatchEvent(ev)
    return ev
  }

  it('ArrowDown moves focus to next visible node', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    const first = container.querySelector('[data-path="a"]')
    first.focus()
    press(first, 'ArrowDown')
    expect(tv.focusedPath).toBe('b')
    expect(container.querySelector('[data-path="b"]').getAttribute('tabindex')).toBe('0')
  })

  it('ArrowUp moves focus to previous visible node', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    const readme = container.querySelector('[data-path="readme.md"]')
    readme.focus()
    press(readme, 'ArrowUp')
    expect(tv.focusedPath).toBe('b')
  })

  it('ArrowRight on collapsed dir expands it', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    const a = container.querySelector('[data-path="a"]')
    a.focus()
    press(a, 'ArrowRight')
    await new Promise((r) => setTimeout(r, 0))
    expect(tv.expandedPaths.has('a')).toBe(true)
    // Focus stays on a
    expect(tv.focusedPath).toBe('a')
  })

  it('ArrowRight on expanded dir moves focus to first child', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    await tv.expand('a')
    const a = container.querySelector('[data-path="a"]')
    a.focus()
    press(a, 'ArrowRight')
    expect(tv.focusedPath).toBe('a/inner.md')
  })

  it('ArrowLeft on expanded dir collapses it', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    await tv.expand('a')
    const a = container.querySelector('[data-path="a"]')
    a.focus()
    press(a, 'ArrowLeft')
    expect(tv.expandedPaths.has('a')).toBe(false)
    expect(tv.focusedPath).toBe('a')
  })

  it('ArrowLeft on child moves focus to parent', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    await tv.expand('a')
    const inner = container.querySelector('[data-path="a/inner.md"]')
    inner.focus()
    press(inner, 'ArrowLeft')
    expect(tv.focusedPath).toBe('a')
  })

  it('Home focuses the first visible node', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    const readme = container.querySelector('[data-path="readme.md"]')
    readme.focus()
    press(readme, 'Home')
    expect(tv.focusedPath).toBe('a')
  })

  it('End focuses the last visible node', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    const a = container.querySelector('[data-path="a"]')
    a.focus()
    press(a, 'End')
    expect(tv.focusedPath).toBe('readme.md')
  })

  it('arrow keys preventDefault to stop page scroll', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    const a = container.querySelector('[data-path="a"]')
    a.focus()
    const ev = press(a, 'ArrowDown')
    expect(ev.defaultPrevented).toBe(true)
  })

  it('exactly one treeitem has tabindex=0 at all times', async () => {
    const tv = new TreeView(container, { loader: makeLoader(nested) })
    await tv.ready
    const first = container.querySelector('[data-path="a"]')
    first.focus()
    press(first, 'ArrowDown')
    const zeros = container.querySelectorAll('[role="treeitem"][tabindex="0"]')
    expect(zeros.length).toBe(1)
  })
})
