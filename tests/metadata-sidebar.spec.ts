import { test, expect } from '@playwright/test'

test.describe('Metadata sidebar', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/')
    await page.evaluate(() => localStorage.clear())
  })

  test('quick links and metadata sections render', async ({ page }) => {
    await page.goto('/n/readme')
    await page.click('#sidebar-toggle')
    await expect(page.locator('#quick-links-section')).toBeVisible()
    await expect(page.locator('#recent-section')).toBeVisible()
    await expect(page.locator('#filter-section')).toBeVisible()
    await expect(page.locator('#quick-links-section a', { hasText: 'Tags' })).toHaveAttribute('href', '/tags')
    await expect(page.locator('#quick-links-section a', { hasText: 'Types' })).toHaveAttribute('href', '/types')
    await expect(page.locator('#quick-links-section a', { hasText: 'Dates' })).toHaveAttribute('href', '/dates')
  })

  test('recent note navigation opens note in the main panel', async ({ page }) => {
    await page.goto('/n/readme')
    await page.click('#sidebar-toggle')
    await page.locator('#recent-section a', { hasText: 'Day One' }).click()
    await expect(page.locator('#note-card')).toContainText('Day One')
    await expect(page).toHaveURL(/\/n\/2/)
  })

  test('tag filter opens tagged note listing and stops watching notes', async ({ page }) => {
    await page.goto('/n/readme')
    await page.click('#sidebar-toggle')
    await page.waitForFunction(() => (window as any).__nviewWatchedNoteID === 1)
    await page.locator('#tags-content a', { hasText: 'daily' }).click()
    await expect(page.locator('#dir-listing')).toBeVisible()
    await expect(page.locator('#dir-listing')).toContainText('Day One')
    await expect(page).toHaveURL(/\/tags\/daily/)
    expect(await page.evaluate(() => (window as any).__nviewWatchedNoteID as number)).toBe(0)
  })

  test('type and date quick links render index pages', async ({ page }) => {
    await page.goto('/n/readme')
    await page.click('#sidebar-toggle')
    await page.locator('#quick-links-section a', { hasText: 'Types' }).click()
    await expect(page.locator('#dir-listing')).toContainText('Types')
    await expect(page).toHaveURL(/\/types/)

    await page.locator('#quick-links-section a', { hasText: 'Dates' }).click()
    await expect(page.locator('#dir-listing')).toContainText('Dates')
    await expect(page).toHaveURL(/\/dates/)
  })

  test('browser back restores note pane content', async ({ page }) => {
    await page.goto('/n/readme')
    await page.click('#sidebar-toggle')
    await page.locator('#recent-section a', { hasText: 'Day One' }).click()
    await expect(page.locator('#note-card')).toContainText('Day One')

    await page.goBack()
    await expect(page).toHaveURL(/\/n\/readme/)
    await expect(page.locator('#note-card')).toContainText('Welcome')
  })
})
