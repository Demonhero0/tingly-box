import { Alert, Button } from '@mui/material';
import { Link as RouterLink } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useModels } from '@/hooks/useModels';
import { useAppData } from '@/contexts/AppDataContext';

/**
 * Shown when the configured endpoint cannot even list models: names the
 * concrete endpoint and hands over the way to fix it, instead of letting the
 * first message fail.
 */
export function ConnectionHint() {
    const { t } = useTranslation();
    const { error, loading } = useModels();
    const { settings } = useAppData();
    if (loading || !error || settings?.default_model) return null;
    return (
        <Alert
            severity="warning"
            variant="outlined"
            action={<Button component={RouterLink} to="/settings" color="inherit" size="small">{t('connection.open')}</Button>}
            sx={{ mb: 2, '& .MuiAlert-message': { minWidth: 0 } }}
        >
            {t('connection.unreachable', { endpoint: settings?.base_url ?? '' })}
        </Alert>
    );
}
