import { useEffect, useState } from 'react';
import { Alert, Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, TextField, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { api, unwrap, type ContextResponse } from '@/client';
import { formatTokens } from '@/utils/format';

/**
 * Shows the exact system prompt a project's chats are sent — the real value,
 * not a description of it. In retrieval mode a sample question shows which
 * excerpts would be picked.
 */
export function ContextPreviewDialog({ projectId, open, onClose }: { projectId: string; open: boolean; onClose: () => void }) {
    const { t } = useTranslation();
    const [ctx, setCtx] = useState<ContextResponse | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [q, setQ] = useState('');
    const [query, setQuery] = useState('');

    useEffect(() => {
        if (!open) return;
        setError(null);
        unwrap(api.GET('/api/v1/projects/{id}/context', { params: { path: { id: projectId }, query: { q: query || undefined } } }))
            .then(setCtx)
            .catch((e: Error) => setError(e.message));
    }, [open, projectId, query]);

    return (
        <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
            <DialogTitle>{t('context.title')}</DialogTitle>
            <DialogContent>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>{t('context.intro')}</Typography>
                {error && <Alert severity="error">{error}</Alert>}
                {ctx?.mode === 'retrieval' && (
                    <Box component="form" onSubmit={(e) => { e.preventDefault(); setQuery(q); }} sx={{ display: 'flex', gap: 1, mb: 1.5 }}>
                        <TextField size="small" fullWidth value={q} onChange={(e) => setQ(e.target.value)} placeholder={t('context.samplePlaceholder')} />
                        <Button type="submit" variant="outlined">{t('context.try')}</Button>
                    </Box>
                )}
                {ctx && (
                    <>
                        <Typography variant="caption" color="text.secondary">
                            {t('context.stats', { tokens: formatTokens(ctx.tokens), mode: ctx.mode === 'retrieval' ? t('knowledge.modeRetrieval') : t('knowledge.modeInline') })}
                            {ctx.mode === 'retrieval' && ` · ${t('context.excerpts', { count: ctx.excerpts })}`}
                        </Typography>
                        <Box
                            component="pre"
                            sx={{ mt: 1, p: 1.5, maxHeight: '55vh', overflow: 'auto', bgcolor: 'action.hover', borderRadius: 2, fontSize: 12.5, whiteSpace: 'pre-wrap', wordBreak: 'break-word', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' }}
                        >
                            {ctx.system_prompt || t('context.empty')}
                        </Box>
                    </>
                )}
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2 }}>
                <Button onClick={onClose}>{t('common.close')}</Button>
            </DialogActions>
        </Dialog>
    );
}
