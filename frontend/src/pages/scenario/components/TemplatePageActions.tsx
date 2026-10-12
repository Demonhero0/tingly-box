import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
    Add as AddIcon,
    Bolt as BoltIcon,
    BugReport as TroubleshootIcon,
    FoldDown as FoldDownIcon,
    FoldUp as FoldUpIcon,
    HelpOutline as HelpOutlineIcon,
    Key as KeyIcon,
    MoreHoriz as MoreIcon,
    Sort as SortIcon,
    SortByAlpha as SortByAlphaIcon,
} from '@/components/icons';
import { Button, Divider, IconButton, ListItemIcon, ListItemText, Menu, MenuItem, Stack, Tooltip } from '@mui/material';
import type { RuleSortMode } from '@/pages/scenario/hooks/useRuleSort';

export interface TemplatePageActionsProps {
    collapsible: boolean;
    allExpanded: boolean;
    onToggleExpandAll: () => void;
    showAddApiKeyButton?: boolean;
    onAddApiKeyClick: () => void;
    allowAddRule: boolean;
    onCreateRule: () => void;
    showExpandCollapseButton?: boolean;
    onViewLogs?: () => void;
    /** Run the quick streaming probe on every active rule in the list. */
    onProbeAll?: () => void;
    onShowGuide?: () => void;
    // Rule-list display order — purely a view concern, doesn't touch the
    // rules' actual stored/routing order. Omit both to hide the item.
    sortMode?: RuleSortMode;
    onToggleSort?: () => void;
    // Probe props
    scenario?: string;
}

/**
 * The rules toolbar: the two everyday actions (Test All, Connect AI), New
 * Rule where rules can be added, and a "⋯" menu for the rest — Troubleshoot
 * and the list-view controls that used to be icon buttons beside the title
 * (sort order, fold all, "How routing works").
 */
export const TemplatePageActions: React.FC<TemplatePageActionsProps> = ({
    collapsible,
    allExpanded,
    onToggleExpandAll,
    showAddApiKeyButton = true,
    onAddApiKeyClick,
    allowAddRule,
    onCreateRule,
    showExpandCollapseButton = true,
    onViewLogs,
    onProbeAll,
    onShowGuide,
    sortMode,
    onToggleSort,
}) => {
    const { t } = useTranslation();
    const [moreAnchor, setMoreAnchor] = useState<HTMLElement | null>(null);
    const closeMenu = () => setMoreAnchor(null);
    const run = (fn?: () => void) => () => { closeMenu(); fn?.(); };

    const showSort = !!onToggleSort && !!sortMode;
    const showFold = showExpandCollapseButton && collapsible;
    const hasViewItems = showSort || showFold || !!onShowGuide;
    const hasMenu = !!onViewLogs || hasViewItems;

    return (
        <Stack direction="row" spacing={1.5} sx={{
            alignItems: "center"
        }}>
            {onProbeAll && (
                <Tooltip title={t('probe.testAllHint')}>
                    <Button
                        variant="outlined"
                        startIcon={<BoltIcon />}
                        onClick={onProbeAll}
                        size="small"
                    >
                        {t('probe.testAll')}
                    </Button>
                </Tooltip>
            )}
            {showAddApiKeyButton && (
                <Tooltip title={t('templateActions.connectAI')}>
                    <Button
                        variant="outlined"
                        startIcon={<KeyIcon/>}
                        onClick={onAddApiKeyClick}
                        size="small"
                    >
                        {t('templateActions.connectAI')}
                    </Button>
                </Tooltip>
            )}
            {allowAddRule && (
                <Tooltip title={t('templateActions.createNewRule')}>
                    <Button
                        variant="contained"
                        startIcon={<AddIcon/>}
                        onClick={onCreateRule}
                        size="small"
                    >
                        {t('templateActions.newRule')}
                    </Button>
                </Tooltip>
            )}
            {hasMenu && (
                <>
                    <Tooltip title={t('templateActions.more')}>
                        <IconButton
                            size="small"
                            aria-label={t('templateActions.more')}
                            aria-haspopup="menu"
                            onClick={(e) => setMoreAnchor(e.currentTarget)}
                            sx={{ color: 'text.secondary' }}
                        >
                            <MoreIcon fontSize="small" />
                        </IconButton>
                    </Tooltip>
                    <Menu
                        anchorEl={moreAnchor}
                        open={Boolean(moreAnchor)}
                        onClose={closeMenu}
                        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
                        transformOrigin={{ vertical: 'top', horizontal: 'right' }}
                    >
                        {onViewLogs && (
                            <MenuItem onClick={run(onViewLogs)}>
                                <ListItemIcon><TroubleshootIcon fontSize="small" /></ListItemIcon>
                                <ListItemText>{t('templateActions.troubleshoot')}</ListItemText>
                            </MenuItem>
                        )}
                        {onViewLogs && hasViewItems && <Divider />}
                        {showSort && (
                            <MenuItem onClick={run(onToggleSort)}>
                                <ListItemIcon>
                                    {sortMode === 'original' ? <SortIcon fontSize="small" /> : <SortByAlphaIcon fontSize="small" color="primary" />}
                                </ListItemIcon>
                                <ListItemText>{sortMode === 'original' ? t('templateActions.sortMenuToName') : t('templateActions.sortMenuToOriginal')}</ListItemText>
                            </MenuItem>
                        )}
                        {showFold && (
                            <MenuItem onClick={run(onToggleExpandAll)}>
                                <ListItemIcon>{allExpanded ? <FoldUpIcon fontSize="small" /> : <FoldDownIcon fontSize="small" />}</ListItemIcon>
                                <ListItemText>{allExpanded ? t('templateActions.collapseAllRules') : t('templateActions.expandAllRules')}</ListItemText>
                            </MenuItem>
                        )}
                        {onShowGuide && (
                            <MenuItem onClick={run(onShowGuide)}>
                                <ListItemIcon><HelpOutlineIcon fontSize="small" /></ListItemIcon>
                                <ListItemText>{t('templateActions.howRoutingWorks', { defaultValue: 'How routing works' })}</ListItemText>
                            </MenuItem>
                        )}
                    </Menu>
                </>
            )}
        </Stack>
    );
};
