// Typed API client generated from ../openapi.json (`pnpm gen:api`). Do not
// hand-edit schema.d.ts; regenerate it with `task codegen`.
import createClient from 'openapi-fetch';
import type { components, paths } from './schema';

export type { components, paths };
type S = components['schemas'];

export type Project = S['Project'];
export type ProjectSummary = S['ProjectSummary'];
export type ProjectDetail = S['ProjectDetailResponse'];
export type Capacity = S['Capacity'];
export type KnowledgeFileSummary = S['KnowledgeFileSummary'];
export type KnowledgeFile = S['KnowledgeFile'];
export type ContextResponse = S['ContextResponse'];
export type Conversation = S['Conversation'];
export type ConversationSummary = S['ConversationSummary'];
export type ConversationDetail = S['ConversationDetailResponse'];
export type Message = S['Message'];
export type StreamEvent = S['StreamEvent'];
export type TurnContext = S['TurnContext'];
export type Settings = S['SettingsResponse'];
export type ConnectionDraft = S['ConnectionDraft'];
export type Model = S['Model'];
export type TestResponse = S['TestResponse'];
export type UploadResponse = S['UploadResponse'];

export const api = createClient<paths>({ baseUrl: '' });

/** Error carrying the server's `{"error": "..."}` message. */
export class ApiError extends Error {
    status: number;
    constructor(message: string, status: number) {
        super(message);
        this.status = status;
    }
}

/**
 * Unwraps an openapi-fetch result: returns `data`, or throws ApiError with
 * the server's message.
 */
export async function unwrap<T>(p: Promise<{ data?: T; error?: unknown; response: Response }>): Promise<T> {
    const { data, error, response } = await p;
    if (!response.ok || data === undefined) {
        const msg = (error as { error?: string } | undefined)?.error ?? `${response.status} ${response.statusText}`;
        throw new ApiError(msg, response.status);
    }
    return data;
}
