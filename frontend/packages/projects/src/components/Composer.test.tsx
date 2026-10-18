import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Composer } from './Composer';

const { mobile } = vi.hoisted(() => ({ mobile: { value: false } }));
vi.mock('@mui/material', async (original) => ({ ...await original<typeof import('@mui/material')>(), useMediaQuery: () => mobile.value }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => ({ 'composer.sendShort': 'Send', 'composer.stop': 'Stop' })[key] ?? key }) }));
beforeEach(() => { mobile.value = false; });
afterEach(cleanup);

describe('Composer', () => {
    it('clears at once and puts a refused message back while the box is still empty', async () => {
        const send = vi.fn().mockResolvedValueOnce(false);
        render(<Composer placeholder="Reply" onSend={send} />);
        const input = screen.getByPlaceholderText('Reply');
        fireEvent.change(input, { target: { value: 'hello' } });
        await act(async () => fireEvent.keyDown(input, { key: 'Enter' }));
        expect(send).toHaveBeenCalledExactlyOnceWith('hello', []);
        expect(input).toHaveValue('hello');
    });

    it('keeps text until sent, keeps what is typed meanwhile, and never sends twice', async () => {
        let finish!: (accepted: boolean) => void;
        const send = vi.fn(() => new Promise<boolean>((resolve) => { finish = resolve; }));
        render(<Composer placeholder="Reply" keepUntilSent onSend={send} />);
        const input = screen.getByPlaceholderText('Reply');
        fireEvent.change(input, { target: { value: 'first message' } });
        fireEvent.keyDown(input, { key: 'Enter' });
        fireEvent.keyDown(input, { key: 'Enter' });
        fireEvent.change(input, { target: { value: 'next draft' } });
        await act(async () => finish(true));
        expect(send).toHaveBeenCalledExactlyOnceWith('first message', []);
        expect(input).toHaveValue('next draft');
    });

    it('keeps a failed send and clears only an accepted unchanged draft', async () => {
        const send = vi.fn().mockResolvedValueOnce(false).mockResolvedValueOnce(true);
        render(<Composer placeholder="Reply" keepUntilSent onSend={send} />);
        const input = screen.getByPlaceholderText('Reply');
        fireEvent.change(input, { target: { value: 'keep me' } });
        await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Send' })));
        expect(input).toHaveValue('keep me');
        await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Send' })));
        expect(input).toHaveValue('');
    });

    it('does not send when Enter confirms an IME candidate or adds a line', () => {
        const send = vi.fn();
        render(<Composer placeholder="Reply" onSend={send} />);
        const input = screen.getByPlaceholderText('Reply');
        fireEvent.change(input, { target: { value: '中文' } });
        fireEvent.keyDown(input, { key: 'Enter', isComposing: true });
        fireEvent.keyDown(input, { key: 'Enter', shiftKey: true });
        expect(send).not.toHaveBeenCalled();
    });

    it('on a phone, Enter adds a line and Ctrl+Enter sends', async () => {
        mobile.value = true;
        const send = vi.fn().mockResolvedValue(true);
        render(<Composer placeholder="Reply" onSend={send} />);
        const input = screen.getByRole('textbox', { name: 'Reply' });
        fireEvent.change(input, { target: { value: 'Mobile text' } });
        fireEvent.keyDown(input, { key: 'Enter' });
        fireEvent.keyDown(input, { key: 'Enter', ctrlKey: true, isComposing: true });
        expect(send).not.toHaveBeenCalled();
        await act(async () => fireEvent.keyDown(input, { key: 'Enter', ctrlKey: true }));
        expect(send).toHaveBeenCalledExactlyOnceWith('Mobile text', []);
    });

    it('while a turn runs: Stop once at a time, and Send only when the host takes messages meanwhile', async () => {
        let finish!: () => void;
        const stop = vi.fn(() => new Promise<void>((resolve) => { finish = resolve; }));
        const view = render(<Composer placeholder="Reply" streaming onStop={stop} onSend={vi.fn()} />);
        const button = screen.getByRole('button', { name: 'Stop' });
        fireEvent.click(button);
        fireEvent.click(button);
        expect(stop).toHaveBeenCalledOnce();
        expect(button).toBeDisabled();
        await act(async () => finish());
        expect(button).toBeEnabled();
        fireEvent.change(screen.getByPlaceholderText('Reply'), { target: { value: 'later' } });
        expect(screen.queryByRole('button', { name: 'Send' })).not.toBeInTheDocument();
        view.rerender(<Composer placeholder="Reply" streaming sendWhileStreaming onStop={stop} onSend={vi.fn()} />);
        expect(screen.getByRole('button', { name: 'Send' })).toBeEnabled();
    });
});

describe('Composer attachments', () => {
    const stored = (name: string) => ({ name, path: `uploads/x/${name}`, type: 'text/plain', size: 3 });

    it('turns a long paste into a text file and sends it with the message', async () => {
        const attach = vi.fn(async (files: File[]) => files.map((f) => stored(f.name)));
        const onSend = vi.fn(async () => true);
        render(<Composer attach={attach} onSend={onSend} />);
        const box = screen.getByRole('textbox');
        const long = Array.from({ length: 60 }, (_, i) => `line ${i}`).join('\n');
        fireEvent.paste(box, { clipboardData: { files: [], getData: () => long } });
        await act(async () => {});
        expect(attach).toHaveBeenCalledTimes(1);
        expect(attach.mock.calls[0][0][0].name).toBe('pasted-text.txt');
        expect((box as HTMLTextAreaElement).value).toBe('');
        // The file alone is enough to send.
        fireEvent.click(screen.getByRole('button', { name: 'Send' }));
        await act(async () => {});
        expect(onSend).toHaveBeenCalledWith('', [stored('pasted-text.txt')]);
    });

    it('leaves a short paste in the box and holds Send while a file uploads', async () => {
        let finish: (a: ReturnType<typeof stored>[]) => void = () => {};
        const attach = vi.fn(() => new Promise<ReturnType<typeof stored>[]>((r) => { finish = r; }));
        const onSend = vi.fn(async () => true);
        render(<Composer attach={attach} onSend={onSend} />);
        fireEvent.paste(screen.getByRole('textbox'), { clipboardData: { files: [], getData: () => 'short' } });
        expect(attach).not.toHaveBeenCalled();
        const file = new File(['abc'], 'notes.txt', { type: 'text/plain' });
        fireEvent.drop(screen.getByRole('textbox'), { dataTransfer: { files: [file], types: ['Files'] } });
        fireEvent.change(screen.getByRole('textbox'), { target: { value: 'see this' } });
        const send = screen.getByRole('button', { name: 'Send' });
        expect(send).toBeDisabled();
        await act(async () => finish([stored('notes.txt')]));
        expect(send).toBeEnabled();
        fireEvent.click(send);
        await act(async () => {});
        expect(onSend).toHaveBeenCalledWith('see this', [stored('notes.txt')]);
    });

    it('shows a failed upload and sends nothing', async () => {
        const attach = vi.fn(async () => { throw new Error('too big'); });
        render(<Composer attach={attach} onSend={vi.fn()} />);
        fireEvent.drop(screen.getByRole('textbox'), { dataTransfer: { files: [new File(['a'], 'a.bin')], types: ['Files'] } });
        await act(async () => {});
        expect(screen.getByRole('alert')).toHaveTextContent('too big');
        expect(screen.getByRole('button', { name: 'Send' })).toBeDisabled();
    });
});
