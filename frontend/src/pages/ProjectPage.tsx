import { useCallback, useEffect, useState } from 'react';
import { Alert, Box, Button, Chip, IconButton, Link, Skeleton, Tooltip, Typography } from '@mui/material';
import { Link as RouterLink, useNavigate, useParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { api, unwrap, type ConversationSummary, type ProjectDetail } from '@/client';
import { ChatActionsMenu } from '@/components/ChatActionsMenu';
import { Composer } from '@/components/Composer';
import { ConnectionHint } from '@/components/ConnectionHint';
import { ContextPreviewDialog } from '@/components/ContextPreviewDialog';
import { ChatBubble, ChevronRight, MemoryOff, MoreHoriz, Psychology, Star, StarBorder } from '@/components/icons';
import { KnowledgePanel } from '@/components/KnowledgePanel';
import { ProjectActionsMenu } from '@/components/ProjectActionsMenu';
import { useNotify } from '@/hooks/useNotify';
import { useChosenModel, useStartChat } from '@/hooks/useStartChat';
import { useAppData } from '@/contexts/AppDataContext';
import { timeAgo } from '@/utils/format';

function ChatRow({ chat, onChanged }: { chat: ConversationSummary; onChanged: () => void }) {
    const { t, i18n } = useTranslation();
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);
    return (
        <Box
            component={RouterLink}
            to={`/chat/${chat.id}`}
            sx={{ display: 'flex', alignItems: 'center', gap: 1.5, px: 2, py: 1.5, textDecoration: 'none', color: 'inherit', borderTop: 1, borderColor: 'divider', '&:hover': { bgcolor: 'action.hover' }, '& .row-menu': { opacity: anchor ? 1 : 0 }, '&:hover .row-menu': { opacity: 1 } }}
        >
            <Box sx={{ flex: 1, minWidth: 0 }}>
                <Typography noWrap sx={{ color: 'text.primary', fontWeight: 500, fontSize: 14.5 }}>
                    {chat.title || t('chat.untitled')}
                    {chat.starred && <Star sx={{ fontSize: 14, ml: 0.75, verticalAlign: '-2px' }} color="primary" />}
                </Typography>
                <Typography variant="caption" color="text.secondary">
                    {t('project.lastMessage', { time: timeAgo(chat.updated_at, i18n.language) })}
                    {chat.memory_disabled && ` · ${t('memory.offInChat')}`}
                </Typography>
            </Box>
            <IconButton
                className="row-menu"
                size="small"
                aria-label={t('common.more')}
                onClick={(e) => {
                    e.preventDefault();
                    e.stopPropagation();
                    setAnchor(e.currentTarget);
                }}
            >
                <MoreHoriz fontSize="small" />
            </IconButton>
            <ChatActionsMenu chat={chat} anchorEl={anchor} onClose={() => setAnchor(null)} onChanged={onChanged} onDeleted={onChanged} />
        </Box>
    );
}

export default function ProjectPage() {
    const { id = '' } = useParams();
    const { t } = useTranslation();
    const navigate = useNavigate();
    const notify = useNotify();
    const { refresh } = useAppData();
    const start = useStartChat();
    const { model, setModel } = useChosenModel();
    const [detail, setDetail] = useState<ProjectDetail | null>(null);
    const [chats, setChats] = useState<ConversationSummary[] | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [menuAnchor, setMenuAnchor] = useState<HTMLElement | null>(null);
    const [memoryOff, setMemoryOff] = useState(false);
    const [contextOpen, setContextOpen] = useState(false);

    const load = useCallback(() => {
        unwrap(api.GET('/api/v1/projects/{id}', { params: { path: { id } } }))
            .then((d) => {
                setDetail(d);
                setError(null);
            })
            .catch((e: Error) => setError(e.message));
        unwrap(api.GET('/api/v1/conversations', { params: { query: { project_id: id } } }))
            .then((r) => setChats(r.conversations))
            .catch(() => setChats([]));
    }, [id]);

    useEffect(() => {
        setDetail(null);
        setChats(null);
        load();
    }, [load]);

    if (error) {
        return (
            <Box sx={{ maxWidth: 720, mx: 'auto', pt: 8, px: 2 }}>
                <Alert severity="error" action={<Button color="inherit" component={RouterLink} to="/projects">{t('projects.all')}</Button>}>{error}</Alert>
            </Box>
        );
    }

    const p = detail?.project;
    const archived = !!p?.archived_at;
    const toggleStar = () => {
        if (!p) return;
        unwrap(api.PATCH('/api/v1/projects/{id}', { params: { path: { id: p.id } }, body: { starred: !p.starred } }))
            .then(() => {
                load();
                refresh();
            })
            .catch((e: Error) => notify(e.message, 'error'));
    };

    return (
        <Box sx={{ flex: 1, overflowY: 'auto' }}>
            <Box
                sx={{
                    maxWidth: 1180,
                    mx: 'auto',
                    px: { xs: 2, md: 4 },
                    pt: { xs: 7, md: 4 },
                    pb: 6,
                    display: 'grid',
                    gridTemplateColumns: { xs: '1fr', lg: 'minmax(0, 1fr) 360px' },
                    gap: { xs: 3, lg: 5 },
                    alignItems: 'start',
                }}
            >
                {/* Main column */}
                <Box sx={{ minWidth: 0 }}>
                    <Link component={RouterLink} to="/projects" underline="hover" color="text.secondary" sx={{ fontSize: 13.5, display: 'inline-flex', alignItems: 'center', gap: 0.25 }}>
                        {t('projects.all')}
                        <ChevronRight sx={{ fontSize: 15 }} />
                    </Link>
                    {p ? (
                        <>
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mt: 1 }}>
                                <Typography variant="h1" sx={{ flex: 1, minWidth: 0, fontFamily: 'Georgia, "Songti SC", serif', fontWeight: 400, wordBreak: 'break-word' }}>{p.name}</Typography>
                                <Tooltip title={p.starred ? t('common.unstar') : t('common.star')}>
                                    <IconButton onClick={toggleStar} aria-label={p.starred ? t('common.unstar') : t('common.star')}>
                                        {p.starred ? <Star color="primary" /> : <StarBorder />}
                                    </IconButton>
                                </Tooltip>
                                <IconButton onClick={(e) => setMenuAnchor(e.currentTarget)} aria-label={t('common.more')}><MoreHoriz /></IconButton>
                            </Box>
                            {p.description && <Typography color="text.secondary" sx={{ mt: 0.5, whiteSpace: 'pre-wrap' }}>{p.description}</Typography>}
                            <ProjectActionsMenu
                                project={p}
                                anchorEl={menuAnchor}
                                onClose={() => setMenuAnchor(null)}
                                onChanged={() => load()}
                                onDeleted={() => navigate('/projects')}
                            />
                        </>
                    ) : (
                        <Skeleton variant="text" width="50%" height={48} />
                    )}

                    {archived && (
                        <Alert
                            severity="info"
                            sx={{ mt: 2 }}
                            action={
                                <Button
                                    color="inherit"
                                    size="small"
                                    onClick={() =>
                                        unwrap(api.PATCH('/api/v1/projects/{id}', { params: { path: { id } }, body: { archived: false } }))
                                            .then(() => {
                                                load();
                                                refresh();
                                            })
                                            .catch((e: Error) => notify(e.message, 'error'))
                                    }
                                >
                                    {t('project.unarchive')}
                                </Button>
                            }
                        >
                            {t('project.archivedBanner')}
                        </Alert>
                    )}

                    <Box sx={{ mt: 3 }}>
                        <ConnectionHint />
                        <Composer
                            disabled={!p || archived}
                            model={model}
                            onModelChange={setModel}
                            placeholder={t('project.composerPlaceholder')}
                            onSend={(text) => start({ text, model, projectId: id, memoryDisabled: memoryOff })}
                            leftSlot={
                                <Tooltip title={memoryOff ? t('memory.offHint') : t('memory.onHint')}>
                                    <Chip
                                        size="small"
                                        variant={memoryOff ? 'filled' : 'outlined'}
                                        icon={memoryOff ? <MemoryOff sx={{ fontSize: 16 }} /> : <Psychology sx={{ fontSize: 16 }} />}
                                        label={memoryOff ? t('memory.chipOff') : t('memory.chipOn')}
                                        onClick={() => setMemoryOff((v) => !v)}
                                    />
                                </Tooltip>
                            }
                        />
                    </Box>

                    <Box sx={{ mt: 4 }}>
                        <Typography variant="h5" sx={{ mb: 1 }}>{t('project.chatsTitle')}</Typography>
                        {chats === null ? (
                            [0, 1, 2].map((i) => <Skeleton key={i} height={56} />)
                        ) : chats.length === 0 ? (
                            <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 3, p: 3, textAlign: 'center', color: 'text.secondary' }}>
                                <ChatBubble sx={{ fontSize: 28, color: 'text.disabled' }} />
                                <Typography variant="body2" sx={{ mt: 1 }}>{t('project.noChats')}</Typography>
                            </Box>
                        ) : (
                            <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 3, overflow: 'hidden', '& > a:first-of-type': { borderTop: 0 } }}>
                                {chats.map((c) => (
                                    <ChatRow key={c.id} chat={c} onChanged={load} />
                                ))}
                            </Box>
                        )}
                    </Box>
                </Box>

                {/* Project knowledge panel */}
                <Box sx={{ position: { lg: 'sticky' }, top: { lg: 24 } }}>
                    {detail ? (
                        <KnowledgePanel
                            projectId={id}
                            instructions={detail.project.instructions}
                            memory={detail.project.memory}
                            memoryUpdatedAt={detail.project.memory_updated_at}
                            files={detail.knowledge}
                            capacity={detail.capacity}
                            readOnly={archived}
                            onChanged={load}
                            onViewContext={() => setContextOpen(true)}
                        />
                    ) : (
                        <Skeleton variant="rounded" height={420} />
                    )}
                </Box>
            </Box>
            <ContextPreviewDialog projectId={id} open={contextOpen} onClose={() => setContextOpen(false)} />
        </Box>
    );
}
