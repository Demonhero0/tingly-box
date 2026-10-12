import { Box, Chip, type ChipProps } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { CheckCircle, CircleDashed, HandStop, Merge, Spinner } from '@/components/icons';
import type { Thread } from '@/client';

/** Overview groups, in the order the Overview shows them: what needs the
 * user first. */
export const STATUS_ORDER = ['waiting', 'ready', 'working', 'idle', 'resolved'] as const;
export type ThreadStatus = (typeof STATUS_ORDER)[number];

const COLOR: Record<ThreadStatus, ChipProps['color']> = {
    waiting: 'warning',
    ready: 'success',
    working: 'info',
    idle: 'default',
    resolved: 'default',
};

export function statusColor(s: string): ChipProps['color'] {
    return COLOR[s as ThreadStatus] ?? 'default';
}

function StatusIcon({ status }: { status: string }) {
    const sx = { fontSize: 14 };
    switch (status) {
        case 'working':
            return <Spinner sx={{ ...sx, animation: 'spin 1s linear infinite' }} />;
        case 'waiting':
            return <HandStop sx={sx} />;
        case 'ready':
            return <Merge sx={sx} />;
        case 'resolved':
            return <CheckCircle sx={sx} />;
        default:
            return <CircleDashed sx={sx} />;
    }
}

export function ThreadStatusChip({ thread, size = 'small' }: { thread: Pick<Thread, 'status'>; size?: 'small' | 'medium' }) {
    const { t } = useTranslation();
    return (
        <Chip
            size={size}
            color={statusColor(thread.status)}
            variant={thread.status === 'resolved' || thread.status === 'idle' ? 'outlined' : 'filled'}
            icon={<Box component="span" sx={{ display: 'inline-flex', pl: 0.5 }}><StatusIcon status={thread.status} /></Box>}
            label={t(`thread.status.${thread.status}`)}
            sx={{ height: 22, fontSize: 12, '& .MuiChip-label': { px: 0.75 } }}
        />
    );
}
