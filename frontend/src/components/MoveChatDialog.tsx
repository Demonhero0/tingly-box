import { useEffect, useMemo, useState } from 'react';
import { Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, InputAdornment, List, ListItemButton, ListItemIcon, ListItemText, TextField, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { api, unwrap, type ProjectSummary } from '@/client';
import { Folder, Search } from '@/components/icons';

interface Props {
    open: boolean;
    currentProjectId?: string;
    onClose: () => void;
    onPick: (projectId: string) => Promise<void>;
}

/** "Move chat" — browse or search active projects. */
export function MoveChatDialog({ open, currentProjectId, onClose, onPick }: Props) {
    const { t } = useTranslation();
    const [projects, setProjects] = useState<ProjectSummary[]>([]);
    const [q, setQ] = useState('');

    useEffect(() => {
        if (!open) return;
        setQ('');
        unwrap(api.GET('/api/v1/projects')).then((r) => setProjects(r.projects)).catch(() => setProjects([]));
    }, [open]);

    const shown = useMemo(() => {
        const needle = q.trim().toLowerCase();
        return projects.filter((p) => p.id !== currentProjectId && (!needle || p.name.toLowerCase().includes(needle)));
    }, [projects, q, currentProjectId]);

    return (
        <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth>
            <DialogTitle>{currentProjectId ? t('chat.moveTitle') : t('chat.addToProjectTitle')}</DialogTitle>
            <DialogContent sx={{ pb: 0 }}>
                <TextField
                    size="small"
                    fullWidth
                    autoFocus
                    placeholder={t('projects.search')}
                    value={q}
                    onChange={(e) => setQ(e.target.value)}
                    slotProps={{ input: { startAdornment: <InputAdornment position="start"><Search fontSize="small" /></InputAdornment> } }}
                />
                <List dense sx={{ mt: 1, maxHeight: 340, overflow: 'auto' }}>
                    {shown.map((p) => (
                        <ListItemButton key={p.id} onClick={() => void onPick(p.id).then(onClose)} sx={{ borderRadius: 1.5 }}>
                            <ListItemIcon sx={{ minWidth: 34 }}><Folder fontSize="small" /></ListItemIcon>
                            <ListItemText primary={p.name} secondary={p.description || undefined} slotProps={{ secondary: { noWrap: true } }} />
                        </ListItemButton>
                    ))}
                    {shown.length === 0 && (
                        <Box sx={{ py: 3, textAlign: 'center' }}>
                            <Typography variant="body2" color="text.secondary">{t('projects.noneFound')}</Typography>
                        </Box>
                    )}
                </List>
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2 }}>
                <Button onClick={onClose} color="inherit">{t('common.cancel')}</Button>
            </DialogActions>
        </Dialog>
    );
}
