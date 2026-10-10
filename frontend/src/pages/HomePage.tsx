import { useEffect, useState } from 'react';
import { Box, Chip, ListItemIcon, ListItemText, Menu, MenuItem, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { api, unwrap, type ProjectSummary } from '@/client';
import { Composer } from '@/components/Composer';
import { ConnectionHint } from '@/components/ConnectionHint';
import { Close, Folder } from '@/components/icons';
import { useChosenModel, useStartChat } from '@/hooks/useStartChat';

function greetingKey(): string {
    const h = new Date().getHours();
    if (h < 5) return 'home.greetingNight';
    if (h < 12) return 'home.greetingMorning';
    if (h < 18) return 'home.greetingAfternoon';
    return 'home.greetingEvening';
}

export default function HomePage() {
    const { t } = useTranslation();
    const start = useStartChat();
    const { model, setModel } = useChosenModel();
    const [projects, setProjects] = useState<ProjectSummary[]>([]);
    const [project, setProject] = useState<ProjectSummary | null>(null);
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);

    useEffect(() => {
        unwrap(api.GET('/api/v1/projects')).then((r) => setProjects(r.projects)).catch(() => {});
    }, []);

    return (
        <Box sx={{ flex: 1, display: 'flex', flexDirection: 'column', justifyContent: 'center', px: 2, pb: '12vh', overflowY: 'auto' }}>
            <Box sx={{ width: '100%', maxWidth: 720, mx: 'auto' }}>
                <Typography variant="h1" sx={{ textAlign: 'center', mb: 4, fontFamily: 'Georgia, "Songti SC", serif', fontWeight: 400, fontSize: { xs: '1.8rem', md: '2.4rem' } }}>
                    {t(greetingKey())}
                </Typography>
                <ConnectionHint />
                <Composer
                    autoFocus
                    minRows={3}
                    model={model}
                    onModelChange={setModel}
                    placeholder={project ? t('home.placeholderProject', { name: project.name }) : t('home.placeholder')}
                    onSend={(text) => start({ text, model, projectId: project?.id })}
                    leftSlot={
                        projects.length > 0 && (
                            <Chip
                                size="small"
                                variant={project ? 'filled' : 'outlined'}
                                icon={<Folder sx={{ fontSize: 16 }} />}
                                label={project ? project.name : t('home.useProject')}
                                onClick={(e) => setAnchor(e.currentTarget)}
                                onDelete={project ? () => setProject(null) : undefined}
                                deleteIcon={project ? <Close /> : undefined}
                                sx={{ maxWidth: 260 }}
                            />
                        )
                    }
                />
                <Menu anchorEl={anchor} open={!!anchor} onClose={() => setAnchor(null)} slotProps={{ paper: { sx: { maxHeight: 360, minWidth: 240 } } }}>
                    {projects.map((p) => (
                        <MenuItem key={p.id} selected={p.id === project?.id} onClick={() => { setProject(p); setAnchor(null); }}>
                            <ListItemIcon><Folder fontSize="small" /></ListItemIcon>
                            <ListItemText primary={p.name} />
                        </MenuItem>
                    ))}
                </Menu>
            </Box>
        </Box>
    );
}
