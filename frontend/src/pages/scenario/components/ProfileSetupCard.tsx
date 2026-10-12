import { Box, Button, Card, CircularProgress, IconButton, Stack, Tooltip, Typography } from '@mui/material';
import { CheckCircle as CheckCircleIcon, ContentCopy as ContentCopyIcon } from '@/components/icons';
import { useTranslation } from 'react-i18next';
import { useCopyFeedback } from '@/hooks/useCopyFeedback';
import { fontMono, fontSizes } from '@/theme/fonts';

export interface ProfileSetupCardProps {
    /** The launch command for this profile, as the connection card shows it. */
    command: string;
    /** An AI provider is connected (null while loading). */
    providerConnected: boolean | null;
    /** This profile has sent a request through the gateway (null while loading). */
    launched: boolean | null;
    onConnectProvider: () => void;
    /** Shown once every step is done, to put a card that was brought back away again. */
    onDismiss?: () => void;
}

const Step: React.FC<{ n: number; done: boolean; active: boolean; loading?: boolean }> = ({ n, done, active, loading }) => (
    loading ? <CircularProgress size={20} sx={{ flexShrink: 0 }} /> : done ? (
        <CheckCircleIcon sx={{ fontSize: 22, color: 'success.main', flexShrink: 0 }} />
    ) : (
        <Box sx={{
            width: 22, height: 22, borderRadius: '50%', flexShrink: 0,
            display: 'flex', alignItems: 'center', justifyContent: 'center',
            bgcolor: active ? 'primary.main' : 'action.disabledBackground',
            color: active ? 'primary.contrastText' : 'text.disabled',
            fontSize: fontSizes.xs, fontWeight: 700,
        }}>{n}</Box>
    )
);

/**
 * What a new Profile page is missing next to an Agent page's Quick Start: two
 * steps that say how to use it. A profile inherits the main routing, so there is
 * no model step; "done" comes from facts (a provider exists, a request arrived),
 * never from a click.
 */
export const ProfileSetupCard: React.FC<ProfileSetupCardProps> = ({ command, providerConnected, launched, onConnectProvider, onDismiss }) => {
    const { t } = useTranslation();
    const { copied, copy } = useCopyFeedback(1500);
    const providerDone = providerConnected === true;
    const launchDone = launched === true;
    const firstOpen = !providerDone ? 1 : !launchDone ? 2 : 0;

    return (
        <Card sx={{ borderRadius: 2, border: '1px solid', borderColor: 'divider', boxShadow: 'none', backgroundColor: 'background.paper', p: 2 }} data-testid="profile-setup-card">
            <Stack direction="row" sx={{ alignItems: 'center', justifyContent: 'space-between', mb: 1 }}>
                <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>{t('agentSetup.profile.title')}</Typography>
                {firstOpen === 0 && onDismiss && (
                    <Button size="small" variant="text" onClick={onDismiss} sx={{ textTransform: 'none', color: 'text.secondary' }}>
                        {t('agentSetup.profile.hide')}
                    </Button>
                )}
            </Stack>
            <Stack spacing={1}>
                <Stack direction="row" spacing={1.25} sx={{ alignItems: 'center', flexWrap: 'wrap', rowGap: 0.5, px: 1.5 }}>
                    <Step n={1} done={providerDone} active={firstOpen === 1} loading={providerConnected === null} />
                    <Typography variant="body2" sx={{ fontWeight: 500, flex: 1 }} color={providerDone ? 'text.primary' : 'primary.main'}>
                        {t('agentSetup.profile.step1')}
                    </Typography>
                    {providerDone
                        ? <Typography variant="body2" color="text.secondary">{t('agentSetup.profile.step1Done')}</Typography>
                        : <Button size="small" variant="contained" onClick={onConnectProvider} sx={{ py: 0.25 }}>{t('agentSetup.profile.connect')}</Button>}
                </Stack>
                <Box sx={{ px: 1.5 }}>
                    <Stack direction="row" spacing={1.25} sx={{ alignItems: 'center', flexWrap: 'wrap', rowGap: 0.5 }}>
                        <Step n={2} done={launchDone} active={firstOpen === 2} loading={launched === null} />
                        <Typography variant="body2" sx={{ fontWeight: 500, flex: 1 }} color={launchDone ? 'text.primary' : firstOpen === 2 ? 'primary.main' : 'text.disabled'}>
                            {t('agentSetup.profile.step2')}
                        </Typography>
                        {launchDone && <Typography variant="body2" color="text.secondary">{t('agentSetup.profile.step2Done')}</Typography>}
                    </Stack>
                    {!launchDone && (
                        <Box sx={{ pl: 4.25, mt: 0.75 }}>
                            <Typography variant="body2" color="text.secondary" sx={{ mb: 0.75 }}>{t('agentSetup.profile.step2Hint')}</Typography>
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, maxWidth: 640, px: 1.5, py: 0.5, borderRadius: 1.5, bgcolor: 'action.hover' }}>
                                <Typography variant="body2" sx={{ fontFamily: fontMono, fontSize: fontSizes.md, flex: 1, minWidth: 0, overflowWrap: 'anywhere' }}>{command}</Typography>
                                <Tooltip title={copied ? t('common.copied', { defaultValue: 'Copied' }) : t('agentSetup.profile.copy')}>
                                    <IconButton size="small" aria-label={t('agentSetup.profile.copy')} onClick={() => copy(command)}>
                                        <ContentCopyIcon fontSize="small" />
                                    </IconButton>
                                </Tooltip>
                            </Box>
                        </Box>
                    )}
                </Box>
            </Stack>
            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1.5, px: 1.5 }}>
                {t('agentSetup.profile.inherit')}
            </Typography>
        </Card>
    );
};

export default ProfileSetupCard;
