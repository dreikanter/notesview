import { test, expect } from '@playwright/test';

test.describe('Sidebar Tree Navigation', () => {
  test.beforeEach(async ({ page }) => {
    // Start with a clean localStorage state
    await page.goto('/');
    await page.evaluate(() => {
      localStorage.clear();
    });
  });

  test('sidebar shows FILES and TAGS sections', async ({ page }) => {
    await page.goto('/view/README.md');

    // Open sidebar
    await page.click('#sidebar-toggle');

    // Check both section headings are visible
    await expect(page.locator('#files-section')).toBeVisible();
    await expect(page.locator('#tags-section')).toBeVisible();
    await expect(page.locator('#files-section button')).toContainText('FILES');
    await expect(page.locator('#tags-section button')).toContainText('TAGS');
  });

  test('FILES section shows root directory entries', async ({ page }) => {
    await page.goto('/view/README.md');
    await page.click('#sidebar-toggle');

    // Should see directories and files at root
    const filesContent = page.locator('#files-content');
    await expect(filesContent.locator('a', { hasText: 'journal' })).toBeVisible();
    await expect(filesContent.locator('a', { hasText: 'projects' })).toBeVisible();
    await expect(filesContent.locator('a', { hasText: 'README.md' })).toBeVisible();
  });

  test('TAGS section shows all tags', async ({ page }) => {
    await page.goto('/view/README.md');
    await page.click('#sidebar-toggle');

    const tagsContent = page.locator('#tags-content');
    // Tags from fixtures: readme, intro, journal, daily, project
    await expect(tagsContent.locator('a', { hasText: 'daily' })).toBeVisible();
    await expect(tagsContent.locator('a', { hasText: 'journal' })).toBeVisible();
    await expect(tagsContent.locator('a', { hasText: 'intro' })).toBeVisible();
  });

  test('clicking a directory shows listing in main panel', async ({ page }) => {
    await page.goto('/view/README.md');
    await page.click('#sidebar-toggle');

    // Click the journal directory in sidebar
    await page.locator('#files-content a', { hasText: 'journal' }).click();

    // Main panel should show a directory listing with the journal's notes
    const listing = page.locator('#dir-listing');
    await expect(listing).toBeVisible();
    await expect(listing.locator('a', { hasText: 'day-one.md' })).toBeVisible();
    await expect(listing.locator('a', { hasText: 'day-two.md' })).toBeVisible();
  });

  test('clicking a directory expands it in sidebar', async ({ page }) => {
    await page.goto('/view/README.md');
    await page.click('#sidebar-toggle');

    // Click journal directory
    await page.locator('#files-content a', { hasText: 'journal' }).click();

    // Sidebar files section should now show journal's contents
    const filesContent = page.locator('#files-content');
    await expect(filesContent.locator('a', { hasText: 'day-one.md' })).toBeVisible();
    await expect(filesContent.locator('a', { hasText: 'day-two.md' })).toBeVisible();
  });

  test('clicking an expanded directory collapses it', async ({ page }) => {
    await page.goto('/view/README.md');
    await page.click('#sidebar-toggle');

    const filesContent = page.locator('#files-content');

    // Expand journal
    await filesContent.locator('a', { hasText: 'journal' }).click();
    await expect(filesContent.locator('a', { hasText: 'day-one.md' })).toBeVisible();

    // Click journal again — should collapse (target becomes root)
    await filesContent.locator('a', { hasText: 'journal' }).click();

    // Children gone, journal and its root-level siblings still visible
    await expect(filesContent.locator('a', { hasText: 'day-one.md' })).toHaveCount(0);
    await expect(filesContent.locator('a', { hasText: 'journal' })).toBeVisible();
    await expect(filesContent.locator('a', { hasText: 'projects' })).toBeVisible();

    // URL reflects collapse to root listing
    await expect(page).toHaveURL(/\/dir\/?$/);
  });

  test('clicking a note opens it in main panel', async ({ page }) => {
    await page.goto('/view/README.md');
    await page.click('#sidebar-toggle');

    // Click journal directory first
    await page.locator('#files-content a', { hasText: 'journal' }).click();

    // Wait for sidebar to update
    await page.locator('#files-content a', { hasText: 'day-one.md' }).waitFor();

    // Click a note in the sidebar
    await page.locator('#files-content a', { hasText: 'day-one.md' }).click();

    // Main panel should show the note content
    const noteCard = page.locator('#note-card');
    await expect(noteCard).toBeVisible();
    await expect(noteCard).toContainText('Day One');
  });

  test('clicking a note from main panel listing opens it', async ({ page }) => {
    await page.goto('/view/README.md');
    await page.click('#sidebar-toggle');

    // Click journal directory to show listing in main panel
    await page.locator('#files-content a', { hasText: 'journal' }).click();

    // Click a note from the main panel listing
    const listing = page.locator('#dir-listing');
    await listing.locator('a', { hasText: 'day-one.md' }).click();

    // Main panel should show the note
    const noteCard = page.locator('#note-card');
    await expect(noteCard).toBeVisible();
    await expect(noteCard).toContainText('Day One');
  });

  test('clicking a tag shows its notes in main panel', async ({ page }) => {
    await page.goto('/view/README.md');
    await page.click('#sidebar-toggle');

    // Click the "daily" tag
    await page.locator('#tags-content a', { hasText: 'daily' }).click();

    // Main panel should show a listing of notes with that tag
    const listing = page.locator('#dir-listing');
    await expect(listing).toBeVisible();
    await expect(listing).toContainText('day-one.md');
    await expect(listing).toContainText('day-two.md');
  });

  test('clicking a tag keeps sidebar tags list unchanged', async ({ page }) => {
    await page.goto('/view/README.md');
    await page.click('#sidebar-toggle');

    // Click the "daily" tag
    await page.locator('#tags-content a', { hasText: 'daily' }).click();
    await page.locator('#dir-listing').waitFor();

    // Sidebar tags section should still show the flat tag list (no expansion)
    const tagsContent = page.locator('#tags-content');
    await expect(tagsContent.locator('a', { hasText: 'daily' })).toBeVisible();
    await expect(tagsContent.locator('a', { hasText: 'intro' })).toBeVisible();
    // Tag's notes should NOT appear in the sidebar
    await expect(tagsContent.locator('a', { hasText: 'day-one.md' })).not.toBeVisible();
  });

  test('section collapse/expand toggles visibility', async ({ page }) => {
    await page.goto('/view/README.md');
    await page.click('#sidebar-toggle');

    // FILES section should be visible initially
    await expect(page.locator('#files-content')).toBeVisible();

    // Click FILES heading to collapse
    await page.locator('#files-section button').click();
    await expect(page.locator('#files-content')).toBeHidden();

    // Click again to expand
    await page.locator('#files-section button').click();
    await expect(page.locator('#files-content')).toBeVisible();
  });

  test('entry lists look identical in sidebar and main panel', async ({ page }) => {
    await page.goto('/view/README.md');
    await page.click('#sidebar-toggle');

    // Click journal directory to show listing in both places
    await page.locator('#files-content a', { hasText: 'journal' }).click();

    // Both sidebar and main panel should have entry-link class items
    const sidebarLinks = page.locator('#files-content .entry-link');
    const mainLinks = page.locator('#dir-listing .entry-link');

    await expect(sidebarLinks).not.toHaveCount(0);
    await expect(mainLinks).not.toHaveCount(0);

    // Both should contain the same notes
    await expect(sidebarLinks.locator('text=day-one.md')).toBeVisible();
    await expect(mainLinks.locator('text=day-one.md')).toBeVisible();
  });

  test('tag pills in note content trigger navigation', async ({ page }) => {
    await page.goto('/view/README.md');
    await page.click('#sidebar-toggle');

    // The README has tags: [readme, intro]
    // Click the "intro" tag pill in the note content
    const tagPill = page.locator('#note-card a', { hasText: 'intro' });
    await expect(tagPill).toBeVisible();
    await tagPill.click();

    // Main panel should show listing of notes with "intro" tag
    const listing = page.locator('#dir-listing');
    await expect(listing).toBeVisible();
    // Both README.md and alpha.md have the "intro" tag
    await expect(listing).toContainText('README.md');
    await expect(listing).toContainText('alpha.md');
  });

  test('sidebar shows tree hierarchy with ancestor chain', async ({ page }) => {
    await page.goto('/view/README.md');
    await page.click('#sidebar-toggle');

    // Click journal directory
    await page.locator('#files-content a', { hasText: 'journal' }).click();

    // Sidebar should show journal expanded with its files indented,
    // AND root-level siblings still visible
    const filesContent = page.locator('#files-content');
    await expect(filesContent.locator('a', { hasText: 'journal' })).toBeVisible();
    await expect(filesContent.locator('a', { hasText: 'day-one.md' })).toBeVisible();
    // Root-level siblings should still be visible
    await expect(filesContent.locator('a', { hasText: 'projects' })).toBeVisible();
    await expect(filesContent.locator('a', { hasText: 'README.md' })).toBeVisible();
  });

  test('clicking a tag shows notes in main panel only', async ({ page }) => {
    await page.goto('/view/README.md');
    await page.click('#sidebar-toggle');

    // Click the "daily" tag
    await page.locator('#tags-content a', { hasText: 'daily' }).click();

    // Main panel should show tagged notes
    const listing = page.locator('#dir-listing');
    await expect(listing).toBeVisible();
    await expect(listing.locator('a', { hasText: 'day-one.md' })).toBeVisible();

    // Sidebar tags section stays flat — no expansion
    const tagsContent = page.locator('#tags-content');
    await expect(tagsContent.locator('a', { hasText: 'daily' })).toBeVisible();
    await expect(tagsContent.locator('a', { hasText: 'day-one.md' })).not.toBeVisible();
  });

  test('URL updates when navigating to a note', async ({ page }) => {
    await page.goto('/view/README.md');
    await page.click('#sidebar-toggle');

    // Navigate to journal
    await page.locator('#files-content a', { hasText: 'journal' }).click();
    await page.locator('#files-content a', { hasText: 'day-one.md' }).waitFor();

    // Click a note
    await page.locator('#files-content a', { hasText: 'day-one.md' }).click();
    await page.locator('#note-card').waitFor();

    // URL should have updated
    await expect(page).toHaveURL(/\/view\/journal\/day-one\.md/);
  });

  test('URL updates when navigating to a directory', async ({ page }) => {
    await page.goto('/view/README.md');
    await page.click('#sidebar-toggle');

    await page.locator('#files-content a', { hasText: 'journal' }).click();
    await page.locator('#dir-listing').waitFor();

    await expect(page).toHaveURL(/\/dir\/journal/);
  });

  test('URL updates when navigating to a tag', async ({ page }) => {
    await page.goto('/view/README.md');
    await page.click('#sidebar-toggle');

    await page.locator('#tags-content a', { hasText: 'daily' }).click();
    await page.locator('#dir-listing').waitFor();

    await expect(page).toHaveURL(/\/tags\/daily/);
  });

  test('page reload preserves current view', async ({ page }) => {
    await page.goto('/view/README.md');
    await page.click('#sidebar-toggle');

    // Navigate to journal dir
    await page.locator('#files-content a', { hasText: 'journal' }).click();
    await page.locator('#dir-listing').waitFor();

    // Reload the page
    await page.reload();

    // Should still show the journal directory listing
    const listing = page.locator('#dir-listing');
    await expect(listing).toBeVisible();
    await expect(listing.locator('a', { hasText: 'day-one.md' })).toBeVisible();
  });
});
