import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Thread } from '../../client';
import { autoAnswerOf, ComposerStrip } from './ComposerStrip';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

afterEach(() => {
    cleanup();
    localStorage.clear();
});

const thread = (o: Partial<Thread>): Thread => ({
    id: 't', project_id: 'p', title: 'Fix login', task: '', status: 'working', origin: 'user', worktree: '/work/app',
    commits_ahead: 0, files_changed: 0, uncommitted: false, turns: 0, input_tokens: 0, output_tokens: 0, created_at: '', updated_at: '', ...o,
} as Thread);

describe('ComposerStrip', () => {
    it('defaults to waiting ten minutes in a project thread and to off in a folder session', () => {
        expect(autoAnswerOf(thread({}))).toBe('delay');
        expect(autoAnswerOf(thread({ project_id: '' }))).toBe('off');
        expect(autoAnswerOf(thread({ project_id: '', auto_answer: 'now' }))).toBe('now');
    });

    it('folds to the fact that matters now plus a count, and unfolds to one line per fact', () => {
        const onAutoAnswer = vi.fn();
        const th = thread({ branch: 'tb/fix-login', base_branch: 'main' });
        const { rerender } = render(<ComposerStrip thread={th} onAutoAnswer={onAutoAnswer} />);
        // Folded: the branch leads, the folder and the setting are counted.
        expect(screen.getByText('tb/fix-login')).toBeInTheDocument();
        expect(screen.getByText('+2')).toBeInTheDocument();
        expect(screen.queryByText('app')).toBeNull();
        // A question about to answer itself takes the line.
        rerender(<ComposerStrip thread={th} onAutoAnswer={onAutoAnswer} autoAt={new Date().toISOString()} />);
        expect(screen.getByText('strip.autoAt')).toBeInTheDocument();
        expect(screen.queryByText('tb/fix-login')).toBeNull();
        fireEvent.click(screen.getByRole('button', { name: 'strip.expand' }));
        expect(screen.getByText('/work/app')).toBeInTheDocument();
        expect(screen.getByText('tb/fix-login')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'strip.autoAnswer.change' }));
        fireEvent.click(screen.getByText('strip.autoAnswer.now'));
        expect(onAutoAnswer).toHaveBeenCalledWith('now');
    });

    it('names a folder session’s branch, and drops auto-answer once resolved', () => {
        render(<ComposerStrip thread={thread({ project_id: '', head_branch: 'main', status: 'resolved' })} onAutoAnswer={vi.fn()} />);
        expect(screen.getByText('main')).toBeInTheDocument();
        expect(screen.queryByText('strip.autoAnswer.off')).toBeNull();
    });
});
