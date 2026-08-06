import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import IntegrationDashboardPanel from './IntegrationDashboardPanel';

vi.mock('@/components/shared/Toast', () => ({
  useToast: () => ({
    showToast: vi.fn(),
  }),
}));

describe('IntegrationDashboardPanel', () => {
  it('renders dashboard header', () => {
    render(<IntegrationDashboardPanel />);
    expect(screen.getByText('Integration Dashboard')).toBeInTheDocument();
  });

  it('renders summary cards', () => {
    render(<IntegrationDashboardPanel />);
    expect(screen.getByText('Total Providers')).toBeInTheDocument();
    expect(screen.getByText('Devices Monitored')).toBeInTheDocument();
    expect(screen.getByText('Active Alerts')).toBeInTheDocument();
    expect(screen.getByText('Alerts Today')).toBeInTheDocument();
  });

  it('displays provider health cards', () => {
    render(<IntegrationDashboardPanel />);
    const crowdstrikeElements = screen.getAllByText('CrowdStrike');
    expect(crowdstrikeElements.length).toBeGreaterThan(0);
    const sentineloneElements = screen.getAllByText('SentinelOne');
    expect(sentineloneElements.length).toBeGreaterThan(0);
    const defenderElements = screen.getAllByText('Microsoft Defender');
    expect(defenderElements.length).toBeGreaterThan(0);
    const veeamElements = screen.getAllByText('Veeam');
    expect(veeamElements.length).toBeGreaterThan(0);
    const autotaskElements = screen.getAllByText('Autotask');
    expect(autotaskElements.length).toBeGreaterThan(0);
  });

  it('shows healthy status indicators', () => {
    render(<IntegrationDashboardPanel />);
    const healthyElements = screen.getAllByText(/Healthy|Degraded|Not Configured/);
    expect(healthyElements.length).toBeGreaterThan(3);
  });

  it('renders recent events feed', () => {
    render(<IntegrationDashboardPanel />);
    expect(screen.getByText(/Recent Events/)).toBeInTheDocument();
  });

  it('displays event severity labels', () => {
    render(<IntegrationDashboardPanel />);
    const severityElements = screen.getAllByText(/critical|warning|info/);
    expect(severityElements.length).toBeGreaterThan(2);
  });

  it('shows unacknowledged alert banner', () => {
    render(<IntegrationDashboardPanel />);
    expect(screen.getByText(/Unacknowledged Critical Alert/)).toBeInTheDocument();
  });

  it('renders refresh button', () => {
    render(<IntegrationDashboardPanel />);
    expect(screen.getByText('Refresh')).toBeInTheDocument();
  });

  it('renders filter controls', () => {
    render(<IntegrationDashboardPanel />);
    expect(screen.getByRole('combobox')).toBeInTheDocument();
    expect(screen.getByText('Unacknowledged Only')).toBeInTheDocument();
  });

  it('allows filtering events by severity', () => {
    render(<IntegrationDashboardPanel />);
    const select = screen.getByRole('combobox');
    fireEvent.change(select, { target: { value: 'critical' } });
    expect(select).toHaveValue('critical');
  });

  it('toggles unacknowledged filter', () => {
    render(<IntegrationDashboardPanel />);
    const toggleBtn = screen.getByText('Unacknowledged Only');
    fireEvent.click(toggleBtn);
    expect(toggleBtn).toHaveClass('bg-red-100');
  });

  it('shows provider device count', () => {
    render(<IntegrationDashboardPanel />);
    expect(screen.getByText('420')).toBeInTheDocument();
    expect(screen.getByText('156')).toBeInTheDocument();
  });

  it('shows provider latency', () => {
    render(<IntegrationDashboardPanel />);
    expect(screen.getByText('45ms')).toBeInTheDocument();
    expect(screen.getByText('230ms')).toBeInTheDocument();
  });

  it('shows provider uptime', () => {
    render(<IntegrationDashboardPanel />);
    expect(screen.getByText('99.97%')).toBeInTheDocument();
    expect(screen.getByText('98.5%')).toBeInTheDocument();
  });

  it('shows last sync times for providers', () => {
    render(<IntegrationDashboardPanel />);
    const lastSyncElements = screen.getAllByText(/Last Sync/);
    expect(lastSyncElements.length).toBeGreaterThan(0);
  });
});
