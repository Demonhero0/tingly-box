import { Box, Button, Chip, Dialog, DialogActions, DialogContent, IconButton, Stack, Tooltip, Typography } from '@mui/material';
import { Refresh as RefreshIcon } from '@/components/icons';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link as RouterLink } from 'react-router-dom';
import DialogHeader from '@/components/DialogHeader';
// Straight from runProbe, not the probe barrel (which pulls in the probe dialogs).
import { formatLatency } from '@/components/probe/runProbe';
import { api } from '@/services/api';
import { fontMono, fontSizes } from '@/theme/fonts';

const REQUEST_LIMIT = 20;
const REFRESH_MS = 30_000;

interface RecentRequest {
    id: number;
    timestamp: string;
    status: string;
    model: string;
    request_model?: string;
    provider_name?: string;
    latency_ms?: number;
    error_code?: string;
}

/**
 * AgentActivityDialog — a quick look at one agent's latest requests, opened
 * from the "Requests" button in its status row. It links into the dashboard
 * filtered to this agent, which keeps the analysis half. Nothing loads or
 * refreshes while the dialog is closed.
 */
const AgentActivityDialog: React.FC<{
    scenario: string;
    open: boolean;
    onClose: () => void;
}> = ({ scenario, open, onClose }) => {
    const { t, i18n } = useTranslation();
    const [recent, setRecent] = useState<RecentRequest[] | null>(null);
    const [loading, setLoading] = useState(false);
    const [failed, setFailed] = useState(false);
    const seq = useRef(0);

    const load = useCallback(async () => {
        const mine = ++seq.current;
        setLoading(true);
        try {
            const records = await api.getUsageRecords({ scenario, limit: REQUEST_LIMIT });
            if (mine !== seq.current) return;
            setRecent(Array.isArray(records?.data) ? records.data : []);
            setFailed(false);
        } catch {
            if (mine === seq.current) setFailed(true);
        } finally {
            if (mine === seq.current) setLoading(false);
        }
    }, [scenario]);

    useEffect(() => {
        if (!open) return;
        void load();
        // Watching an agent means leaving this open: keep it fresh, but not
        // from a background tab.
        const timer = window.setInterval(() => {
            if (document.visibilityState === 'visible') void load();
        }, REFRESH_MS);
        return () => window.clearInterval(timer);
    }, [open, load]);

    const rtf = useMemo(() => new Intl.RelativeTimeFormat(i18n.language, { numeric: 'auto', style: 'narrow' }), [i18n.language]);
    const relative = (iso: string) => {
        const seconds = Math.round((new Date(iso).getTime() - Date.now()) / 1000);
        if (Math.abs(seconds) < 60) return rtf.format(seconds, 'second');
        if (Math.abs(seconds) < 3600) return rtf.format(Math.round(seconds / 60), 'minute');
        if (Math.abs(seconds) < 86400) return rtf.format(Math.round(seconds / 3600), 'hour');
        return rtf.format(Math.round(seconds / 86400), 'day');
    };

    const dashboardHref = `/dashboard/today?scenario=${encodeURIComponent(scenario)}`;
    const titleId = 'agent-requests-title';

    let body: React.ReactNode;
    if (failed && recent === null) {
        body = <Typography variant="body2" sx={{ color: 'text.secondary' }}>{t('agentActivity.loadFailed')}</Typography>;
    } else if (recent !== null && recent.length === 0) {
        // Nothing yet is itself the signal: the first request will show up here.
        body = <Typography variant="body2" sx={{ color: 'text.secondary' }}>{t('agentActivity.empty')}</Typography>;
    } else {
        body = (
            <Stack divider={<Box sx={{ borderTop: '1px solid', borderColor: 'divider' }} />}>
                {(recent ?? []).map((r) => {
                    const ok = r.status !== 'error';
                    const route = r.request_model && r.request_model !== r.model
                        ? `${r.request_model} → ${r.model}`
                        : r.model;
                    return (
                        <Stack key={r.id} direction="row" spacing={1.5} sx={{ alignItems: 'center', py: 0.75, minWidth: 0 }}>
                            <Typography variant="caption" sx={{ color: 'text.secondary', width: 56, flexShrink: 0 }}>
                                {relative(r.timestamp)}
                            </Typography>
                            <Chip
                                size="small"
                                variant="outlined"
                                color={ok ? 'success' : 'error'}
                                label={ok ? t('agentActivity.ok') : (r.error_code || t('agentActivity.error'))}
                                sx={{ height: 20, fontSize: fontSizes.xs, flexShrink: 0 }}
                            />
                            <Typography variant="body2" noWrap sx={{ fontFamily: fontMono, minWidth: 0, flex: 1 }} title={route}>
                                {route}
                            </Typography>
                            {r.provider_name && (
                                <Typography variant="caption" noWrap sx={{ color: 'text.secondary', maxWidth: 140, display: { xs: 'none', sm: 'block' } }}>
                                    {r.provider_name}
                                </Typography>
                            )}
                            {r.latency_ms ? (
                                <Typography variant="caption" sx={{ color: 'text.secondary', width: 48, textAlign: 'right', flexShrink: 0 }}>
                                    {formatLatency(r.latency_ms)}
                                </Typography>
                            ) : null}
                        </Stack>
                    );
                })}
            </Stack>
        );
    }

    return (
        <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth aria-labelledby={titleId}>
            <DialogHeader
                title={t('agentActivity.requests')}
                titleId={titleId}
                closeLabel={t('common.close')}
                onClose={onClose}
            />
            <DialogContent>{body}</DialogContent>
            <DialogActions sx={{ px: 3, pb: 2, justifyContent: 'space-between' }}>
                <Tooltip title={t('agentActivity.refresh')}>
                    <span>
                        <IconButton size="small" onClick={() => void load()} disabled={loading} aria-label={t('agentActivity.refresh')}>
                            <RefreshIcon fontSize="small" />
                        </IconButton>
                    </span>
                </Tooltip>
                <Button component={RouterLink} to={dashboardHref} onClick={onClose} size="small" variant="contained">
                    {t('agentActivity.openDashboard')}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default AgentActivityDialog;
