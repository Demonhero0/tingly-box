import { useEffect, useMemo, useState } from 'react';
import { Box, Button, Card, CardActionArea, IconButton, InputAdornment, MenuItem, Select, Skeleton, Tab, Tabs, TextField, Typography } from '@mui/material';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { api, unwrap, type ProjectSummary } from '@/client';
import { Add, Folder, MoreHoriz, Search, Star } from '@/components/icons';
import { ProjectActionsMenu } from '@/components/ProjectActionsMenu';
import { ProjectFormDialog } from '@/components/ProjectFormDialog';
import { useNotify } from '@/hooks/useNotify';
import { useAppData } from '@/contexts/AppDataContext';
import { formatTokens, timeAgo } from '@/utils/format';
import { readLocal, writeLocal } from '@/utils/storage';

type Sort = 'activity' | 'name' | 'created';

function ProjectCard({ p, onChanged }: { p: ProjectSummary; onChanged: () => void }) {
    const { t, i18n } = useTranslation();
    const navigate = useNavigate();
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);
    return (
        <Card variant="outlined" sx={{ position: 'relative', borderRadius: 3, '&:hover': { borderColor: 'text.disabled' }, '& .card-menu': { opacity: anchor ? 1 : 0 }, '&:hover .card-menu': { opacity: 1 } }}>
            <CardActionArea onClick={() => navigate(`/project/${p.id}`)} sx={{ p: 2.25, height: '100%', display: 'flex', flexDirection: 'column', alignItems: 'stretch', justifyContent: 'flex-start' }}>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, pr: 4 }}>
                    <Typography variant="h5" noWrap sx={{ color: 'text.primary' }}>{p.name}</Typography>
                    {p.starred && <Star sx={{ fontSize: 16 }} color="primary" />}
                </Box>
                <Typography
                    variant="body2"
                    color="text.secondary"
                    sx={{ mt: 0.75, minHeight: 40, display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical', overflow: 'hidden' }}
                >
                    {p.description || ' '}
                </Typography>
                <Typography variant="caption" color="text.secondary" sx={{ mt: 1.5 }}>
                    {t('projects.cardMeta', {
                        time: timeAgo(p.last_activity_at, i18n.language),
                        chats: p.conversation_count,
                        files: p.knowledge_count,
                        tokens: formatTokens(p.knowledge_tokens),
                    })}
                </Typography>
            </CardActionArea>
            <IconButton className="card-menu" size="small" onClick={(e) => setAnchor(e.currentTarget)} sx={{ position: 'absolute', top: 12, right: 12 }} aria-label={t('common.more')}>
                <MoreHoriz fontSize="small" />
            </IconButton>
            <ProjectActionsMenu project={p} anchorEl={anchor} onClose={() => setAnchor(null)} onChanged={onChanged} onDeleted={onChanged} />
        </Card>
    );
}

export default function ProjectsPage() {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const notify = useNotify();
    const { refresh } = useAppData();
    const [tab, setTab] = useState<'active' | 'archived'>('active');
    const [q, setQ] = useState('');
    const [sort, setSort] = useState<Sort>(() => (readLocal('projectSort') as Sort) || 'activity');
    const [projects, setProjects] = useState<ProjectSummary[] | null>(null);
    const [creating, setCreating] = useState(false);
    const [tick, setTick] = useState(0);

    useEffect(() => {
        let alive = true;
        const timer = window.setTimeout(() => {
            unwrap(api.GET('/api/v1/projects', { params: { query: { q: q || undefined, archived: tab === 'archived' || undefined } } }))
                .then((r) => alive && setProjects(r.projects))
                .catch((e: Error) => alive && notify(e.message, 'error'));
        }, q ? 200 : 0);
        return () => {
            alive = false;
            window.clearTimeout(timer);
        };
    }, [q, tab, tick, notify]);

    const sorted = useMemo(() => {
        if (!projects) return null;
        const list = [...projects];
        if (sort === 'name') list.sort((a, b) => a.name.localeCompare(b.name));
        else if (sort === 'created') list.sort((a, b) => b.created_at.localeCompare(a.created_at));
        // 'activity' keeps the server order: starred first, then recent activity.
        return list;
    }, [projects, sort]);

    return (
        <Box sx={{ flex: 1, overflowY: 'auto' }}>
            <Box sx={{ maxWidth: 1040, mx: 'auto', px: { xs: 2, md: 4 }, pt: { xs: 7, md: 5 }, pb: 6 }}>
                <Box sx={{ display: 'flex', alignItems: 'center', mb: 3 }}>
                    <Typography variant="h1" sx={{ flex: 1, fontFamily: 'Georgia, "Songti SC", serif', fontWeight: 400 }}>{t('projects.title')}</Typography>
                    <Button variant="contained" startIcon={<Add />} onClick={() => setCreating(true)}>{t('projects.new')}</Button>
                </Box>

                <TextField
                    fullWidth
                    placeholder={t('projects.search')}
                    value={q}
                    onChange={(e) => setQ(e.target.value)}
                    slotProps={{ input: { startAdornment: <InputAdornment position="start"><Search fontSize="small" /></InputAdornment> } }}
                    sx={{ mb: 2, '& .MuiOutlinedInput-root': { bgcolor: 'background.paper' } }}
                />

                <Box sx={{ display: 'flex', alignItems: 'center', borderBottom: 1, borderColor: 'divider', mb: 3 }}>
                    <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ flex: 1, minHeight: 40, '& .MuiTab-root': { minHeight: 40 } }}>
                        <Tab value="active" label={t('projects.tabActive')} />
                        <Tab value="archived" label={t('projects.tabArchived')} />
                    </Tabs>
                    <Typography variant="body2" color="text.secondary" sx={{ mr: 1 }}>{t('projects.sortBy')}</Typography>
                    <Select
                        size="small"
                        variant="standard"
                        disableUnderline
                        value={sort}
                        onChange={(e) => {
                            setSort(e.target.value as Sort);
                            writeLocal('projectSort', e.target.value);
                        }}
                        sx={{ fontSize: 14 }}
                    >
                        <MenuItem value="activity">{t('projects.sortActivity')}</MenuItem>
                        <MenuItem value="name">{t('projects.sortName')}</MenuItem>
                        <MenuItem value="created">{t('projects.sortCreated')}</MenuItem>
                    </Select>
                </Box>

                {sorted === null ? (
                    <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: '1fr 1fr', lg: '1fr 1fr 1fr' }, gap: 2 }}>
                        {[0, 1, 2].map((i) => <Skeleton key={i} variant="rounded" height={140} />)}
                    </Box>
                ) : sorted.length === 0 ? (
                    <Box sx={{ textAlign: 'center', py: 8, color: 'text.secondary' }}>
                        <Folder sx={{ fontSize: 40, color: 'text.disabled' }} />
                        <Typography variant="h4" sx={{ mt: 1.5, color: 'text.primary' }}>
                            {q ? t('projects.noneFound') : tab === 'archived' ? t('projects.noArchived') : t('projects.emptyTitle')}
                        </Typography>
                        {!q && tab === 'active' && (
                            <>
                                <Typography variant="body1" sx={{ mt: 1, maxWidth: 520, mx: 'auto' }}>{t('projects.emptyBody')}</Typography>
                                <Button variant="contained" startIcon={<Add />} sx={{ mt: 3 }} onClick={() => setCreating(true)}>{t('projects.new')}</Button>
                            </>
                        )}
                    </Box>
                ) : (
                    <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', sm: '1fr 1fr', lg: '1fr 1fr 1fr' }, gap: 2 }}>
                        {sorted.map((p) => (
                            <ProjectCard key={p.id} p={p} onChanged={() => setTick((x) => x + 1)} />
                        ))}
                    </Box>
                )}
            </Box>

            <ProjectFormDialog
                open={creating}
                mode="create"
                onClose={() => setCreating(false)}
                onSubmit={async (v) => {
                    try {
                        const p = await unwrap(api.POST('/api/v1/projects', { body: v }));
                        refresh();
                        navigate(`/project/${p.id}`);
                    } catch (e) {
                        notify((e as Error).message, 'error');
                        throw e;
                    }
                }}
            />
        </Box>
    );
}
