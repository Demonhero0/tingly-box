import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { api, unwrap, type ConversationSummary, type ProjectSummary, type Settings } from '@/client';

/**
 * Data the shell (sidebar) shows on every page: recent and starred chats,
 * starred projects and the connection settings. Pages call `refresh()` after
 * a change the sidebar should reflect.
 */
interface AppData {
    recents: ConversationSummary[];
    starredChats: ConversationSummary[];
    starredProjects: ProjectSummary[];
    settings: Settings | null;
    refresh: () => void;
    refreshSettings: () => void;
}

const Ctx = createContext<AppData | null>(null);

export function AppDataProvider({ children }: { children: ReactNode }) {
    const [recents, setRecents] = useState<ConversationSummary[]>([]);
    const [starredChats, setStarredChats] = useState<ConversationSummary[]>([]);
    const [starredProjects, setStarredProjects] = useState<ProjectSummary[]>([]);
    const [settings, setSettings] = useState<Settings | null>(null);
    const [tick, setTick] = useState(0);
    const [settingsTick, setSettingsTick] = useState(0);

    useEffect(() => {
        let alive = true;
        Promise.all([
            unwrap(api.GET('/api/v1/conversations', { params: { query: { limit: 30 } } })),
            unwrap(api.GET('/api/v1/conversations', { params: { query: { starred: true } } })),
            unwrap(api.GET('/api/v1/projects')),
        ]).then(([r, s, p]) => {
            if (!alive) return;
            setRecents(r.conversations);
            setStarredChats(s.conversations);
            setStarredProjects(p.projects.filter((x) => x.starred));
        }).catch(() => {
            // The sidebar stays as it was; pages surface their own errors.
        });
        return () => {
            alive = false;
        };
    }, [tick]);

    useEffect(() => {
        unwrap(api.GET('/api/v1/settings')).then(setSettings).catch(() => {});
    }, [settingsTick]);

    const refresh = useCallback(() => setTick((t) => t + 1), []);
    const refreshSettings = useCallback(() => setSettingsTick((t) => t + 1), []);
    const value = useMemo(
        () => ({ recents, starredChats, starredProjects, settings, refresh, refreshSettings }),
        [recents, starredChats, starredProjects, settings, refresh, refreshSettings],
    );
    return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useAppData() {
    const v = useContext(Ctx);
    if (!v) throw new Error('useAppData outside AppDataProvider');
    return v;
}
