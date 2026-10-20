import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ProjectsHostProvider, type ProjectsHost } from '../host/ProjectsHost';
import WorkdirPage from './WorkdirPage';

const m = vi.hoisted(() => ({ get: vi.fn(), del: vi.fn(), post: vi.fn(), folders: [] as unknown[], reload: vi.fn() }));
vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (key: string, o?: Record<string, unknown>) => (o ? `${key} ${JSON.stringify(o)}` : key), i18n: { language: 'en' } }),
}));
vi.mock('../client', async (actual) => ({ ...(await actual<typeof import('../client')>()), api: { GET: m.get, DELETE: m.del, POST: m.post } }));
vi.mock('../hooks/useProjectList', () => ({ useProjectList: () => ({ folders: m.folders, all: m.folders, projects: [], waiting: 0, reload: m.reload }) }));

const folder = (working: number, waiting: number) => ({
    id: 'f1', name: 'app', path: '/home/me/app', type: 'folder', starred: false,
    threads: { working, waiting, ready: 0, idle: 0, resolved: 0 },
});
const host: ProjectsHost = { paths: { list: '/p', project: (id) => `/p/${id}`, folder: () => '/f' } };

const show = (entry = '/?path=%2Fhome%2Fme%2Fapp') => render(
    <MemoryRouter initialEntries={[entry]}><ProjectsHostProvider host={host}><WorkdirPage /></ProjectsHostProvider></MemoryRouter>,
);

beforeEach(() => {
    m.get.mockImplementation(async (url: string) => ({
        data: url.endsWith('permission-modes') ? { modes: [] } : { threads: [] },
        response: new Response(),
    }));
    m.del.mockResolvedValue({ data: {}, response: new Response() });
});
afterEach(() => {
    cleanup();
    for (const f of [m.get, m.del, m.post, m.reload]) f.mockReset();
});

describe('WorkdirPage removing a folder', () => {
    it('asks first, counting the sessions it would stop', async () => {
        m.folders = [folder(1, 1)];
        show();
        fireEvent.click(await screen.findByRole('button', { name: 'workdir.remove' }));
        expect(await screen.findByText(/workdir\.removeRunning.*"count":2/)).toBeInTheDocument();
        expect(m.del).not.toHaveBeenCalled();
        fireEvent.click(screen.getByRole('button', { name: 'workdir.removeConfirm' }));
        await waitFor(() => expect(m.del).toHaveBeenCalledTimes(1));
    });

    it('removes at once when nothing is running', async () => {
        m.folders = [folder(0, 0)];
        show();
        fireEvent.click(await screen.findByRole('button', { name: 'workdir.remove' }));
        await waitFor(() => expect(m.del).toHaveBeenCalledTimes(1));
        expect(screen.queryByText(/workdir\.removeRunning/)).not.toBeInTheDocument();
    });

    it('keeps Attach disabled in the new-session prompt until a folder is chosen', async () => {
        m.folders = [];
        show('/');
        expect(await screen.findByRole('button', { name: 'composer.attach' })).toBeDisabled();
    });
});
