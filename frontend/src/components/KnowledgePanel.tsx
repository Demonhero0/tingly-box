import { useRef, useState, type DragEvent, type ReactNode } from 'react';
import { Alert, Box, Button, Chip, IconButton, LinearProgress, ListItemIcon, ListItemText, Menu, MenuItem, Paper, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { api, unwrap, type Capacity, type KnowledgeFileSummary, type UploadResponse } from '@/client';
import { Add, Delete, Description, Edit, Info, MoreHoriz, PostAdd, Psychology, Refresh, UploadFile } from '@/components/icons';
import { TextEditDialog } from './TextEditDialog';
import { ConfirmDialog } from './ConfirmDialog';
import { useNotify } from '@/hooks/useNotify';
import { formatBytes, formatTokens, timeAgo } from '@/utils/format';

function Section({ title, action, children }: { title: string; action?: ReactNode; children: ReactNode }) {
    return (
        <Box sx={{ py: 2, '& + &': { borderTop: 1, borderColor: 'divider' } }}>
            <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
                <Typography variant="h6" sx={{ flex: 1 }}>{title}</Typography>
                {action}
            </Box>
            {children}
        </Box>
    );
}

function Preview({ text, empty, onClick }: { text: string; empty: string; onClick: () => void }) {
    return (
        <Box
            role="button"
            tabIndex={0}
            onClick={onClick}
            onKeyDown={(e) => e.key === 'Enter' && onClick()}
            sx={{ p: 1.25, borderRadius: 2, border: 1, borderColor: 'divider', '&:hover': { bgcolor: 'action.hover' }, outline: 'none' }}
        >
            <Typography
                variant="body2"
                sx={{ color: text ? 'text.primary' : 'text.secondary', whiteSpace: 'pre-wrap', display: '-webkit-box', WebkitLineClamp: 4, WebkitBoxOrient: 'vertical', overflow: 'hidden' }}
            >
                {text || empty}
            </Typography>
        </Box>
    );
}

/** Concrete numbers, not just a percentage; retrieval mode is labelled, not toggled. */
function CapacityBar({ capacity }: { capacity: Capacity }) {
    const { t } = useTranslation();
    const retrieval = capacity.mode === 'retrieval';
    const limit = retrieval ? capacity.max_tokens : capacity.inline_budget;
    const pct = Math.min(100, (capacity.knowledge_tokens / limit) * 100);
    return (
        <Box sx={{ mb: 1.5 }}>
            <LinearProgress variant="determinate" value={pct} color={retrieval ? 'info' : pct > 85 ? 'warning' : 'primary'} sx={{ height: 6, borderRadius: 3, bgcolor: 'action.hover' }} />
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mt: 0.75 }}>
                <Typography variant="caption" color="text.secondary" sx={{ flex: 1 }}>
                    {t('knowledge.capacity', { used: formatTokens(capacity.knowledge_tokens), limit: formatTokens(limit) })}
                </Typography>
                <Tooltip title={retrieval ? t('knowledge.retrievalHint', { budget: formatTokens(capacity.inline_budget) }) : t('knowledge.inlineHint', { budget: formatTokens(capacity.inline_budget) })}>
                    <Chip
                        size="small"
                        variant="outlined"
                        color={retrieval ? 'info' : 'default'}
                        icon={<Info sx={{ fontSize: 14 }} />}
                        label={retrieval ? t('knowledge.modeRetrieval') : t('knowledge.modeInline')}
                        sx={{ height: 22, fontSize: 11.5 }}
                    />
                </Tooltip>
            </Box>
        </Box>
    );
}

interface Props {
    projectId: string;
    instructions: string;
    memory: string;
    memoryUpdatedAt?: string;
    files: KnowledgeFileSummary[];
    capacity: Capacity;
    readOnly: boolean;
    onChanged: () => void;
    onViewContext: () => void;
}

export function KnowledgePanel({ projectId, instructions, memory, memoryUpdatedAt, files, capacity, readOnly, onChanged, onViewContext }: Props) {
    const { t, i18n } = useTranslation();
    const notify = useNotify();
    const fileInput = useRef<HTMLInputElement>(null);
    const [addAnchor, setAddAnchor] = useState<HTMLElement | null>(null);
    const [memAnchor, setMemAnchor] = useState<HTMLElement | null>(null);
    const [editInstructions, setEditInstructions] = useState(false);
    const [editMemory, setEditMemory] = useState(false);
    const [addingText, setAddingText] = useState(false);
    const [openFile, setOpenFile] = useState<{ id: string; name: string; content: string } | null>(null);
    const [deleting, setDeleting] = useState<KnowledgeFileSummary | null>(null);
    const [uploading, setUploading] = useState(false);
    const [refreshingMemory, setRefreshingMemory] = useState(false);
    const [dragOver, setDragOver] = useState(false);

    const patchProject = async (body: { instructions?: string; memory?: string }) => {
        try {
            await unwrap(api.PATCH('/api/v1/projects/{id}', { params: { path: { id: projectId } }, body }));
            onChanged();
        } catch (e) {
            notify((e as Error).message, 'error');
            throw e;
        }
    };

    const upload = async (list: FileList | File[]) => {
        const arr = Array.from(list);
        if (arr.length === 0) return;
        const form = new FormData();
        arr.forEach((f) => form.append('files', f));
        setUploading(true);
        try {
            const res = await fetch(`/api/v1/projects/${projectId}/knowledge`, { method: 'POST', body: form });
            const body = (await res.json()) as UploadResponse & { error?: string };
            if (!res.ok) throw new Error(body.error ?? res.statusText);
            if (body.files.length) notify(t('knowledge.uploaded', { count: body.files.length }), 'success');
            if (body.rejected.length) {
                notify(body.rejected.map((r) => `${r.name}: ${r.reason}`).join('\n'), 'warning');
            }
            onChanged();
        } catch (e) {
            notify((e as Error).message, 'error');
        } finally {
            setUploading(false);
            if (fileInput.current) fileInput.current.value = '';
        }
    };

    const onDrop = (e: DragEvent) => {
        e.preventDefault();
        setDragOver(false);
        if (!readOnly && e.dataTransfer.files.length) void upload(e.dataTransfer.files);
    };

    const openKnowledge = async (f: KnowledgeFileSummary) => {
        try {
            const full = await unwrap(api.GET('/api/v1/knowledge/{id}', { params: { path: { id: f.id } } }));
            setOpenFile({ id: full.id, name: full.name, content: full.content });
        } catch (e) {
            notify((e as Error).message, 'error');
        }
    };

    return (
        <Paper
            variant="outlined"
            onDragOver={(e) => {
                e.preventDefault();
                if (!readOnly) setDragOver(true);
            }}
            onDragLeave={() => setDragOver(false)}
            onDrop={onDrop}
            sx={{ borderRadius: 3, px: 2.5, py: 0.5, position: 'relative', borderColor: dragOver ? 'primary.main' : 'divider', borderStyle: dragOver ? 'dashed' : 'solid' }}
        >
            <Section
                title={t('instructions.title')}
                action={!readOnly && (
                    <Tooltip title={t('common.edit')}>
                        <IconButton size="small" onClick={() => setEditInstructions(true)} aria-label={t('common.edit')}><Edit fontSize="small" /></IconButton>
                    </Tooltip>
                )}
            >
                <Preview text={instructions} empty={t('instructions.empty')} onClick={() => !readOnly && setEditInstructions(true)} />
            </Section>

            <Section
                title={t('memory.title')}
                action={!readOnly && (
                    <IconButton size="small" onClick={(e) => setMemAnchor(e.currentTarget)} aria-label={t('common.more')}><MoreHoriz fontSize="small" /></IconButton>
                )}
            >
                <Preview text={memory} empty={t('memory.empty')} onClick={() => !readOnly && setEditMemory(true)} />
                <Typography variant="caption" color="text.secondary" sx={{ display: 'flex', alignItems: 'center', gap: 0.5, mt: 0.75 }}>
                    <Psychology sx={{ fontSize: 14 }} />
                    {refreshingMemory ? t('memory.updating') : memoryUpdatedAt ? t('memory.updatedAt', { time: timeAgo(memoryUpdatedAt, i18n.language) }) : t('memory.auto')}
                </Typography>
                <Menu anchorEl={memAnchor} open={!!memAnchor} onClose={() => setMemAnchor(null)}>
                    <MenuItem onClick={() => { setMemAnchor(null); setEditMemory(true); }}>
                        <ListItemIcon><Edit fontSize="small" /></ListItemIcon>
                        <ListItemText>{t('memory.edit')}</ListItemText>
                    </MenuItem>
                    <MenuItem
                        disabled={refreshingMemory}
                        onClick={() => {
                            setMemAnchor(null);
                            setRefreshingMemory(true);
                            unwrap(api.POST('/api/v1/projects/{id}/memory/refresh', { params: { path: { id: projectId } } }))
                                .then(() => {
                                    notify(t('memory.updated'), 'success');
                                    onChanged();
                                })
                                .catch((e: Error) => notify(e.message, 'error'))
                                .finally(() => setRefreshingMemory(false));
                        }}
                    >
                        <ListItemIcon><Refresh fontSize="small" /></ListItemIcon>
                        <ListItemText>{t('memory.updateNow')}</ListItemText>
                    </MenuItem>
                </Menu>
            </Section>

            <Section
                title={t('knowledge.title')}
                action={!readOnly && (
                    <Tooltip title={t('knowledge.add')}>
                        <IconButton size="small" onClick={(e) => setAddAnchor(e.currentTarget)} aria-label={t('knowledge.add')}><Add fontSize="small" /></IconButton>
                    </Tooltip>
                )}
            >
                <CapacityBar capacity={capacity} />
                {uploading && <LinearProgress sx={{ mb: 1 }} />}
                {files.length === 0 ? (
                    <Box
                        role="button"
                        tabIndex={0}
                        onClick={() => !readOnly && fileInput.current?.click()}
                        sx={{ border: 1, borderStyle: 'dashed', borderColor: 'divider', borderRadius: 2, p: 2.5, textAlign: 'center', color: 'text.secondary', '&:hover': { bgcolor: readOnly ? undefined : 'action.hover' } }}
                    >
                        <UploadFile sx={{ fontSize: 26, color: 'text.disabled' }} />
                        <Typography variant="body2" sx={{ mt: 0.5 }}>{t('knowledge.empty')}</Typography>
                    </Box>
                ) : (
                    <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.75 }}>
                        {files.map((f) => (
                            <Box
                                key={f.id}
                                role="button"
                                tabIndex={0}
                                onClick={() => void openKnowledge(f)}
                                onKeyDown={(e) => e.key === 'Enter' && void openKnowledge(f)}
                                sx={{ display: 'flex', alignItems: 'center', gap: 1.25, p: 1, borderRadius: 2, border: 1, borderColor: 'divider', '&:hover': { bgcolor: 'action.hover' }, '& .del': { opacity: 0 }, '&:hover .del': { opacity: 1 } }}
                            >
                                <Description fontSize="small" sx={{ color: 'text.secondary' }} />
                                <Box sx={{ flex: 1, minWidth: 0 }}>
                                    <Typography variant="body2" noWrap sx={{ color: 'text.primary', fontWeight: 500 }}>{f.name}</Typography>
                                    <Typography variant="caption" color="text.secondary">
                                        {t('knowledge.fileMeta', { tokens: formatTokens(f.tokens), size: formatBytes(f.size) })}
                                    </Typography>
                                </Box>
                                {!readOnly && (
                                    <IconButton
                                        className="del"
                                        size="small"
                                        aria-label={t('common.delete')}
                                        onClick={(e) => {
                                            e.stopPropagation();
                                            setDeleting(f);
                                        }}
                                    >
                                        <Delete fontSize="small" />
                                    </IconButton>
                                )}
                            </Box>
                        ))}
                    </Box>
                )}
                <Button size="small" color="inherit" onClick={onViewContext} sx={{ mt: 1.5, color: 'text.secondary' }}>
                    {t('context.view')}
                </Button>
            </Section>

            <input ref={fileInput} type="file" multiple hidden onChange={(e) => e.target.files && void upload(e.target.files)} />
            <Menu anchorEl={addAnchor} open={!!addAnchor} onClose={() => setAddAnchor(null)}>
                <MenuItem onClick={() => { setAddAnchor(null); fileInput.current?.click(); }}>
                    <ListItemIcon><UploadFile fontSize="small" /></ListItemIcon>
                    <ListItemText primary={t('knowledge.uploadFiles')} secondary={t('knowledge.uploadHint')} />
                </MenuItem>
                <MenuItem onClick={() => { setAddAnchor(null); setAddingText(true); }}>
                    <ListItemIcon><PostAdd fontSize="small" /></ListItemIcon>
                    <ListItemText>{t('knowledge.addText')}</ListItemText>
                </MenuItem>
            </Menu>

            {dragOver && (
                <Alert severity="info" sx={{ position: 'absolute', inset: 8, display: 'flex', alignItems: 'center', justifyContent: 'center', pointerEvents: 'none', opacity: 0.95 }}>
                    {t('knowledge.dropHere')}
                </Alert>
            )}

            <TextEditDialog
                open={editInstructions}
                title={t('instructions.dialogTitle')}
                intro={t('instructions.intro')}
                placeholder={t('instructions.placeholder')}
                initialText={instructions}
                onClose={() => setEditInstructions(false)}
                onSubmit={async ({ text }) => patchProject({ instructions: text })}
            />
            <TextEditDialog
                open={editMemory}
                title={t('memory.dialogTitle')}
                intro={t('memory.intro')}
                initialText={memory}
                onClose={() => setEditMemory(false)}
                onSubmit={async ({ text }) => patchProject({ memory: text })}
            />
            <TextEditDialog
                open={addingText}
                title={t('knowledge.addText')}
                nameField={{ label: t('knowledge.titleLabel'), initial: '' }}
                placeholder={t('knowledge.textPlaceholder')}
                initialText=""
                submitLabel={t('knowledge.addSubmit')}
                onClose={() => setAddingText(false)}
                onSubmit={async ({ text, name }) => {
                    try {
                        await unwrap(api.POST('/api/v1/projects/{id}/knowledge/text', { params: { path: { id: projectId } }, body: { name, content: text } }));
                        onChanged();
                    } catch (e) {
                        notify((e as Error).message, 'error');
                        throw e;
                    }
                }}
            />
            <TextEditDialog
                open={!!openFile}
                title={openFile?.name ?? ''}
                nameField={{ label: t('knowledge.titleLabel'), initial: openFile?.name ?? '' }}
                initialText={openFile?.content ?? ''}
                readOnly={readOnly}
                onClose={() => setOpenFile(null)}
                onSubmit={async ({ text, name }) => {
                    if (!openFile) return;
                    try {
                        await unwrap(api.PATCH('/api/v1/knowledge/{id}', { params: { path: { id: openFile.id } }, body: { name, content: text } }));
                        onChanged();
                    } catch (e) {
                        notify((e as Error).message, 'error');
                        throw e;
                    }
                }}
            />
            <ConfirmDialog
                open={!!deleting}
                title={t('knowledge.deleteTitle')}
                message={t('knowledge.deleteMessage', { name: deleting?.name ?? '' })}
                confirmLabel={t('common.delete')}
                danger
                onClose={() => setDeleting(null)}
                onConfirm={() => {
                    if (!deleting) return;
                    unwrap(api.DELETE('/api/v1/knowledge/{id}', { params: { path: { id: deleting.id } } }))
                        .then(onChanged)
                        .catch((e: Error) => notify(e.message, 'error'));
                }}
            />
        </Paper>
    );
}
