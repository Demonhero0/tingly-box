import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PROJECTS_CHANGED } from '../host/ProjectsHost';
import { RELOAD_DEBOUNCE_MS, reloadProjectListSoon, subscribeProjectList } from './projectListStore';
import type { ProjectEvent } from './useProjectEvents';

const m = vi.hoisted(() => ({ get: vi.fn(), open: vi.fn(), close: vi.fn(), emit: undefined as undefined | ((ev: ProjectEvent) => void) }));
vi.mock('../client', async (actual) => ({ ...(await actual<typeof import('../client')>()), api: { GET: m.get } }));
vi.mock('./useProjectEvents', () => ({
    openEventStream: (url: string, onEvent: (ev: ProjectEvent) => void) => {
        m.open(url);
        m.emit = onEvent;
        return m.close;
    },
}));

const thread = (): ProjectEvent => ({ type: 'thread', project_id: 'f1', data: {} });

beforeEach(() => {
    vi.useFakeTimers();
    m.get.mockResolvedValue({ data: { projects: [] }, response: new Response() });
});
afterEach(() => {
    vi.useRealTimers();
    for (const f of [m.get, m.open, m.close]) f.mockReset();
});

describe('shared project list', () => {
    it('opens one stream and one request for any number of subscribers, and closes with the last', () => {
        const leave = [subscribeProjectList(() => undefined), subscribeProjectList(() => undefined), subscribeProjectList(() => undefined)];
        expect(m.open).toHaveBeenCalledTimes(1);
        expect(m.open).toHaveBeenCalledWith('/api/v1/projects/events');
        expect(m.get).toHaveBeenCalledTimes(1);
        leave[0]();
        leave[1]();
        expect(m.close).not.toHaveBeenCalled();
        leave[2]();
        expect(m.close).toHaveBeenCalledTimes(1);
    });

    it('turns a burst of events and page reports into one fetch per debounce window', async () => {
        const leave = subscribeProjectList(() => undefined);
        await vi.advanceTimersByTimeAsync(0);
        m.get.mockClear();
        for (let i = 0; i < 4; i++) {
            m.emit?.(thread());
            window.dispatchEvent(new Event(PROJECTS_CHANGED));
            reloadProjectListSoon();
        }
        await vi.advanceTimersByTimeAsync(RELOAD_DEBOUNCE_MS - 1);
        expect(m.get).not.toHaveBeenCalled();
        await vi.advanceTimersByTimeAsync(1);
        expect(m.get).toHaveBeenCalledTimes(1);
        leave();
    });

    it('hands every event to the subscribers that asked for them', () => {
        const a = vi.fn();
        const b = vi.fn();
        const leaveA = subscribeProjectList(() => undefined, a);
        const leaveB = subscribeProjectList(() => undefined, b);
        const ev = { type: 'thread_event', project_id: 'f1', data: {} } as ProjectEvent;
        m.emit?.(ev);
        expect(a).toHaveBeenCalledWith(ev);
        expect(b).toHaveBeenCalledWith(ev);
        leaveA();
        leaveB();
    });
});
