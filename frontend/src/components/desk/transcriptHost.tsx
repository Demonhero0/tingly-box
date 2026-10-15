import CodeBlock from '@/components/CodeBlock';
import type {CodeBlockComponent, DraftStorage} from '@tingly/projects/transcript';
import {deskStore} from './useDeskStorage';

// What Desk hands the shared transcript: request-card drafts in the tab's
// sessionStorage, under the keys Desk always used (desk.requestDraft:<id>), so
// they survive a reload; and the app's highlighted code block for replies.

const keep = (value: unknown) => value;

export const deskDrafts: DraftStorage = {
    read: (key, decode) => deskStore(`desk.${key}`, decode).snapshot(),
    write: (key, value) => deskStore(`desk.${key}`, keep).update(() => value),
    subscribe: (key, listener) => deskStore(`desk.${key}`, keep).subscribe(listener),
};

export const DeskCodeBlock: CodeBlockComponent = ({code, language}) => (
    <CodeBlock code={code} language={language} maxHeight={480} sx={{margin: '12px 0'}}/>
);
