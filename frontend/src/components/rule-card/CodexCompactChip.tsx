import { Box, Button, Link, Popover, Slider, Stack, Tooltip, Typography } from '@mui/material';
import { alpha, type SxProps, type Theme } from '@mui/material/styles';
import React from 'react';
import { useTranslation } from 'react-i18next';
import { ExpandMore as ExpandMoreIcon } from '@/components/icons';
import { api } from '@/services/api';
import { fontSizes } from '@/theme/fonts';
import {
    CODEX_COMPACT,
    lowZoneStart,
    planCodexCompact,
    trackPosition,
    type CodexCompactStatus,
} from './codexCompact';

interface CodexCompactChipProps {
    /** The rule's own percent; 0 = follow the Auto Config default. */
    percent: number;
    /** Whether the rule's 1M flag is on (the catalog window is then 1M). */
    context1M: boolean;
    disabled?: boolean;
    onChange: (percent: number) => void;
    buttonSx: SxProps<Theme>;
    highlightedButtonSx: SxProps<Theme>;
}

const fmt = (n: number) => n.toLocaleString('en-US');

// What Auto Config currently holds (the shared default every rule without its
// own value follows). Read lazily, when the popover opens.
interface SharedDefaults {
    percent: number;
    effective: number;
}

const parsePercent = (raw: unknown, fallback: number): number => {
    const n = Number.parseInt(String(raw ?? ''), 10);
    return Number.isFinite(n) && n >= CODEX_COMPACT.min && n <= 100 ? n : fallback;
};

const NOTE_COLOR: Record<Exclude<CodexCompactStatus, 'ok'>, 'warning' | 'info'> = {
    low: 'warning',
    limited: 'info',
};

/**
 * The "Compact: 75%" chip on a Codex rule's header: opens a popover with a
 * slider for the share of the context window at which Codex auto-compacts this
 * model. Config-only — it lands in the generated Codex model catalog the next
 * time Auto Config is applied and never touches request routing.
 */
export const CodexCompactChip: React.FC<CodexCompactChipProps> = ({
    percent,
    context1M,
    disabled,
    onChange,
    buttonSx,
    highlightedButtonSx,
}) => {
    const { t } = useTranslation();
    const [anchor, setAnchor] = React.useState<HTMLElement | null>(null);
    const [draft, setDraft] = React.useState<number | null>(null);
    const [shared, setShared] = React.useState<SharedDefaults>({
        percent: CODEX_COMPACT.defaultPercent,
        effective: CODEX_COMPACT.defaultEffectivePercent,
    });

    const open = Boolean(anchor);
    const overridden = percent > 0;
    const shown = draft ?? (overridden ? percent : shared.percent);
    const window = context1M ? CODEX_COMPACT.window1M : CODEX_COMPACT.window;
    const plan = planCodexCompact({ window, percent: shown, effectivePercent: shared.effective });
    const lowStart = lowZoneStart(window);
    const noteKind = plan.status === 'ok' ? null : NOTE_COLOR[plan.status];

    React.useEffect(() => {
        if (!open) return;
        let cancelled = false;
        (async () => {
            try {
                const result = await api.getAppliedCodexConfig();
                const prefs = result?.preferences ?? {};
                if (!cancelled) {
                    setShared({
                        percent: parsePercent(prefs.auto_compact_percent, CODEX_COMPACT.defaultPercent),
                        effective: parsePercent(prefs.effective_context_window_percent, CODEX_COMPACT.defaultEffectivePercent),
                    });
                }
            } catch {
                // Keep the built-in defaults; the preview is only a hint.
            }
        })();
        return () => { cancelled = true; };
    }, [open]);

    // Arrow keys commit on every press; wait until the value settles so one
    // adjustment is one save.
    const pending = React.useRef<{ timer: ReturnType<typeof setTimeout>; value: number } | null>(null);
    const flush = React.useCallback(() => {
        if (!pending.current) return;
        clearTimeout(pending.current.timer);
        const { value } = pending.current;
        pending.current = null;
        onChange(value);
        setDraft(null);
    }, [onChange]);
    const commit = (value: number) => {
        if (pending.current) clearTimeout(pending.current.timer);
        pending.current = { value, timer: setTimeout(flush, 500) };
    };
    // A value still waiting out its debounce is saved, not dropped, if the card
    // goes away first.
    const flushRef = React.useRef(flush);
    flushRef.current = flush;
    React.useEffect(() => () => flushRef.current(), []);

    const close = () => {
        flush();
        setAnchor(null);
        setDraft(null);
    };

    const noteText = {
        low: t('ruleHeader.compact.low', { remain: fmt(plan.remain), min: fmt(CODEX_COMPACT.minRemaining) }),
        limited: t('ruleHeader.compact.limited', { effective: shared.effective, applied: plan.applied }),
        ok: '',
    }[plan.status];

    return (
        <Box sx={{ ml: 0.5 }} onClick={(e) => e.stopPropagation()}>
            <Tooltip title={t('ruleHeader.compact.tooltip')}>
                <span>
                    <Button
                        size="small"
                        variant="outlined"
                        color={overridden ? 'primary' : 'inherit'}
                        endIcon={<ExpandMoreIcon sx={{ fontSize: 14 }} />}
                        disabled={disabled}
                        aria-label={t('ruleHeader.compact.aria', { value: overridden ? `${percent}%` : t('ruleHeader.compact.auto') })}
                        aria-haspopup="dialog"
                        aria-expanded={open}
                        onClick={(e) => setAnchor(e.currentTarget)}
                        sx={overridden ? highlightedButtonSx : buttonSx}
                    >
                        {t('ruleHeader.compact.chip', { value: overridden ? `${percent}%` : t('ruleHeader.compact.auto') })}
                    </Button>
                </span>
            </Tooltip>
            <Popover
                open={open}
                anchorEl={anchor}
                anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
                transformOrigin={{ vertical: 'top', horizontal: 'left' }}
                onClose={close}
                onClick={(e) => e.stopPropagation()}
                slotProps={{ paper: { sx: { width: 340, p: 2, borderRadius: 2 } } }}
            >
                <Stack spacing={1.5}>
                    <Stack direction="row" sx={{ alignItems: 'baseline', justifyContent: 'space-between' }}>
                        <Typography sx={{ fontSize: fontSizes.md, fontWeight: 600 }}>{t('ruleHeader.compact.title')}</Typography>
                        <Typography sx={{ fontSize: fontSizes.lg, fontWeight: 600, fontVariantNumeric: 'tabular-nums' }}>
                            {shown}%
                        </Typography>
                    </Stack>
                    <Typography variant="body2" sx={{ color: 'text.secondary', fontSize: fontSizes.sm }}>
                        {overridden || draft !== null
                            ? t('ruleHeader.compact.description')
                            : t('ruleHeader.compact.following', { percent: shared.percent })}
                    </Typography>

                    <Box>
                        <Box sx={{ position: 'relative', px: 0.5 }}>
                            {/* Rail and the "too few tokens left" zone. */}
                            <Box sx={{ position: 'absolute', left: 4, right: 4, top: '50%', height: 4, mt: '-2px', borderRadius: 2, bgcolor: 'action.disabledBackground', overflow: 'hidden' }}>
                                {lowStart !== null && (
                                    <Box sx={{
                                        position: 'absolute', top: 0, bottom: 0,
                                        left: `${trackPosition(lowStart)}%`,
                                        right: 0,
                                        bgcolor: (theme) => alpha(theme.palette.warning.main, 0.55),
                                    }} />
                                )}
                            </Box>
                            <Slider
                                size="small"
                                min={CODEX_COMPACT.min}
                                max={CODEX_COMPACT.max}
                                step={1}
                                value={shown}
                                disabled={disabled}
                                onChange={(_, v) => setDraft(v as number)}
                                onChangeCommitted={(_, v) => {
                                    const next = v as number;
                                    setDraft(next);
                                    commit(next);
                                }}
                                aria-label={t('ruleHeader.compact.title')}
                                valueLabelDisplay="off"
                                sx={{ '& .MuiSlider-rail': { opacity: 0 }, '& .MuiSlider-track': { height: 4, border: 0 } }}
                            />
                        </Box>
                        <Box sx={{ position: 'relative', height: 16, fontSize: fontSizes.xs, color: 'text.secondary', fontVariantNumeric: 'tabular-nums' }}>
                            <span style={{ position: 'absolute', left: 0 }}>{CODEX_COMPACT.min}%</span>
                            {lowStart !== null && (
                                <span style={{ position: 'absolute', left: `${trackPosition(lowStart)}%`, transform: 'translateX(-50%)' }}>
                                    {Math.round(lowStart)}%
                                </span>
                            )}
                            <span style={{ position: 'absolute', right: 0 }}>100%</span>
                        </Box>
                    </Box>

                    <Stack spacing={0.25} sx={{ fontSize: fontSizes.sm, fontVariantNumeric: 'tabular-nums' }}>
                        {[
                            [t('ruleHeader.compact.window'), context1M ? '1M' : '200K', undefined],
                            [t('ruleHeader.compact.line'), `${plan.applied}% · ${fmt(plan.line)} tokens`, undefined],
                            [t('ruleHeader.compact.remain'), `${fmt(plan.remain)} tokens`, plan.status === 'low' ? 'warning.dark' : 'text.primary'],
                        ].map(([label, value, color]) => (
                            <Stack key={label} direction="row" sx={{ justifyContent: 'space-between' }}>
                                <Box component="span" sx={{ color: 'text.secondary' }}>{label}</Box>
                                <Box component="span" sx={{ color: color ?? 'text.primary', fontWeight: color ? 600 : 400 }}>{value}</Box>
                            </Stack>
                        ))}
                    </Stack>

                    {noteKind && (
                        <Box sx={{
                            p: 1, borderRadius: 1, fontSize: fontSizes.sm, lineHeight: 1.45,
                            color: `${noteKind}.dark`,
                            bgcolor: (theme) => alpha(theme.palette[noteKind].main, 0.1),
                            border: '1px solid',
                            borderColor: (theme) => alpha(theme.palette[noteKind].main, 0.4),
                        }}>
                            {noteText}
                        </Box>
                    )}

                    <Stack direction="row" sx={{ justifyContent: 'space-between', alignItems: 'center', pt: 1, borderTop: 1, borderColor: 'divider', fontSize: fontSizes.xs, color: 'text.secondary' }}>
                        <span>{t('ruleHeader.compact.needsApply')}</span>
                        {overridden && (
                            <Link
                                component="button"
                                type="button"
                                underline="hover"
                                onClick={() => { if (pending.current) { clearTimeout(pending.current.timer); pending.current = null; } setDraft(null); onChange(0); }}
                                sx={{ fontSize: fontSizes.xs }}
                            >
                                {t('ruleHeader.compact.reset', { percent: shared.percent })}
                            </Link>
                        )}
                    </Stack>
                </Stack>
            </Popover>
        </Box>
    );
};

export default CodexCompactChip;
