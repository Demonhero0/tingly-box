import { setupServer } from 'msw/node';
import { afterAll, afterEach, beforeAll, describe, expect, it } from 'vitest';
import { createSkillHandlers } from './skillHandlers';

const server = setupServer();
const base = `${window.location.origin}/api/v2`;
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

const request = async (path: string, method = 'GET', body?: unknown) => {
    const response = await fetch(`${base}${path}`, {
        method,
        headers: { 'Content-Type': 'application/json' },
        ...(body !== undefined && { body: JSON.stringify(body) }),
    });
    return { status: response.status, body: await response.json() };
};

describe('mock skill catalog', () => {
    it('supports discovery, import, content reading and removal from an empty installation', async () => {
        server.use(...createSkillHandlers('newcomer'));
        expect((await request('/skill-locations')).body.data).toEqual([]);
        const discovered = (await request('/skill-locations/discover')).body.data;
        const imported = (await request('/skill-locations/import', 'POST', { locations: discovered.locations })).body.data;
        expect(imported).toHaveLength(1);
        const location = imported[0];
        const refreshed = (await request(`/skill-locations/${location.id}/refresh`, 'POST')).body.data;
        expect(refreshed.skills).toHaveLength(1);
        const skill = refreshed.skills[0];
        const query = new URLSearchParams({ location_id: location.id, skill_id: skill.id });
        const content = (await request(`/skill-content?${query}`)).body.data;
        expect(content.content).toContain('# Getting started');
        await request('/skill-locations/import', 'POST', { locations: discovered.locations });
        expect((await request('/skill-locations')).body.data).toHaveLength(1);
        expect((await request(`/skill-locations/${location.id}`, 'DELETE')).status).toBe(200);
        expect((await request(`/skill-locations/${location.id}/refresh`, 'POST')).status).toBe(404);
        expect((await request(`/skill-content?${query}`)).status).toBe(404);
    });

    it('rejects incomplete additions and retains valid additions in the catalog', async () => {
        server.use(...createSkillHandlers('newcomer'));
        expect((await request('/skill-locations', 'POST', { name: 'Empty' })).status).toBe(400);
        const added = await request('/skill-locations', 'POST', {
            name: 'Custom catalog', path: '/demo/custom', ide_source: 'custom',
        });
        expect(added.body.success).toBe(true);
        expect((await request('/skill-locations')).body.data).toContainEqual(added.body.data);
        expect((await request(`/skill-locations/${added.body.data.id}/refresh`, 'POST')).body.data.skills).toEqual([]);
    });
});
