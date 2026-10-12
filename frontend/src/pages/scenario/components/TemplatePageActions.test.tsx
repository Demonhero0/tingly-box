import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { TemplatePageActions } from './TemplatePageActions';

// No i18n instance in tests: t() returns the key, which is what we match on.
const base = {
    collapsible: true,
    allExpanded: true,
    onToggleExpandAll: vi.fn(),
    onAddApiKeyClick: vi.fn(),
    allowAddRule: true,
    onCreateRule: vi.fn(),
};

describe('TemplatePageActions', () => {
    it('keeps Test All and Connect AI out of the menu, and the rest in it', () => {
        const onViewLogs = vi.fn();
        const onToggleSort = vi.fn();
        const onShowGuide = vi.fn();
        const onToggleExpandAll = vi.fn();
        render(
            <TemplatePageActions
                {...base}
                onProbeAll={vi.fn()}
                onViewLogs={onViewLogs}
                onShowGuide={onShowGuide}
                sortMode="original"
                onToggleSort={onToggleSort}
                onToggleExpandAll={onToggleExpandAll}
            />,
        );
        expect(screen.getByRole('button', { name: /probe.testAll/ })).toBeInTheDocument();
        expect(screen.getByRole('button', { name: /templateActions.connectAI/ })).toBeInTheDocument();
        // Troubleshoot is no longer a toolbar button
        expect(screen.queryByRole('button', { name: 'templateActions.troubleshoot' })).toBeNull();

        const open = () => fireEvent.click(screen.getByRole('button', { name: 'templateActions.more' }));
        open();
        fireEvent.click(screen.getByRole('menuitem', { name: 'templateActions.troubleshoot' }));
        expect(onViewLogs).toHaveBeenCalledOnce();

        open();
        fireEvent.click(screen.getByRole('menuitem', { name: 'templateActions.sortMenuToName' }));
        expect(onToggleSort).toHaveBeenCalledOnce();

        open();
        fireEvent.click(screen.getByRole('menuitem', { name: 'templateActions.collapseAllRules' }));
        expect(onToggleExpandAll).toHaveBeenCalledOnce();

        open();
        fireEvent.click(screen.getByRole('menuitem', { name: /howRoutingWorks|How routing works/ }));
        expect(onShowGuide).toHaveBeenCalledOnce();
    });

    it('has no menu when there is nothing to put in it', () => {
        render(<TemplatePageActions {...base} collapsible={false} />);
        expect(screen.queryByRole('button', { name: 'templateActions.more' })).toBeNull();
    });
});
