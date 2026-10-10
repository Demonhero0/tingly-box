import { useEffect, useState } from 'react';
import { Button, Dialog, DialogActions, DialogContent, DialogTitle, Divider, ListItemIcon, ListItemText, Menu, MenuItem, TextField } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { api, unwrap, type Conversation } from '@/client';
import { Delete, DriveFileMove, Edit, FolderOff, Star, StarBorder } from '@/components/icons';
import { useNotify } from '@/hooks/useNotify';
import { useAppData } from '@/contexts/AppDataContext';
import { ConfirmDialog } from './ConfirmDialog';
import { MoveChatDialog } from './MoveChatDialog';

type ChatLike = Pick<Conversation, 'id' | 'title' | 'starred' | 'project_id'>;

interface Props {
    chat: ChatLike;
    anchorEl: HTMLElement | null;
    onClose: () => void;
    onChanged?: (c: Conversation) => void;
    onDeleted?: () => void;
}

/** Star / rename / add to or move between projects / remove from project / delete. */
export function ChatActionsMenu({ chat, anchorEl, onClose, onChanged, onDeleted }: Props) {
    const { t } = useTranslation();
    const notify = useNotify();
    const { refresh } = useAppData();
    const [renaming, setRenaming] = useState(false);
    const [moving, setMoving] = useState(false);
    const [deleting, setDeleting] = useState(false);
    const [title, setTitle] = useState(chat.title);

    useEffect(() => setTitle(chat.title), [chat.title]);

    const patch = async (body: { title?: string; starred?: boolean; project_id?: string }) => {
        try {
            const c = await unwrap(api.PATCH('/api/v1/conversations/{id}', { params: { path: { id: chat.id } }, body }));
            onChanged?.(c);
            refresh();
            return c;
        } catch (e) {
            notify((e as Error).message, 'error');
            throw e;
        }
    };

    const act = (fn: () => void) => () => {
        onClose();
        fn();
    };

    return (
        <>
            <Menu anchorEl={anchorEl} open={!!anchorEl} onClose={onClose}>
                <MenuItem onClick={act(() => void patch({ starred: !chat.starred }))}>
                    <ListItemIcon>{chat.starred ? <Star fontSize="small" color="primary" /> : <StarBorder fontSize="small" />}</ListItemIcon>
                    <ListItemText>{chat.starred ? t('common.unstar') : t('common.star')}</ListItemText>
                </MenuItem>
                <MenuItem onClick={act(() => setRenaming(true))}>
                    <ListItemIcon><Edit fontSize="small" /></ListItemIcon>
                    <ListItemText>{t('common.rename')}</ListItemText>
                </MenuItem>
                <MenuItem onClick={act(() => setMoving(true))}>
                    <ListItemIcon><DriveFileMove fontSize="small" /></ListItemIcon>
                    <ListItemText>{chat.project_id ? t('chat.moveToProject') : t('chat.addToProject')}</ListItemText>
                </MenuItem>
                {chat.project_id && (
                    <MenuItem onClick={act(() => void patch({ project_id: '' }).then(() => notify(t('chat.removedFromProject'), 'success')))}>
                        <ListItemIcon><FolderOff fontSize="small" /></ListItemIcon>
                        <ListItemText>{t('chat.removeFromProject')}</ListItemText>
                    </MenuItem>
                )}
                <Divider />
                <MenuItem onClick={act(() => setDeleting(true))} sx={{ color: 'error.main' }}>
                    <ListItemIcon><Delete fontSize="small" color="error" /></ListItemIcon>
                    <ListItemText>{t('common.delete')}</ListItemText>
                </MenuItem>
            </Menu>

            <Dialog open={renaming} onClose={() => setRenaming(false)} maxWidth="xs" fullWidth>
                <DialogTitle>{t('chat.renameTitle')}</DialogTitle>
                <DialogContent>
                    <TextField
                        autoFocus
                        fullWidth
                        value={title}
                        onChange={(e) => setTitle(e.target.value)}
                        sx={{ mt: 1 }}
                        onKeyDown={(e) => {
                            if (e.key === 'Enter') void patch({ title }).then(() => setRenaming(false));
                        }}
                    />
                </DialogContent>
                <DialogActions sx={{ px: 3, pb: 2 }}>
                    <Button onClick={() => setRenaming(false)} color="inherit">{t('common.cancel')}</Button>
                    <Button variant="contained" onClick={() => void patch({ title }).then(() => setRenaming(false))}>{t('common.save')}</Button>
                </DialogActions>
            </Dialog>

            <MoveChatDialog
                open={moving}
                currentProjectId={chat.project_id || undefined}
                onClose={() => setMoving(false)}
                onPick={async (pid) => {
                    await patch({ project_id: pid });
                    notify(t('chat.moved'), 'success');
                }}
            />

            <ConfirmDialog
                open={deleting}
                title={t('chat.deleteTitle')}
                message={t('chat.deleteMessage', { title: chat.title || t('chat.untitled') })}
                confirmLabel={t('common.delete')}
                danger
                onClose={() => setDeleting(false)}
                onConfirm={() => {
                    unwrap(api.DELETE('/api/v1/conversations/{id}', { params: { path: { id: chat.id } } }))
                        .then(() => {
                            refresh();
                            onDeleted?.();
                        })
                        .catch((e: Error) => notify(e.message, 'error'));
                }}
            />
        </>
    );
}
