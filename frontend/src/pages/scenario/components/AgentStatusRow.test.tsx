import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import AgentStatusRow, { formatErrorRate, formatRelativeTime } from './AgentStatusRow';

// No i18n instance in tests: t() returns the key, which is what we match on.
const base = {
    connected: true,
    configStatus: null,
    onApplyConfig: vi.fn(),
    onOpenRequests: vi.fn(),
    onOpenUsage: vi.fn(),
    onOpenLog: vi.fn(),
};

describe('AgentStatusRow', () => {
    it('claims nothing until the first answer', () => {
        render(<AgentStatusRow {...base} observation={null} />);
        expect(screen.queryByTestId('agent-status-summary')).toBeNull();
        // the looks-in are always there
        expect(screen.getByRole('button', { name: 'agentStatus.requests' })).toBeInTheDocument();
    });

    it('says connected with the last request, today and error rate', () => {
        render(<AgentStatusRow {...base} observation={{ lastRequestAt: new Date().toISOString(), requestsToday: 200, errorsToday: 1 }} />);
        const text = screen.getByTestId('agent-status-summary').textContent ?? '';
        expect(text).toContain('agentStatus.connected');
        expect(text).toContain('agentStatus.lastRequest');
        expect(text).toContain('agentStatus.today');
        expect(text).toContain('agentStatus.errorRate');
    });

    it('asks to finish the setup when nothing has arrived and it is not connected', () => {
        render(<AgentStatusRow {...base} connected={false} observation={{ lastRequestAt: null, requestsToday: 0, errorsToday: 0 }} />);
        expect(screen.getByTestId('agent-status-summary').textContent).toBe('agentStatus.noRequestsFinishSetup');
    });

    it('offers Reapply as the primary action when the config is out of date', () => {
        const onApplyConfig = vi.fn();
        render(
            <AgentStatusRow
                {...base}
                onApplyConfig={onApplyConfig}
                observation={{ lastRequestAt: null, requestsToday: 0, errorsToday: 0 }}
                configStatus={{ state: 'outdated', differences: [{ key: 'a', applied: '1', expected: '2' }] } as never}
            />,
        );
        fireEvent.click(screen.getByRole('button', { name: 'clientConfigStatus.reapply' }));
        expect(onApplyConfig).toHaveBeenCalledOnce();
    });

    it('opens the log', () => {
        const onOpenLog = vi.fn();
        render(<AgentStatusRow {...base} onOpenLog={onOpenLog} observation={null} />);
        fireEvent.click(screen.getByRole('button', { name: 'agentStatus.log' }));
        expect(onOpenLog).toHaveBeenCalledOnce();
    });

    it('opens requests and usage', () => {
        const onOpenRequests = vi.fn();
        const onOpenUsage = vi.fn();
        render(<AgentStatusRow {...base} onOpenRequests={onOpenRequests} onOpenUsage={onOpenUsage} observation={null} />);
        fireEvent.click(screen.getByRole('button', { name: 'agentStatus.requests' }));
        fireEvent.click(screen.getByRole('button', { name: 'agentStatus.usage' }));
        expect(onOpenRequests).toHaveBeenCalledOnce();
        expect(onOpenUsage).toHaveBeenCalledOnce();
    });
});

describe('formatters', () => {
    it('formats the error rate', () => {
        expect(formatErrorRate(0, 0)).toBeNull();
        expect(formatErrorRate(200, 0)).toBe('0%');
        expect(formatErrorRate(200, 1)).toBe('0.5%');
        expect(formatErrorRate(10, 3)).toBe('30%');
    });

    it('formats relative time', () => {
        const now = Date.parse('2026-10-11T12:00:00Z');
        expect(formatRelativeTime('2026-10-11T11:58:00Z', 'en', now)).toBe('2 minutes ago');
        expect(formatRelativeTime('nope', 'en', now)).toBe('');
    });
});
