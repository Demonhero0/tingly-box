import { useState, type ReactNode } from 'react';
import { Box, Button, IconButton, List, ListItemButton, ListItemIcon, ListItemText, Tooltip, Typography, useTheme } from '@mui/material';
import { NavLink, useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Add, ChatBubble, DarkMode, Folder, Forum, LightMode, MoreHoriz, Settings, SidebarCollapse, SystemMode } from '@/components/icons';
import { useAppData } from '@/contexts/AppDataContext';
import { useThemeMode, type ThemeMode } from '@/contexts/ThemeModeContext';
import { ChatActionsMenu } from '@/components/ChatActionsMenu';
import type { ConversationSummary } from '@/client';
import { sidebarBg } from '@/theme';

export const SIDEBAR_WIDTH = 272;

function Section({ title, children }: { title: string; children: ReactNode }) {
    return (
        <Box sx={{ mt: 2 }}>
            <Typography variant="caption" sx={{ px: 2, color: 'text.secondary', fontWeight: 600 }}>{title}</Typography>
            <List dense disablePadding sx={{ mt: 0.5 }}>{children}</List>
        </Box>
    );
}

function ChatRow({ chat, selected }: { chat: ConversationSummary; selected: boolean }) {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);
    return (
        <>
            <ListItemButton
                selected={selected}
                component={NavLink}
                to={`/chat/${chat.id}`}
                sx={{ mx: 1, borderRadius: 1.5, py: 0.5, pr: 0.5, '& .row-menu': { opacity: anchor ? 1 : 0 }, '&:hover .row-menu': { opacity: 1 } }}
            >
                <ListItemText
                    primary={chat.title || t('chat.untitled')}
                    slotProps={{ primary: { noWrap: true, sx: { fontSize: 13.5, color: 'text.primary' } } }}
                />
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
                    <MoreHoriz fontSize="inherit" />
                </IconButton>
            </ListItemButton>
            <ChatActionsMenu
                chat={chat}
                anchorEl={anchor}
                onClose={() => setAnchor(null)}
                onDeleted={() => {
                    if (selected) navigate('/');
                }}
            />
        </>
    );
}

const NEXT_MODE: Record<ThemeMode, ThemeMode> = { system: 'light', light: 'dark', dark: 'system' };

export function Sidebar({ onCollapse, onNavigate }: { onCollapse?: () => void; onNavigate?: () => void }) {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const location = useLocation();
    const theme = useTheme();
    const { recents, starredChats, starredProjects } = useAppData();
    const { mode, setMode, resolved } = useThemeMode();
    const starredIds = new Set(starredChats.map((c) => c.id));

    const nav = [
        { to: '/chats', label: t('nav.chats'), icon: <Forum fontSize="small" /> },
        { to: '/projects', label: t('nav.projects'), icon: <Folder fontSize="small" /> },
    ];
    const ModeIcon = mode === 'system' ? SystemMode : mode === 'light' ? LightMode : DarkMode;

    return (
        <Box
            onClick={(e) => {
                if ((e.target as HTMLElement).closest('a')) onNavigate?.();
            }}
            sx={{ width: SIDEBAR_WIDTH, height: '100%', display: 'flex', flexDirection: 'column', bgcolor: sidebarBg(resolved), borderRight: 1, borderColor: 'divider' }}
        >
            <Box sx={{ display: 'flex', alignItems: 'center', px: 2, pt: 1.5, pb: 1 }}>
                <Box component={NavLink} to="/" sx={{ display: 'flex', alignItems: 'center', gap: 1, textDecoration: 'none', color: 'text.primary', flex: 1 }}>
                    <Box component="img" src="/icon.svg" alt="" sx={{ width: 22, height: 22 }} />
                    <Typography sx={{ fontWeight: 650, fontSize: 15 }}>Tingly Project</Typography>
                </Box>
                {onCollapse && (
                    <Tooltip title={t('nav.collapse')}>
                        <IconButton size="small" onClick={onCollapse} aria-label={t('nav.collapse')}>
                            <SidebarCollapse fontSize="small" />
                        </IconButton>
                    </Tooltip>
                )}
            </Box>

            <Box sx={{ px: 1.5 }}>
                <Button
                    fullWidth
                    startIcon={<Add />}
                    onClick={() => {
                        navigate('/');
                        onNavigate?.();
                    }}
                    sx={{ justifyContent: 'flex-start', color: 'primary.main', fontWeight: 600, px: 1.25 }}
                >
                    {t('nav.newChat')}
                </Button>
            </Box>
            <List dense disablePadding sx={{ mt: 0.5 }}>
                {nav.map((n) => (
                    <ListItemButton key={n.to} component={NavLink} to={n.to} selected={location.pathname === n.to} sx={{ mx: 1, borderRadius: 1.5 }}>
                        <ListItemIcon sx={{ minWidth: 32 }}>{n.icon}</ListItemIcon>
                        <ListItemText primary={n.label} slotProps={{ primary: { sx: { fontSize: 14, color: 'text.primary' } } }} />
                    </ListItemButton>
                ))}
            </List>

            <Box sx={{ flex: 1, overflowY: 'auto', pb: 2 }}>
                {(starredProjects.length > 0 || starredChats.length > 0) && (
                    <Section title={t('nav.starred')}>
                        {starredProjects.map((p) => (
                            <ListItemButton key={p.id} component={NavLink} to={`/project/${p.id}`} selected={location.pathname === `/project/${p.id}`} sx={{ mx: 1, borderRadius: 1.5, py: 0.5 }}>
                                <ListItemIcon sx={{ minWidth: 28 }}><Folder sx={{ fontSize: 16 }} /></ListItemIcon>
                                <ListItemText primary={p.name} slotProps={{ primary: { noWrap: true, sx: { fontSize: 13.5, color: 'text.primary' } } }} />
                            </ListItemButton>
                        ))}
                        {starredChats.map((c) => (
                            <ChatRow key={c.id} chat={c} selected={location.pathname === `/chat/${c.id}`} />
                        ))}
                    </Section>
                )}
                <Section title={t('nav.recents')}>
                    {recents.filter((c) => !starredIds.has(c.id)).map((c) => (
                        <ChatRow key={c.id} chat={c} selected={location.pathname === `/chat/${c.id}`} />
                    ))}
                    {recents.length === 0 && (
                        <Box sx={{ px: 2, py: 1, display: 'flex', gap: 1, alignItems: 'center', color: 'text.secondary' }}>
                            <ChatBubble sx={{ fontSize: 16 }} />
                            <Typography variant="body2">{t('nav.noChats')}</Typography>
                        </Box>
                    )}
                </Section>
            </Box>

            <Box sx={{ display: 'flex', alignItems: 'center', px: 1, py: 1, borderTop: 1, borderColor: 'divider', gap: 0.5 }}>
                <ListItemButton component={NavLink} to="/settings" selected={location.pathname === '/settings'} sx={{ borderRadius: 1.5, flex: 1 }}>
                    <ListItemIcon sx={{ minWidth: 32 }}><Settings fontSize="small" /></ListItemIcon>
                    <ListItemText primary={t('nav.settings')} slotProps={{ primary: { sx: { fontSize: 14, color: 'text.primary' } } }} />
                </ListItemButton>
                <Tooltip title={t(`theme.${mode}`)}>
                    <IconButton size="small" onClick={() => setMode(NEXT_MODE[mode])} aria-label={t(`theme.${mode}`)} sx={{ color: theme.palette.text.secondary }}>
                        <ModeIcon fontSize="small" />
                    </IconButton>
                </Tooltip>
            </Box>
        </Box>
    );
}
