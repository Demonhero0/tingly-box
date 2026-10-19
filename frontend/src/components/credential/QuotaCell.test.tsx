import { cleanup, render } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { ProviderQuota, QuotaWindow } from '@/types/quota';
import { QuotaCell } from './QuotaCell';

// Keys come back as-is, with interpolated values appended, so a line's name
// and figure can be read off the rendered text.
vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, opts?: Record<string, unknown>) => (opts ? `${key}(${Object.values(opts).join(',')})` : key),
    }),
}));

afterEach(cleanup);

const BALANCE = 'providerTable.quota.balance';

function money(window: Partial<QuotaWindow> & Pick<QuotaWindow, 'key' | 'type' | 'label'>): QuotaWindow {
    return { used: 0, limit: 0, used_percent: 0, unit: 'currency', currency_code: 'USD', description: '', ...window };
}

// OpenRouter as the fetcher reports it: the account's credits, the key's
// spending cap, and this month's spend.
function openRouter(windows: QuotaWindow[]): ProviderQuota {
    return {
        provider_uuid: 'or',
        provider_name: 'OpenRouter',
        provider_type: 'openrouter',
        fetched_at: new Date().toISOString(),
        expires_at: new Date(Date.now() + 300_000).toISOString(),
        windows,
    } as ProviderQuota;
}

const balance = (available: number) => money({ key: 'balance', type: 'balance', kind: 'resource', label: 'Balance', available, unknown: true });
const keyLimit = (available: number, limit: number) =>
    money({ key: 'key_limit', type: 'custom', kind: 'resource', label: 'Key Limit', available, limit, used: limit - available });
const monthlySpend = (used: number) =>
    money({ key: 'monthly', type: 'monthly', label: 'Monthly', used, unlimited: true, window_minutes: 43200 });

describe('QuotaCell money lines', () => {
    it('names only a balance "Balance"; a key spending cap keeps its own label', () => {
        const { container } = render(
            <QuotaCell quota={openRouter([balance(0), keyLimit(50, 50), monthlySpend(0)])} refreshing={false} onRefresh={() => {}} />,
        );
        const text = container.textContent ?? '';
        expect(text.split(BALANCE)).toHaveLength(2);
        expect(text).toContain('Key Limit');
        expect(text).toContain('0.00 USD');
    });

    it('keeps the balance visible when a cap and a spend also want the two money lines', () => {
        const { container } = render(
            <QuotaCell
                quota={openRouter([keyLimit(49.99, 50), monthlySpend(0.01), balance(0)])}
                refreshing={false}
                onRefresh={() => {}}
            />,
        );
        const text = container.textContent ?? '';
        expect(text).toContain(BALANCE);
        expect(text).toContain('+1');
        expect(text).not.toContain('providerTable.quota.used');
    });
});
