import {describe, expect, it} from 'vitest';
import type {MessageInfo, SessionInfo} from '@/services/deskApi';
import {backgroundTasks, cacheHitPct, groupSessionsByFolder, sessionUsage} from './deskUtils';

const msg = (m: Partial<MessageInfo>): MessageInfo => ({content: '', timestamp: '2026-01-01T00:00:00Z', ...m} as MessageInfo);

describe('groupSessionsByFolder', () => {
    it('keeps most-recent-first order between and within folders', () => {
        const s = (id: string, project: string) => ({id, project} as SessionInfo);
        const groups = groupSessionsByFolder([s('a', '/x'), s('b', '/y'), s('c', '/x')]);
        expect(groups.map((g) => [g.path, g.sessions.map((x) => x.id)])).toEqual([['/x', ['a', 'c']], ['/y', ['b']]]);
    });
});

describe('sessionUsage', () => {
    const usage = (payload: object) => msg({kind: 'usage', payload});

    it('totals the turns and keeps the latest turn for model and context', () => {
        const u = sessionUsage([
            msg({role: 'user', content: 'hi'}),
            usage({model: 'tingly/cc', input_tokens: 100, output_tokens: 20, cache_read_tokens: 900, cache_write_tokens: 0, context_tokens: 1000, context_window: 200000}),
            usage({model: 'tingly/cc', input_tokens: 50, output_tokens: 10, cache_read_tokens: 1950, cache_write_tokens: 0, context_tokens: 2000, context_window: 200000}),
        ]);
        expect(u).toMatchObject({input: 150, output: 30, cacheRead: 2850, cacheWrite: 0, latest: {context_tokens: 2000}});
        expect(cacheHitPct(u!)).toBe(95);
    });

    it('is undefined before any turn reached the model', () => {
        expect(sessionUsage([msg({role: 'user', content: 'hi'})])).toBeUndefined();
    });
});

describe('backgroundTasks', () => {
    const started = (call: string, id: string, type: string, description: string) =>
        msg({kind: 'task', request_id: call, payload: {event: 'task_started', task_id: id, task_type: type, background: true, description}});

    it('lists background work, running first, and settles what its process took down', () => {
        const rows = backgroundTasks([
            started('c1', 't1', 'local_bash', 'Old build'),
            msg({kind: 'task', request_id: 'c1', payload: {event: 'task_notification', status: 'completed'}}),
            started('c2', 't2', 'local_agent', 'Review'),
            started('c3', 't3', 'local_bash', 'Dev server'),
            msg({kind: 'task', request_id: 'c3', payload: {event: 'output_file', task_id: 't3', output_file: '/tmp/x/tasks/t3.output'}}),
            // A foreground subagent is not background work.
            msg({kind: 'task', request_id: 'c4', payload: {event: 'task_started', task_id: 't4', background: false}}),
        ], [{task_id: 't3', task_type: 'local_bash', description: 'Dev server'}]);

        expect(rows.map((r) => [r.taskId, r.status, r.ended])).toEqual([
            ['t3', 'running', false],
            ['t2', 'running', true],
            ['t1', 'completed', false],
        ]);
        expect(rows[0].outputFile).toBe('/tmp/x/tasks/t3.output');
    });

    it('carries what a row opens into: the call, and a subagent\'s latest steps and reply', () => {
        const rows = backgroundTasks([
            msg({kind: 'tool_use', content: 'Agent', request_id: 'a1', payload: {prompt: 'check races', subagent_type: 'Explore'}, timestamp: '2026-01-01T00:00:00Z'}),
            started('a1', 'x1', 'local_agent', 'Review'),
            msg({kind: 'tool_use', content: 'Read', request_id: 's1', parent: 'a1', payload: {file_path: 'auth.go'}}),
            msg({kind: 'tool_use', content: 'Grep', request_id: 's2', parent: 'a1', payload: {pattern: 'refresh'}}),
            msg({role: 'assistant', content: 'Found a race.', parent: 'a1'}),
        ], [{task_id: 'x1', task_type: 'local_agent', description: 'Review'}]);
        expect(rows[0]).toMatchObject({
            input: {prompt: 'check races'},
            startedAt: '2026-01-01T00:00:00Z',
            recent: [{name: 'Read', summary: 'auth.go'}, {name: 'Grep', summary: 'refresh'}],
            reply: 'Found a race.',
        });
    });
});
