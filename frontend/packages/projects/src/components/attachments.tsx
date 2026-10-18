import { useEffect, useState } from 'react';
import { Box, ButtonBase, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { apiFetch, type Attachment } from '../client';
import { Description } from './icons';

/** A paste longer than this becomes a file instead of filling the box. */
export const PASTE_AS_FILE_CHARS = 4000;
export const PASTE_AS_FILE_LINES = 40;

export const lineCount = (text: string): number => text.split('\n').length;

export const isLongText = (text: string): boolean => text.length > PASTE_AS_FILE_CHARS || lineCount(text) > PASTE_AS_FILE_LINES;

/** The text file a long paste is stored as. */
export const pastedTextFile = (text: string): File => new File([text], 'pasted-text.txt', { type: 'text/plain' });

export const formatSize = (n: number): string => (n >= 1 << 20 ? `${(n / (1 << 20)).toFixed(1)} MB` : n >= 1 << 10 ? `${Math.round(n / (1 << 10))} KB` : `${n} B`);

// A project keeps its uploads in its library; a working-folder session has no
// project and shares one upload pool (an empty projectId).
const fileUrl = (projectId: string, path: string) => (projectId
    ? `/api/v1/projects/${projectId}/library/file?path=${encodeURIComponent(path)}`
    : `/api/v1/projects/workdirs/attachments/file?path=${encodeURIComponent(path)}`);

/** Stores files for a message (the project's uploads, or the session pool for an empty projectId); returns what to send with it. */
export async function uploadAttachments(projectId: string, files: File[]): Promise<Attachment[]> {
    const form = new FormData();
    files.forEach((f) => form.append('files', f, f.name));
    const res = await apiFetch(projectId ? `/api/v1/projects/${projectId}/attachments` : '/api/v1/projects/workdirs/attachments', { method: 'POST', body: form });
    const body = (await res.json().catch(() => ({}))) as { error?: string; attachments?: Attachment[] };
    if (!res.ok) throw new Error(body.error ?? res.statusText);
    return body.attachments ?? [];
}

const isImage = (a: { type: string }) => a.type.startsWith('image/');

/** Loads a stored file through the host's fetch (so its auth applies); null once it is gone. */
function useStoredFile(projectId: string, path: string, enabled: boolean): string | null | undefined {
    const [url, setUrl] = useState<string | null | undefined>(undefined);
    useEffect(() => {
        if (!enabled) return;
        let made: string | undefined;
        let live = true;
        apiFetch(fileUrl(projectId, path))
            .then((r) => (r.ok ? r.blob() : Promise.reject(new Error(r.statusText))))
            .then((b) => { if (live) { made = URL.createObjectURL(b); setUrl(made); } })
            .catch(() => { if (live) setUrl(null); });
        return () => { live = false; if (made) URL.revokeObjectURL(made); };
    }, [projectId, path, enabled]);
    return url;
}

function AttachmentChip({ projectId, item }: { projectId: string; item: Attachment }) {
    const { t } = useTranslation('projects');
    const image = isImage(item);
    const src = useStoredFile(projectId, item.path, image);
    const open = async () => {
        const res = await apiFetch(fileUrl(projectId, item.path)).catch(() => null);
        if (!res?.ok) return;
        window.open(URL.createObjectURL(await res.blob()), '_blank', 'noopener');
    };
    if (image && src) {
        return (
            <ButtonBase onClick={() => void open()} aria-label={item.name} sx={{ borderRadius: 1, overflow: 'hidden', border: 1, borderColor: 'divider' }}>
                <Box component="img" src={src} alt={item.name} sx={{ display: 'block', maxHeight: 120, maxWidth: 200, objectFit: 'cover' }} />
            </ButtonBase>
        );
    }
    const gone = src === null;
    return (
        <ButtonBase
            onClick={() => void open()}
            disabled={gone}
            sx={{ display: 'flex', alignItems: 'center', gap: 0.75, px: 1, py: 0.5, borderRadius: 1, border: 1, borderColor: 'divider', maxWidth: 240, textAlign: 'left' }}
        >
            <Description fontSize="small" sx={{ color: 'text.secondary', flexShrink: 0 }} />
            <Typography variant="caption" noWrap sx={{ minWidth: 0 }}>{gone ? t('attachments.removed', { name: item.name }) : item.name}</Typography>
            {!gone && <Typography variant="caption" color="text.disabled" sx={{ flexShrink: 0 }}>{formatSize(item.size)}</Typography>}
        </ButtonBase>
    );
}

/** The files a sent message carries: thumbnails for images, chips for the rest. */
export function AttachmentChips({ projectId, items, align = 'flex-end' }: { projectId: string; items: Attachment[]; align?: 'flex-start' | 'flex-end' }) {
    if (items.length === 0) return null;
    return (
        <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.75, justifyContent: align, mb: 0.75 }}>
            {items.map((a) => <AttachmentChip key={a.path} projectId={projectId} item={a} />)}
        </Box>
    );
}
