import { test, expect } from '@playwright/test'

test.describe('Sidebar Tree (client-side TreeView)', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/')
    await page.evaluate(() => localStorage.clear())
  })

  test('FILES and TAGS sections render', async ({ page }) => {
    await page.goto('/view/README.md')
    await page.click('#sidebar-toggle')
    await expect(page.locator('#files-section')).toBeVisible()
    await expect(page.locator('#tags-section')).toBeVisible()
  })

  test('root tree populates from /api/tree/list', async ({ page }) => {
    await page.goto('/view/README.md')
    await page.click('#sidebar-toggle')
    const files = page.locator('#sidebar-tree')
    await expect(files.locator('[data-path="journal"]')).toBeVisible()
    await expect(files.locator('[data-path="projects"]')).toBeVisible()
    await expect(files.locator('[data-path="README.md"]')).toBeVisible()
  })

  test('initial selectedPath reveals and selects the current note on reload', async ({ page }) => {
    await page.goto('/view/journal/day-one.md')
    await page.click('#sidebar-toggle')
    const target = page.locator('#sidebar-tree [data-path="journal/day-one.md"]')
    await expect(target).toBeVisible()
    await expect(target).toHaveAttribute('aria-selected', 'true')
    await expect(page.locator('#sidebar-tree [data-path="journal"]')).toHaveAttribute('aria-expanded', 'true')
  })

  test('chevron click expands and collapses without changing URL', async ({ page }) => {
    await page.goto('/view/README.md')
    await page.click('#sidebar-toggle')
    const files = page.locator('#sidebar-tree')
    await files.locator('[data-path="journal"] .tv-toggle').click()
    await expect(files.locator('[data-path="journal/day-one.md"]')).toBeVisible()
    await expect(page).toHaveURL(/\/view\/README\.md/)
    await files.locator('[data-path="journal"] .tv-toggle').click()
    await expect(files.locator('[data-path="journal/day-one.md"]')).toHaveCount(0)
    await expect(page).toHaveURL(/\/view\/README\.md/)
  })

  test('chevron on one dir does not collapse another expanded dir', async ({ page }) => {
    await page.goto('/view/README.md')
    await page.click('#sidebar-toggle')
    const files = page.locator('#sidebar-tree')
    await files.locator('[data-path="journal"] .tv-toggle').click()
    await files.locator('[data-path="projects"] .tv-toggle').click()
    await expect(files.locator('[data-path="journal/day-one.md"]')).toBeVisible()
    await expect(files.locator('[data-path="projects/alpha.md"]')).toBeVisible()
    await files.locator('[data-path="journal"] .tv-toggle').click()
    await expect(files.locator('[data-path="journal/day-one.md"]')).toHaveCount(0)
    await expect(files.locator('[data-path="projects/alpha.md"]')).toBeVisible()
  })

  test('clicking a note row opens it in the main panel and updates URL', async ({ page }) => {
    await page.goto('/view/README.md')
    await page.click('#sidebar-toggle')
    await page.locator('#sidebar-tree [data-path="journal"] .tv-toggle').click()
    await page.locator('#sidebar-tree [data-path="journal/day-one.md"] .tv-label').click()
    await expect(page.locator('#note-card')).toContainText('Day One')
    await expect(page).toHaveURL(/\/view\/journal\/day-one\.md/)
  })

  test('clicking a directory row loads its listing', async ({ page }) => {
    await page.goto('/view/README.md')
    await page.click('#sidebar-toggle')
    await page.locator('#sidebar-tree [data-path="journal"] .tv-label').click()
    const listing = page.locator('#dir-listing')
    await expect(listing).toBeVisible()
    await expect(listing.locator('a', { hasText: 'day-one.md' })).toBeVisible()
  })

  test('tags section continues to work as a flat list', async ({ page }) => {
    await page.goto('/view/README.md')
    await page.click('#sidebar-toggle')
    await page.locator('#tags-content a', { hasText: 'daily' }).click()
    const listing = page.locator('#dir-listing')
    await expect(listing).toBeVisible()
    await expect(listing.locator('a', { hasText: 'day-one.md' })).toBeVisible()
  })

  test('reload preserves expanded and selected state', async ({ page }) => {
    await page.goto('/view/README.md')
    await page.click('#sidebar-toggle')
    await page.locator('#sidebar-tree [data-path="journal"] .tv-toggle').click()
    await page.locator('#sidebar-tree [data-path="journal/day-one.md"] .tv-label').click()
    await expect(page).toHaveURL(/\/view\/journal\/day-one\.md/)
    await page.reload()
    const target = page.locator('#sidebar-tree [data-path="journal/day-one.md"]')
    await expect(target).toBeVisible()
    await expect(target).toHaveAttribute('aria-selected', 'true')
  })

  test('keyboard: ArrowDown moves focus between visible items', async ({ page }) => {
    await page.goto('/view/README.md')
    await page.click('#sidebar-toggle')
    const tree = page.locator('#sidebar-tree')
    await tree.locator('[data-path="journal"]').focus()
    await page.keyboard.press('ArrowDown')
    const focused = await page.evaluate(() => document.activeElement?.getAttribute('data-path'))
    expect(focused).toBe('projects')
  })

  test('keyboard: ArrowRight expands a collapsed dir', async ({ page }) => {
    await page.goto('/view/README.md')
    await page.click('#sidebar-toggle')
    const journal = page.locator('#sidebar-tree [data-path="journal"]')
    await journal.focus()
    await page.keyboard.press('ArrowRight')
    await expect(journal).toHaveAttribute('aria-expanded', 'true')
    await expect(page.locator('#sidebar-tree [data-path="journal/day-one.md"]')).toBeVisible()
  })

  test('tree root has role=tree and items have role=treeitem', async ({ page }) => {
    await page.goto('/view/README.md')
    await page.click('#sidebar-toggle')
    await expect(page.locator('#sidebar-tree .tv-root')).toHaveAttribute('role', 'tree')
    const items = page.locator('#sidebar-tree [role="treeitem"]')
    await expect(items.first()).toBeVisible()
  })

  test('section collapse/expand still works', async ({ page }) => {
    await page.goto('/view/README.md')
    await page.click('#sidebar-toggle')
    await expect(page.locator('#files-content')).toBeVisible()
    await page.locator('#files-section > button').click()
    await expect(page.locator('#files-content')).toBeHidden()
    await page.locator('#files-section > button').click()
    await expect(page.locator('#files-content')).toBeVisible()
  })

  test('tag navigation stops watching the previous note', async ({ page }) => {
    await page.goto('/view/README.md')
    await page.click('#sidebar-toggle')
    // Wait for sidebar to mount and open the initial EventSource for README.md
    await page.waitForFunction(() => (window as any).__tvWatchedNote !== undefined)
    const initial = await page.evaluate(() => (window as any).__tvWatchedNote as string)
    expect(initial).toBe('README.md')
    // Trigger a tag click
    await page.locator('#tags-content a', { hasText: 'daily' }).click()
    await expect(page.locator('#dir-listing')).toBeVisible()
    const watched = await page.evaluate(() => (window as any).__tvWatchedNote as string)
    // After navigating to a tag view, no note should be watched
    expect(watched).toBe('')
  })

  test('browser back navigates back and restores note pane content', async ({ page }) => {
    await page.goto('/view/README.md')
    await page.click('#sidebar-toggle')
    await page.locator('#sidebar-tree [data-path="journal"] .tv-toggle').click()
    await page.locator('#sidebar-tree [data-path="journal/day-one.md"] .tv-label').click()
    await expect(page).toHaveURL(/\/view\/journal\/day-one\.md/)
    await expect(page.locator('#note-card')).toContainText('Day One')

    await page.goBack()
    await expect(page).toHaveURL(/\/view\/README\.md/)
    // Main pane should reload to show README content
    await expect(page.locator('#note-card')).toContainText('Welcome')
  })
})
