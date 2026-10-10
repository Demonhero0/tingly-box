import { Button, Dialog, DialogActions, DialogContent, DialogTitle, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';

interface Props {
    open: boolean;
    title: string;
    message: string;
    confirmLabel: string;
    danger?: boolean;
    onConfirm: () => void;
    onClose: () => void;
}

export function ConfirmDialog({ open, title, message, confirmLabel, danger, onConfirm, onClose }: Props) {
    const { t } = useTranslation();
    return (
        <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth>
            <DialogTitle>{title}</DialogTitle>
            <DialogContent>
                <Typography variant="body1" color="text.secondary">{message}</Typography>
            </DialogContent>
            <DialogActions sx={{ px: 3, pb: 2 }}>
                <Button onClick={onClose} color="inherit">{t('common.cancel')}</Button>
                <Button variant="contained" color={danger ? 'error' : 'primary'} onClick={() => { onConfirm(); onClose(); }}>
                    {confirmLabel}
                </Button>
            </DialogActions>
        </Dialog>
    );
}
