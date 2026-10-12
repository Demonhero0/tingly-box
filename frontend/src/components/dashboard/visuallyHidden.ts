/** Off-screen but still read by assistive tech (an h1 the sidebar already shows). */
export const visuallyHidden = {
    position: 'absolute', width: 1, height: 1, margin: -1, padding: 0,
    overflow: 'hidden', clip: 'rect(0 0 0 0)', whiteSpace: 'nowrap', border: 0,
} as const;
