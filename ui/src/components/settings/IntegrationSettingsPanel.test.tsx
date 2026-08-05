import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ToastProvider } from '@/components/shared/Toast';
import IntegrationSettingsPanel from './IntegrationSettingsPanel';

function renderPanel() {
  render(
    <ToastProvider>
      <IntegrationSettingsPanel />
    </ToastProvider>
  );
}

afterEach(() => vi.restoreAllMocks());

describe('IntegrationSettingsPanel', () => {
  describe('rendering', () => {
    it('renders the title', () => {
      renderPanel();
      expect(screen.getByText('Integration Settings')).toBeInTheDocument();
    });

    it('renders all three tabs', () => {
      renderPanel();
      expect(screen.getByText('API Keys')).toBeInTheDocument();
      expect(screen.getByText('Webhook URLs')).toBeInTheDocument();
      expect(screen.getByText('Integration Status')).toBeInTheDocument();
    });

    it('highlights the active tab', () => {
      renderPanel();
      const apiKeysTab = screen.getByText('API Keys');
      const parent = apiKeysTab.closest('button');
      expect(parent).toHaveClass('border-blue-500');
    });

    it('renders summary cards', () => {
      renderPanel();
      expect(screen.getByText('Active Keys')).toBeInTheDocument();
      expect(screen.getByText('Active Webhooks')).toBeInTheDocument();
      expect(screen.getByText('Healthy Integrations')).toBeInTheDocument();
    });

    it('shows active keys count in summary', () => {
      renderPanel();
      expect(screen.getByText('3 active keys · 2 active webhooks')).toBeInTheDocument();
    });

    it('renders API keys table by default', () => {
      renderPanel();
      expect(screen.getByText('CrowdStrike Production')).toBeInTheDocument();
      expect(screen.getByText('SentinelOne Staging')).toBeInTheDocument();
      expect(screen.getByText('Veeam Backup Primary')).toBeInTheDocument();
      expect(screen.getByText('Autotask Legacy')).toBeInTheDocument();
    });

    it('shows status badges for API keys', () => {
      renderPanel();
      const activeBadges = screen.getAllByText('active');
      expect(activeBadges.length).toBeGreaterThanOrEqual(3);
      expect(screen.getByText('revoked')).toBeInTheDocument();
    });
  });

  describe('tab switching', () => {
    it('switches to Webhook URLs tab when clicked', async () => {
      renderPanel();
      const user = userEvent.setup();
      await user.click(screen.getByText('Webhook URLs'));
      expect(screen.getByText('CrowdStrike Alert Ingestion')).toBeInTheDocument();
      expect(screen.getByText('Veeam Backup Sync')).toBeInTheDocument();
      expect(screen.getByText('Autotask Ticket Events')).toBeInTheDocument();
    });

    it('switches to Integration Status tab when clicked', async () => {
      renderPanel();
      const user = userEvent.setup();
      await user.click(screen.getByText('Integration Status'));
      expect(screen.getByText('CrowdStrike')).toBeInTheDocument();
      expect(screen.getByText('SentinelOne')).toBeInTheDocument();
      expect(screen.getByText('Veeam')).toBeInTheDocument();
    });
  });

  describe('API key management', () => {
    it('renders the "New Key" button', () => {
      renderPanel();
      expect(screen.getByRole('button', { name: 'New Key' })).toBeInTheDocument();
    });

    it('opens create key dialog when "New Key" is clicked', async () => {
      renderPanel();
      const user = userEvent.setup();
      await user.click(screen.getByRole('button', { name: 'New Key' }));
      expect(screen.getByText('Create API Key')).toBeInTheDocument();
    });

    it('shows integration type selector in create dialog', async () => {
      renderPanel();
      const user = userEvent.setup();
      await user.click(screen.getByRole('button', { name: 'New Key' }));
      // The integration type selector buttons contain the full integration names
      const buttons = screen.getAllByRole('button');
      const psaButton = buttons.find(btn =>
        btn.textContent?.includes('Professional Services')
      );
      expect(psaButton).toBeInTheDocument();
    });

    it('shows provider selector in create dialog for EDR', async () => {
      renderPanel();
      const user = userEvent.setup();
      await user.click(screen.getByRole('button', { name: 'New Key' }));
      // Use combobox role to find the select
      const select = screen.getByRole('combobox');
      expect(select).toBeInTheDocument();
    });

    it('shows provider selector options change based on integration type', async () => {
      renderPanel();
      const user = userEvent.setup();
      await user.click(screen.getByRole('button', { name: 'New Key' }));
      const select = screen.getByRole('combobox');
      // Switch to Backup
      const integrationButtons = screen.getAllByRole('button');
      const backupButton = integrationButtons.find(btn =>
        btn.textContent?.includes('Backup & Recovery')
      );
      if (backupButton) {
        await user.click(backupButton);
      }
      // The select options should have changed
      const optionTexts = Array.from(select.querySelectorAll('option')).map(o => o.textContent);
      expect(optionTexts).toContain('Veeam');
      expect(optionTexts).toContain('Commvault');
    });

    it('creates a new API key when form is submitted', async () => {
      renderPanel();
      const user = userEvent.setup();
      await user.click(screen.getByRole('button', { name: 'New Key' }));
      const input = screen.getByPlaceholderText('e.g., CrowdStrike Production');
      await user.type(input, 'Test Key');
      await user.click(screen.getByRole('button', { name: 'Create Key' }));
      expect(screen.getByText('Test Key')).toBeInTheDocument();
      expect(screen.getByText('API key "Test Key" created')).toBeInTheDocument();
    });

    it('validates key name is not empty', async () => {
      renderPanel();
      const user = userEvent.setup();
      await user.click(screen.getByRole('button', { name: 'New Key' }));
      await user.click(screen.getByRole('button', { name: 'Create Key' }));
      expect(screen.getByText('Key name is required')).toBeInTheDocument();
    });

    it('closes create dialog when Cancel is clicked', async () => {
      renderPanel();
      const user = userEvent.setup();
      await user.click(screen.getByRole('button', { name: 'New Key' }));
      expect(screen.getByText('Create API Key')).toBeInTheDocument();
      await user.click(screen.getByRole('button', { name: 'Cancel' }));
      expect(screen.queryByText('Create API Key')).not.toBeInTheDocument();
    });

    it('closes create dialog when clicking outside', async () => {
      renderPanel();
      const user = userEvent.setup();
      await user.click(screen.getByRole('button', { name: 'New Key' }));
      expect(screen.getByText('Create API Key')).toBeInTheDocument();
      // Click the overlay (the outer div with bg-black/50) to close the dialog
      const overlay = document.querySelector('.bg-black\\/50');
      if (overlay) {
        await user.click(overlay);
      }
      expect(screen.queryByText('Create API Key')).not.toBeInTheDocument();
    });

    it('toggles key visibility when eye icon is clicked', async () => {
      renderPanel();
      const user = userEvent.setup();
      const eyeButtons = screen.getAllByTitle('Show key');
      if (eyeButtons.length > 0) {
        await user.click(eyeButtons[0]);
        expect(eyeButtons[0]).toHaveAttribute('title', 'Hide key');
      }
    });

    it('copies API key to clipboard when copy button is clicked', async () => {
      renderPanel();
      const user = userEvent.setup();
      const copyButtons = screen.getAllByTitle('Copy key');
      if (copyButtons.length > 0) {
        await user.click(copyButtons[0]);
        expect(screen.getByText('API key copied to clipboard')).toBeInTheDocument();
      }
    });

    it('shows revoke button for active keys', () => {
      renderPanel();
      const crowdStrikeRow = screen.getByText('CrowdStrike Production').closest('tr');
      expect(crowdStrikeRow).toBeInTheDocument();
      const revokeBtn = crowdStrikeRow?.querySelector('[title="Revoke key"]');
      expect(revokeBtn).toBeInTheDocument();
    });

    it('revokes a key when confirmation is confirmed', async () => {
      renderPanel();
      const user = userEvent.setup();
      const crowdStrikeRow = screen.getByText('CrowdStrike Production').closest('tr');
      const revokeBtn = crowdStrikeRow?.querySelector('[title="Revoke key"]');
      if (revokeBtn) {
        await user.click(revokeBtn);
        await user.click(screen.getByRole('button', { name: 'Revoke Key' }));
        expect(screen.getByText('API key revoked')).toBeInTheDocument();
      }
    });

    it('updates active keys count after revoking a key', async () => {
      renderPanel();
      const user = userEvent.setup();
      expect(screen.getByText('3 active keys · 2 active webhooks')).toBeInTheDocument();
      const crowdStrikeRow = screen.getByText('CrowdStrike Production').closest('tr');
      const revokeBtn = crowdStrikeRow?.querySelector('[title="Revoke key"]');
      if (revokeBtn) {
        await user.click(revokeBtn);
        await user.click(screen.getByRole('button', { name: 'Revoke Key' }));
        expect(screen.getByText('2 active keys · 2 active webhooks')).toBeInTheDocument();
      }
    });
  });

  describe('webhook management', () => {
    it('renders webhook URLs in table', async () => {
      renderPanel();
      const user = userEvent.setup();
      await user.click(screen.getByText('Webhook URLs'));
      expect(screen.getByText('/api/v1/integrations/edr/alerts')).toBeInTheDocument();
      expect(screen.getByText('/api/v1/integrations/backup/sync')).toBeInTheDocument();
      expect(screen.getByText('/api/v1/integrations/psa/webhooks')).toBeInTheDocument();
    });

    it('copies webhook URL to clipboard', async () => {
      renderPanel();
      const user = userEvent.setup();
      await user.click(screen.getByText('Webhook URLs'));
      const copyBtns = screen.getAllByTitle('Copy URL');
      if (copyBtns.length > 0) {
        await user.click(copyBtns[0]);
        expect(screen.getByText('Webhook URL copied')).toBeInTheDocument();
      }
    });

    it('copies webhook secret to clipboard', async () => {
      renderPanel();
      const user = userEvent.setup();
      await user.click(screen.getByText('Webhook URLs'));
      const copyBtns = screen.getAllByTitle('Copy secret');
      if (copyBtns.length > 0) {
        await user.click(copyBtns[0]);
        expect(screen.getByText('Webhook secret copied')).toBeInTheDocument();
      }
    });

    it('toggles webhook enabled status when clicked', async () => {
      renderPanel();
      const user = userEvent.setup();
      await user.click(screen.getByText('Webhook URLs'));
      const autotaskRow = screen.getByText('Autotask Ticket Events').closest('tr');
      const statusBadge = within(autotaskRow!).getByText('disabled');
      expect(statusBadge).toBeInTheDocument();
      await user.click(statusBadge);
      expect(screen.getByText('Webhook configuration updated')).toBeInTheDocument();
    });

    it('shows delete button for webhooks', async () => {
      renderPanel();
      const user = userEvent.setup();
      await user.click(screen.getByText('Webhook URLs'));
      const veeamRow = screen.getByText('Veeam Backup Sync').closest('tr');
      expect(veeamRow).toBeInTheDocument();
      const deleteBtn = veeamRow?.querySelector('[title="Delete webhook"]');
      expect(deleteBtn).toBeInTheDocument();
    });

    it('deletes a webhook when confirmation is confirmed', async () => {
      renderPanel();
      const user = userEvent.setup();
      await user.click(screen.getByText('Webhook URLs'));
      const veeamRow = screen.getByText('Veeam Backup Sync').closest('tr');
      const deleteBtn = veeamRow?.querySelector('[title="Delete webhook"]');
      if (deleteBtn) {
        await user.click(deleteBtn);
        await user.click(screen.getByRole('button', { name: 'Delete Webhook' }));
        expect(screen.getByText('Webhook configuration deleted')).toBeInTheDocument();
      }
    });

    it('updates active webhooks count after toggling', async () => {
      renderPanel();
      const user = userEvent.setup();
      await user.click(screen.getByText('Webhook URLs'));
      expect(screen.getByText('3 active keys · 2 active webhooks')).toBeInTheDocument();
      const autotaskRow = screen.getByText('Autotask Ticket Events').closest('tr');
      const statusBadge = within(autotaskRow!).getByText('disabled');
      await user.click(statusBadge);
      expect(screen.getByText('3 active keys · 3 active webhooks')).toBeInTheDocument();
    });
  });

  describe('integration status', () => {
    it('renders status cards when status tab is active', async () => {
      renderPanel();
      const user = userEvent.setup();
      await user.click(screen.getByText('Integration Status'));
      expect(screen.getByText('CrowdStrike')).toBeInTheDocument();
      expect(screen.getByText('SentinelOne')).toBeInTheDocument();
    });

    it('shows alert counts for each integration', async () => {
      renderPanel();
      const user = userEvent.setup();
      await user.click(screen.getByText('Integration Status'));
      const countElements = screen.getAllByText('1247');
      // Should appear twice (received + processed)
      expect(countElements.length).toBe(2);
      expect(screen.getByText('83')).toBeInTheDocument();
    });

    it('shows refresh button in status tab', async () => {
      renderPanel();
      const user = userEvent.setup();
      await user.click(screen.getByText('Integration Status'));
      expect(screen.getByRole('button', { name: 'Refresh' })).toBeInTheDocument();
    });

    it('shows spinner when refreshing statuses', async () => {
      renderPanel();
      const user = userEvent.setup();
      await user.click(screen.getByText('Integration Status'));
      await user.click(screen.getByRole('button', { name: 'Refresh' }));
      const refreshBtn = screen.getByRole('button', { name: 'Refresh' });
      const icon = refreshBtn.querySelector('svg');
      expect(icon).toHaveClass('animate-spin');
    });
  });

  describe('integration type providers', () => {
    it('shows correct providers for Backup integration type', async () => {
      renderPanel();
      const user = userEvent.setup();
      await user.click(screen.getByRole('button', { name: 'New Key' }));
      const select = screen.getByRole('combobox');
      // Switch to Backup
      const integrationButtons = screen.getAllByRole('button');
      const backupButton = integrationButtons.find(btn =>
        btn.textContent?.includes('Backup & Recovery')
      );
      if (backupButton) {
        await user.click(backupButton);
      }
      const optionTexts = Array.from(select.querySelectorAll('option')).map(o => o.textContent);
      expect(optionTexts).toContain('Veeam');
      expect(optionTexts).toContain('Commvault');
      expect(optionTexts).toContain('Druva');
      expect(optionTexts).not.toContain('CrowdStrike');
    });

    it('shows correct providers for PSA integration type', async () => {
      renderPanel();
      const user = userEvent.setup();
      await user.click(screen.getByRole('button', { name: 'New Key' }));
      const select = screen.getByRole('combobox');
      // Switch to PSA
      const integrationButtons = screen.getAllByRole('button');
      const psaButton = integrationButtons.find(btn =>
        btn.textContent?.includes('Professional Services')
      );
      if (psaButton) {
        await user.click(psaButton);
      }
      const optionTexts = Array.from(select.querySelectorAll('option')).map(o => o.textContent);
      expect(optionTexts).toContain('Autotask');
      expect(optionTexts).toContain('ConnectWise');
      expect(optionTexts).toContain('Freshservice');
      expect(optionTexts).toContain('Zendesk');
    });
  });
});
