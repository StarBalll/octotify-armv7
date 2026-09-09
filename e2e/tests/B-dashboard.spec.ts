import { test, expect } from '../fixtures/test-fixtures';

test.describe('B-Dashboard: 仪表盘', () => {
  test.beforeEach(async ({ page, auth }) => {
    const testUser = `B_dashboard_${Date.now()}`;
    const testPass = 'Test123456';
    await auth.registerAndLogin(testUser, testPass);
  });

  test('B-51: 加载仪表盘（4个统计卡片可见）', async ({ page }) => {
    await page.goto('/dashboard/index');
    await page.waitForURL(/\/dashboard/, { timeout: 10000 });
    await expect(page.locator('.stat-card').first()).toBeVisible({ timeout: 10000 });
    const statCards = page.locator('.stat-card');
    await expect(statCards).toHaveCount(4);
  });

  test('B-52: 来源总数卡片显示正确', async ({ page }) => {
    await page.goto('/dashboard/index');
    await page.waitForURL(/\/dashboard/, { timeout: 10000 });

    const sourceCard = page.locator('.stat-card').filter({ hasText: '来源总数' });
    await expect(sourceCard).toBeVisible();

    const valueElement = sourceCard.locator('p.text-2xl');
    await expect(valueElement).toBeVisible();
    const value = await valueElement.textContent();
    expect(value).toBeTruthy();
    expect(value === '--' || /^\d+$/.test(value!)).toBeTruthy();
  });

  test('B-53: 渠道总数卡片显示正确', async ({ page }) => {
    await page.goto('/dashboard/index');
    await page.waitForURL(/\/dashboard/, { timeout: 10000 });

    const channelCard = page.locator('.stat-card').filter({ hasText: '渠道总数' });
    await expect(channelCard).toBeVisible();

    const valueElement = channelCard.locator('p.text-2xl');
    await expect(valueElement).toBeVisible();
    const value = await valueElement.textContent();
    expect(value).toBeTruthy();
    expect(value === '--' || /^\d+$/.test(value!)).toBeTruthy();
  });

  test('B-54: 最近推送表格展示', async ({ page }) => {
    await page.goto('/dashboard/index');
    await page.waitForURL(/\/dashboard/, { timeout: 10000 });

    const table = page.locator('.el-table').first();
    await expect(table).toBeVisible({ timeout: 10000 });

    await expect(page.getByText('消息标题')).toBeVisible();
    await expect(page.getByText('来源名称')).toBeVisible();
    await expect(page.getByText('渠道名称')).toBeVisible();
    await expect(page.getByText('状态')).toBeVisible();
    await expect(page.getByText('创建时间')).toBeVisible();
  });

  test('B-55: 点击标题跳转至消息详情页', async ({ page }) => {
    await page.goto('/dashboard/index');
    await page.waitForURL(/\/dashboard/, { timeout: 10000 });

    const table = page.locator('.el-table').first();
    await expect(table).toBeVisible({ timeout: 10000 });

    const emptyText = page.getByText('暂无推送记录');
    const hasData = await emptyText.isHidden().catch(() => true);

    if (hasData) {
      const firstTitleLink = page.locator('.el-table a').first();
      const isLinkVisible = await firstTitleLink.isVisible().catch(() => false);

      if (isLinkVisible) {
        await firstTitleLink.click();

        // 点击标题应导航至消息详情页 /message/detail/<id>
        await page.waitForURL(/\/message\/detail\/\d+/, { timeout: 10000 });
        expect(page.url()).toMatch(/\/message\/detail\/\d+/);

        // 详情页应完成加载（出现"返回"按钮或消息描述区）
        await expect(page.locator('.el-descriptions').first()).toBeVisible({
          timeout: 10000,
        });
      }
    }
  });
});