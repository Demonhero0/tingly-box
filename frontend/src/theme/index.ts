import { createTheme, type PaletteOptions } from '@mui/material/styles';
import { enUS, zhCN } from '@mui/material/locale';

// Claude brand tokens, the same source as tingly-box's `claude` palette:
//   Dark #141413 · Light #FAF9F5 · Mid Gray #B0AEA5 · Light Gray #E8E6DC
//   Orange #D97757 (accent) · Blue #6A9BCC · Green #788C5D
const accent = { main: '#D97757', light: '#E89572', dark: '#C26146', contrastText: '#ffffff' };

const light: PaletteOptions = {
    mode: 'light',
    primary: accent,
    secondary: { main: '#B0AEA5', light: '#C7C5BC', dark: '#8E8C84', contrastText: '#141413' },
    success: { main: '#788C5D' },
    error: { main: '#BF4434' },
    warning: { main: '#C2873B' },
    info: { main: '#6A9BCC' },
    background: { default: '#FAF9F5', paper: '#FFFFFF' },
    text: { primary: '#141413', secondary: '#6B6863', disabled: '#B0AEA5' },
    divider: '#E8E6DC',
    action: { hover: '#F2EFE3', selected: '#E8E6DC' },
};

const dark: PaletteOptions = {
    mode: 'dark',
    primary: accent,
    secondary: { main: '#8E8C84', contrastText: '#FAF9F5' },
    success: { main: '#94A87A' },
    error: { main: '#D26659' },
    warning: { main: '#DDA45A' },
    info: { main: '#8AB3DB' },
    background: { default: '#1F1E1D', paper: '#262624' },
    text: { primary: '#FAF9F5', secondary: '#B0AEA5', disabled: '#6B6863' },
    divider: '#3A3936',
    action: { hover: '#30302E', selected: '#3A3936' },
};

export type ResolvedMode = 'light' | 'dark';

export const sidebarBg = (mode: ResolvedMode) => (mode === 'light' ? '#F5F4ED' : '#1A1918');

export function createAppTheme(mode: ResolvedMode, language: string) {
    return createTheme(
        {
            palette: mode === 'light' ? light : dark,
            shape: { borderRadius: 10 },
            typography: {
                fontFamily: '"Inter", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", system-ui, -apple-system, "Segoe UI", sans-serif',
                h1: { fontSize: '1.75rem', fontWeight: 600 },
                h2: { fontSize: '1.5rem', fontWeight: 600 },
                h3: { fontSize: '1.25rem', fontWeight: 600 },
                h4: { fontSize: '1.125rem', fontWeight: 600 },
                h5: { fontSize: '1rem', fontWeight: 600 },
                h6: { fontSize: '0.9rem', fontWeight: 600 },
                body1: { fontSize: '0.9375rem', lineHeight: 1.65 },
                body2: { fontSize: '0.8125rem', lineHeight: 1.55 },
                caption: { fontSize: '0.75rem' },
                button: { textTransform: 'none', fontWeight: 600 },
            },
            components: {
                // Native-feeling controls: arrow cursor, as in tingly-box.
                MuiButtonBase: { styleOverrides: { root: { cursor: 'default' } } },
                MuiButton: { defaultProps: { disableElevation: true } },
                MuiPaper: { styleOverrides: { root: { backgroundImage: 'none' } } },
                MuiDialog: { styleOverrides: { paper: { borderRadius: 14 } } },
                MuiTooltip: { defaultProps: { arrow: true } },
                MuiChip: { styleOverrides: { root: { fontWeight: 500 } } },
            },
        },
        language.startsWith('zh') ? zhCN : enUS,
    );
}
