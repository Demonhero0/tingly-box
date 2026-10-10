import { useState } from 'react';
import { Divider, ListItemIcon, ListItemText, Menu, MenuItem, Tooltip } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { api, unwrap, type Project } from '@/client';
import { Archive, Delete, Edit, Star, StarBorder, Unarchive } from '@/components/icons';
import { useNotify } from '@/hooks/useNotify';
import { useAppData } from '@/contexts/AppDataContext';
import { ConfirmDialog } from './ConfirmDialog';
import { ProjectFormDialog } from './ProjectFormDialog';

type ProjectLike = Pick<Project, 'id' | 'name' | 'description' | 'starred' | 'archived_at'>;

interface Props {
    project: ProjectLike;
    anchorEl: HTMLElement | null;
    onClose: () => void;
    onChanged?: (p: Project) => void;
    onDeleted?: () => void;
}

/** Star / edit details / archive or unarchive / delete (only when not archived). */
export function ProjectActionsMenu({ project, anchorEl, onClose, onChanged, onDeleted }: Props) {
    const { t } = useTranslation();
    const notify = useNotify();
    const { refresh } = useAppData();
    const [editing, setEditing] = useState(false);
    const [deleting, setDeleting] = useState(false);
    const archived = !!project.archived_at;

    const patch = async (body: { name?: string; description?: string; starred?: boolean; archived?: boolean }) => {
        try {
            const p = await unwrap(api.PATCH('/api/v1/projects/{id}', { params: { path: { id: project.id } }, body }));
            onChanged?.(p);
            refresh();
            return p;
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
                <MenuItem onClick={act(() => void patch({ starred: !project.starred }))}>
                    <ListItemIcon>{project.starred ? <Star fontSize="small" color="primary" /> : <StarBorder fontSize="small" />}</ListItemIcon>
                    <ListItemText>{project.starred ? t('common.unstar') : t('common.star')}</ListItemText>
                </MenuItem>
                <MenuItem onClick={act(() => setEditing(true))}>
                    <ListItemIcon><Edit fontSize="small" /></ListItemIcon>
                    <ListItemText>{t('project.editDetails')}</ListItemText>
                </MenuItem>
                <MenuItem onClick={act(() => void patch({ archived: !archived }).then(() => notify(archived ? t('project.unarchived') : t('project.archived'), 'success')))}>
                    <ListItemIcon>{archived ? <Unarchive fontSize="small" /> : <Archive fontSize="small" />}</ListItemIcon>
                    <ListItemText>{archived ? t('project.unarchive') : t('project.archive')}</ListItemText>
                </MenuItem>
                <Divider />
                <Tooltip title={archived ? t('project.unarchiveToDelete') : ''} placement="left">
                    <span>
                        <MenuItem disabled={archived} onClick={act(() => setDeleting(true))} sx={{ color: 'error.main' }}>
                            <ListItemIcon><Delete fontSize="small" color="error" /></ListItemIcon>
                            <ListItemText>{t('common.delete')}</ListItemText>
                        </MenuItem>
                    </span>
                </Tooltip>
            </Menu>

            <ProjectFormDialog
                open={editing}
                mode="edit"
                initial={{ name: project.name, description: project.description }}
                onClose={() => setEditing(false)}
                onSubmit={async (v) => {
                    await patch(v);
                }}
            />
            <ConfirmDialog
                open={deleting}
                title={t('project.deleteTitle')}
                message={t('project.deleteMessage', { name: project.name })}
                confirmLabel={t('project.deleteConfirm')}
                danger
                onClose={() => setDeleting(false)}
                onConfirm={() => {
                    unwrap(api.DELETE('/api/v1/projects/{id}', { params: { path: { id: project.id } } }))
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
