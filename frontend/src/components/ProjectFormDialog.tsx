import { useEffect, useState } from 'react';
import { Alert, Button, Dialog, DialogActions, DialogContent, DialogTitle, Stack, TextField } from '@mui/material';
import { useTranslation } from 'react-i18next';

interface Props {
    open: boolean;
    mode: 'create' | 'edit';
    initial?: { name: string; description: string };
    onClose: () => void;
    onSubmit: (v: { name: string; description: string }) => Promise<void>;
}

/** Name + description. Neither is sent to the model, as on claude.ai — the
 * dialog says so, so nobody tries to steer the model through the description. */
export function ProjectFormDialog({ open, mode, initial, onClose, onSubmit }: Props) {
    const { t } = useTranslation();
    const [name, setName] = useState('');
    const [description, setDescription] = useState('');
    const [busy, setBusy] = useState(false);

    useEffect(() => {
        if (open) {
            setName(initial?.name ?? '');
            setDescription(initial?.description ?? '');
        }
    }, [open, initial]);

    const submit = async () => {
        if (!name.trim()) return;
        setBusy(true);
        try {
            await onSubmit({ name: name.trim(), description: description.trim() });
            onClose();
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
            <DialogTitle>{mode === 'create' ? t('project.createTitle') : t('project.editTitle')}</DialogTitle>
            <DialogContent>
                <Stack spacing={2.5} sx={{ pt: 1 }}>
                    <TextField
                        label={t('project.nameLabel')}
                        placeholder={t('project.namePlaceholder')}
                        value={name}
                        onChange={(e) => setName(e.target.value)}
                        autoFocus
                        fullWidth
                        onKeyDown={(e) => e.key === 'Enter' && void submit()}
                    />
                    <TextField
                        label={t('project.descriptionLabel')}
                        placeholder={t('project.descriptionPlaceholder')}
                        value={description}
                        onChange={(e) => setDescription(e.target.value)}
                        multiline
                        minRows={3}
                        fullWidth
                    />
                    <Alert severity="info" variant="outlined">{t('project.privateHint')}</Alert>
                </Stack>
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2 }}>
                <Button onClick={onClose} color="inherit">{t('common.cancel')}</Button>
                <Button variant="contained" onClick={() => void submit()} disabled={!name.trim() || busy}>
                    {mode === 'create' ? t('project.create') : t('common.save')}
                </Button>
            </DialogActions>
        </Dialog>
    );
}
