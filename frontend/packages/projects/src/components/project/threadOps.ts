// What a thread page does to its thread, over either set of routes: a
// project's thread, or a folder session (a thread with no project). The
// panel works with one of these so it never branches on the URL itself.
import { api, unwrap, type Attachment } from '../../client';

export interface ThreadOps {
    send: (content: string, attachments?: Attachment[]) => Promise<unknown>;
    respond: (requestId: string, allow: boolean, remember: boolean, answer?: string, answers?: Record<string, string>) => Promise<unknown>;
    stop: () => Promise<unknown>;
    /** Resolve a thread; archive a session. */
    finish: () => Promise<unknown>;
    reopen: () => Promise<unknown>;
    /** Release a session to a terminal: the command that resumes it. Threads have none. */
    handoff: () => Promise<string>;
    /** When a question with a suggested answer is answered without the user. */
    setAutoAnswer: (mode: '' | 'off' | 'delay' | 'now') => Promise<unknown>;
}

export function projectThreadOps(projectId: string, tid: string): ThreadOps {
    const path = { params: { path: { id: projectId, tid } } };
    return {
        send: (content, attachments) => unwrap(api.POST('/api/v1/projects/{id}/threads/{tid}/messages', { ...path, body: { content, attachments: attachments?.map((a) => a.path) } })),
        respond: (request_id, allow, remember, answer, answers) => unwrap(api.POST('/api/v1/projects/{id}/threads/{tid}/respond', {
            ...path, body: { request_id, allow, scope: remember ? 'thread' : 'once', answer: answer || undefined, answers: answers && Object.keys(answers).length ? answers : undefined },
        })),
        stop: () => unwrap(api.POST('/api/v1/projects/{id}/threads/{tid}/stop', path)),
        finish: () => unwrap(api.POST('/api/v1/projects/{id}/threads/{tid}/resolve', path)),
        reopen: () => unwrap(api.POST('/api/v1/projects/{id}/threads/{tid}/reopen', path)),
        handoff: () => Promise.reject(new Error('a project thread is continued in its worktree')),
        setAutoAnswer: (auto_answer) => unwrap(api.PUT('/api/v1/projects/{id}/threads/{tid}/auto-answer', { ...path, body: { auto_answer } })),
    };
}

export function sessionOps(sid: string): ThreadOps {
    const path = { params: { path: { sid } } };
    return {
        send: (content, attachments) => unwrap(api.POST('/api/v1/projects/sessions/{sid}/messages', { ...path, body: { content, attachments: attachments?.map((a) => a.path) } })),
        respond: (request_id, allow, remember, answer, answers) => unwrap(api.POST('/api/v1/projects/sessions/{sid}/respond', {
            ...path, body: { request_id, allow, scope: remember ? 'thread' : 'once', answer: answer || undefined, answers: answers && Object.keys(answers).length ? answers : undefined },
        })),
        stop: () => unwrap(api.POST('/api/v1/projects/sessions/{sid}/stop', path)),
        finish: () => unwrap(api.POST('/api/v1/projects/sessions/{sid}/archive', path)),
        reopen: () => unwrap(api.POST('/api/v1/projects/sessions/{sid}/reopen', path)),
        handoff: async () => (await unwrap(api.POST('/api/v1/projects/sessions/{sid}/handoff', path))).command,
        setAutoAnswer: (auto_answer) => unwrap(api.PUT('/api/v1/projects/sessions/{sid}/auto-answer', { ...path, body: { auto_answer } })),
    };
}
