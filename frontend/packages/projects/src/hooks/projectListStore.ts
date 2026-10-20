import { api, unwrap, type ProjectSummary } from '../client';
import { PROJECTS_CHANGED } from '../host/ProjectsHost';
import { openEventStream, type ProjectEvent } from './useProjectEvents';

export const POLL_MS = 30_000;
export const RELOAD_DEBOUNCE_MS = 300;

/**
 * The one project list of the page: a module-level store, so the sidebar, the
 * folder page and the IM project directory (which may sit under different
 * host providers) read the same answer from a single event stream, poll and
 * request. Ref-counted: the first subscriber starts the stream and poll, the
 * last one stops them.
 */
interface Store {
    list: ProjectSummary[] | null;
    subscribers: number;
    changeListeners: Set<() => void>;
    eventListeners: Set<(ev: ProjectEvent) => void>;
    openListeners: Set<() => void>;
    latest: number;
    timer?: number;
    stop?: () => void;
}

const store: Store = {
    list: null,
    subscribers: 0,
    changeListeners: new Set(),
    eventListeners: new Set(),
    openListeners: new Set(),
    latest: 0,
};

export const getProjectList = (): ProjectSummary[] | null => store.list;

/** Fetch now. Only the latest answer lands. */
export function reloadProjectList(): void {
    if (document.visibilityState === 'hidden') return;
    const request = ++store.latest;
    unwrap(api.GET('/api/v1/projects'))
        .then((r) => {
            if (request !== store.latest) return;
            store.list = r.projects;
            store.changeListeners.forEach((l) => l());
        })
        .catch(() => {
            // Unreachable: keep the last list.
        });
}

/**
 * Events, page reports and tab changes come in bursts (a turn writes many,
 * and a page reports each): one reload per debounce window, whoever asked.
 */
export function reloadProjectListSoon(): void {
    window.clearTimeout(store.timer);
    store.timer = window.setTimeout(reloadProjectList, RELOAD_DEBOUNCE_MS);
}

const onEvent = (ev: ProjectEvent) => {
    if (ev.type === 'thread' || ev.type === 'project') reloadProjectListSoon();
    store.eventListeners.forEach((l) => l(ev));
};

const onOpen = () => {
    reloadProjectList();
    store.openListeners.forEach((l) => l());
};

function start(): () => void {
    reloadProjectList();
    const poll = window.setInterval(reloadProjectList, POLL_MS);
    window.addEventListener(PROJECTS_CHANGED, reloadProjectListSoon);
    document.addEventListener('visibilitychange', reloadProjectListSoon);
    const close = openEventStream('/api/v1/projects/events', onEvent, onOpen);
    return () => {
        store.latest++;
        window.clearInterval(poll);
        window.clearTimeout(store.timer);
        window.removeEventListener(PROJECTS_CHANGED, reloadProjectListSoon);
        document.removeEventListener('visibilitychange', reloadProjectListSoon);
        close();
    };
}

/** Joins the shared list; returns the leave function. */
export function subscribeProjectList(
    onChange: () => void,
    onProjectEvent?: (ev: ProjectEvent) => void,
    onReopen?: () => void,
): () => void {
    store.changeListeners.add(onChange);
    if (onProjectEvent) store.eventListeners.add(onProjectEvent);
    if (onReopen) store.openListeners.add(onReopen);
    if (store.subscribers++ === 0) store.stop = start();
    return () => {
        store.changeListeners.delete(onChange);
        if (onProjectEvent) store.eventListeners.delete(onProjectEvent);
        if (onReopen) store.openListeners.delete(onReopen);
        if (--store.subscribers === 0) {
            store.stop?.();
            store.stop = undefined;
        }
    };
}
