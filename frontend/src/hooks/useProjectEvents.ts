import { useEffect, useRef } from 'react';

export interface ProjectEvent {
    type: 'message' | 'message_delta' | 'thread' | 'thread_event' | 'project' | 'coordinator';
    project_id: string;
    data: unknown;
}

/**
 * Subscribes to a project's live event stream. onOpen runs on every
 * (re)connect so the page can re-fetch anything it missed while offline.
 */
export function useProjectEvents(projectId: string, onEvent: (ev: ProjectEvent) => void, onOpen?: () => void) {
    const handler = useRef(onEvent);
    const opened = useRef(onOpen);
    handler.current = onEvent;
    opened.current = onOpen;

    useEffect(() => {
        if (!projectId) return;
        let first = true;
        const es = new EventSource(`/api/v1/projects/${projectId}/events`);
        es.onopen = () => {
            if (!first) opened.current?.();
            first = false;
        };
        es.onmessage = (m) => {
            try {
                handler.current(JSON.parse(m.data) as ProjectEvent);
            } catch {
                // ignore malformed frames
            }
        };
        return () => es.close();
    }, [projectId]);
}
