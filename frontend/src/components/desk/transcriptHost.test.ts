import {afterEach, describe, expect, it} from 'vitest';
import {deskDrafts} from './transcriptHost';

afterEach(() => sessionStorage.clear());

describe('deskDrafts', () => {
    it('keeps request drafts in the tab under the keys Desk always used', () => {
        const decode = (saved: unknown) => {
            const s = saved && typeof saved === 'object' ? saved as {answer?: unknown} : {};
            return {answer: typeof s.answer === 'string' ? s.answer : ''};
        };
        expect(deskDrafts.read('requestDraft:q1', decode)).toEqual({answer: ''});
        deskDrafts.write('requestDraft:q1', {answer: 'main', status: 'idle'});
        expect(JSON.parse(sessionStorage.getItem('desk.requestDraft:q1')!)).toEqual({answer: 'main', status: 'idle'});
        expect(deskDrafts.read('requestDraft:q1', decode)).toEqual({answer: 'main', status: 'idle'});
    });
});
