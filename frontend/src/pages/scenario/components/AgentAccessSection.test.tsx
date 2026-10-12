import { act, fireEvent, render, renderHook, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import AgentAccessSection, { useAccessExpanded } from './AgentAccessSection';

// No i18n instance in tests: t() returns the key, which is what we match on.
describe('AgentAccessSection', () => {
    it('folds to an arrow and the URL; the whole line opens it', () => {
        const onToggle = vi.fn();
        render(
            <AgentAccessSection expanded={false} onToggle={onToggle} url="http://localhost:1/tingly/x">
                <div>details</div>
            </AgentAccessSection>,
        );
        expect(screen.getByText('http://localhost:1/tingly/x')).toBeInTheDocument();
        expect(screen.queryByText('details')).toBeNull();
        // nothing else on the folded line
        expect(screen.getAllByRole('button')).toHaveLength(1);
        fireEvent.click(screen.getByRole('button', { name: 'agentAccess.expand' }));
        expect(onToggle).toHaveBeenCalledOnce();
    });

    it('shows the details, not the URL line, when open', () => {
        render(
            <AgentAccessSection expanded onToggle={vi.fn()} url="http://localhost:1/tingly/x">
                <div>details</div>
            </AgentAccessSection>,
        );
        expect(screen.getByText('details')).toBeInTheDocument();
        expect(screen.queryByText('http://localhost:1/tingly/x')).toBeNull();
    });
});

describe('useAccessExpanded', () => {
    beforeEach(() => localStorage.clear());

    it('follows the default until the user chooses, then remembers the choice', () => {
        const { result, rerender } = renderHook(({ def }) => useAccessExpanded('claude_code', def), { initialProps: { def: true } });
        expect(result.current.expanded).toBe(true);
        // connected: folds by itself
        rerender({ def: false });
        expect(result.current.expanded).toBe(false);
        // the user opens it: stays open whatever the default says
        act(() => result.current.toggle());
        expect(result.current.expanded).toBe(true);
        rerender({ def: false });
        expect(result.current.expanded).toBe(true);
        expect(localStorage.getItem('agent-access-expanded-claude_code')).toBe('true');
        // a later visit starts from the saved choice
        const again = renderHook(() => useAccessExpanded('claude_code', false));
        expect(again.result.current.expanded).toBe(true);
    });

    it('works without storage', () => {
        const spy = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('blocked'); });
        const { result } = renderHook(() => useAccessExpanded('x', false));
        expect(result.current.expanded).toBe(false);
        spy.mockRestore();
    });
});
