// The Agent page's status line (under the title): "is it connected, and is it
// being used" at a glance, plus the two looks-in (Requests, Usage) and — when
// the client's config file has drifted from this page — the one primary
// action, Reapply. See .design/agent-page-redesign.md §3.2 for the config
// state and the Agent page v4 sketch for the layout.
import { Box, Button, Tooltip, Typography } from '@mui/material';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { ClientConfigStatus } from '@/hooks/useClientConfigStatus';
import type { AgentObservation } from '@/pages/scenario/hooks/useAgentObservation';
import { fontMono, fontSizes } from '@/theme/fonts';

/** "2 minutes ago" in the UI language; empty for an unparsable timestamp. */
export const formatRelativeTime = (iso: string, language: string, now: number = Date.now()): string => {
    const then = new Date(iso).getTime();
    if (Number.isNaN(then)) return '';
    const rtf = new Intl.RelativeTimeFormat(language, { numeric: 'auto', style: 'long' });
    const seconds = Math.round((then - now) / 1000);
    if (Math.abs(seconds) < 60) return rtf.format(Math.min(seconds, 0), 'second');
    if (Math.abs(seconds) < 3600) return rtf.format(Math.round(seconds / 60), 'minute');
    if (Math.abs(seconds) < 86400) return rtf.format(Math.round(seconds / 3600), 'hour');
    return rtf.format(Math.round(seconds / 86400), 'day');
};

/** Share of today's requests that failed, as "0.5%"; null with no requests. */
export const formatErrorRate = (requests: number, errors: number): string | null => {
    if (requests <= 0) return null;
    const pct = (errors / requests) * 100;
    return `${pct === 0 ? '0' : pct < 10 ? pct.toFixed(1) : Math.round(pct)}%`;
};

export interface AgentStatusRowProps {
    /** null until the first answer: nothing is claimed while loading. */
    observation: AgentObservation | null;
    /** The real "connected" signal (what Quick Start counts as done). */
    connected: boolean;
    /** The client's config readback, for agents that have one. */
    configStatus: ClientConfigStatus | null;
    /** Opens the page's Auto Config (reapply / first apply). */
    onApplyConfig: () => void;
    onOpenRequests: () => void;
    onOpenUsage: () => void;
    onOpenLog: () => void;
}

const Dot: React.FC<{ color: string }> = ({ color }) => (
    <Box component="span" aria-hidden sx={{ width: 8, height: 8, borderRadius: '50%', bgcolor: color, flexShrink: 0 }} />
);

export const AgentStatusRow: React.FC<AgentStatusRowProps> = ({
    observation,
    connected,
    configStatus,
    onApplyConfig,
    onOpenRequests,
    onOpenUsage,
    onOpenLog,
}) => {
    const { t, i18n } = useTranslation();
    const hasRequests = !!observation?.lastRequestAt;

    const summary = useMemo(() => {
        if (!observation) return null;
        const parts: string[] = [];
        if (connected) parts.push(t('agentStatus.connected'));
        if (observation.lastRequestAt) {
            parts.push(t('agentStatus.lastRequest', { ago: formatRelativeTime(observation.lastRequestAt, i18n.language) }));
            parts.push(t('agentStatus.today', { count: observation.requestsToday }));
            const rate = formatErrorRate(observation.requestsToday, observation.errorsToday);
            if (rate) parts.push(t('agentStatus.errorRate', { rate }));
        } else {
            parts.push(connected ? t('agentStatus.noRequestsYet') : t('agentStatus.noRequestsFinishSetup'));
        }
        return parts.join(' · ');
    }, [observation, connected, t, i18n.language]);

    const outdated = configStatus?.state === 'outdated';
    const notApplied = !!configStatus && configStatus.state !== 'applied' && !outdated;
    const diffs = configStatus?.differences ?? [];
    const path = configStatus?.path ?? '';

    const dotColor = !observation ? 'transparent' : hasRequests && connected ? 'success.main' : 'warning.main';

    return (
        <Box sx={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', columnGap: 2, rowGap: 1, minHeight: 32 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, minWidth: 0, flex: '1 1 320px' }}>
                {summary && (
                    <>
                        <Dot color={dotColor} />
                        <Typography variant="body2" data-testid="agent-status-summary" sx={{ color: 'text.secondary' }}>
                            {summary}
                        </Typography>
                    </>
                )}
            </Box>

            <Box sx={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', gap: 1.5 }}>
                {outdated && (
                    <>
                        <Tooltip
                            arrow
                            title={(
                                <Box>
                                    <Typography variant="caption" sx={{ display: 'block', mb: 0.5 }}>{t('clientConfigStatus.outdatedTooltip', { path })}</Typography>
                                    {diffs.map((d: { key?: string; applied?: string; expected?: string }) => (
                                        <Box key={d.key} sx={{ fontFamily: fontMono, fontSize: 11, lineHeight: 1.5 }}>
                                            {d.key}: {d.applied || '—'} → {d.expected || '—'}
                                        </Box>
                                    ))}
                                </Box>
                            )}
                        >
                            <Typography variant="body2" sx={{ color: 'warning.main', fontWeight: 500, fontSize: fontSizes.md }}>
                                {t('agentStatus.stale', { count: diffs.length })}
                            </Typography>
                        </Tooltip>
                        <Button variant="contained" size="small" onClick={onApplyConfig}>
                            {t('clientConfigStatus.reapply')}
                        </Button>
                    </>
                )}
                {notApplied && (
                    <>
                        <Tooltip arrow title={t('clientConfigStatus.notAppliedTooltip', { path })}>
                            <Typography variant="body2" sx={{ color: 'text.secondary', fontSize: fontSizes.md }}>
                                {t('clientConfigStatus.notApplied')}
                            </Typography>
                        </Tooltip>
                        <Button variant="contained" size="small" onClick={onApplyConfig}>
                            {t('clientConfigStatus.apply')}
                        </Button>
                    </>
                )}
                <Button variant="outlined" size="small" onClick={onOpenRequests}>
                    {t('agentStatus.requests')}
                </Button>
                <Tooltip title={t('agentStatus.usageTooltip')} describeChild>
                    <Button variant="outlined" size="small" onClick={onOpenUsage}>
                        {t('agentStatus.usage')}
                    </Button>
                </Tooltip>
                <Tooltip title={t('agentStatus.logTooltip')} describeChild>
                    <Button variant="outlined" size="small" onClick={onOpenLog}>
                        {t('agentStatus.log')}
                    </Button>
                </Tooltip>
            </Box>
        </Box>
    );
};

export default AgentStatusRow;
