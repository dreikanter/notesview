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
})
