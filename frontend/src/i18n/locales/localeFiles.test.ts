import { describe, expect, it } from 'vitest';

type Bundle = Record<string, unknown>;

// Each locale is split into feature-area files (locales/<lang>/<area>.ts) that
// index.ts spreads into one object, so a namespace defined in two areas would
// silently lose one copy. Keep the areas disjoint and identical across locales.
const AREAS = import.meta.glob('./*/*.ts', { import: 'default', eager: true }) as Record<string, Bundle>;

const byLocale = new Map<string, Map<string, string[]>>();
for (const [path, bundle] of Object.entries(AREAS)) {
    const [, locale, file] = path.split('/');
    const area = file.replace(/\.ts$/, '');
    if (area === 'index') continue;
    if (!byLocale.has(locale)) byLocale.set(locale, new Map());
    byLocale.get(locale)!.set(area, Object.keys(bundle).sort());
}

const layout = (locale: string) =>
    Object.fromEntries([...byLocale.get(locale)!.entries()].sort(([a], [b]) => a.localeCompare(b)));

describe('locale files', () => {
    it.each([...byLocale.keys()])('%s puts each namespace in exactly one area', (locale) => {
        const seen = new Map<string, string>();
        const duplicates: string[] = [];
        for (const [area, namespaces] of byLocale.get(locale)!) {
            for (const ns of namespaces) {
                if (seen.has(ns)) duplicates.push(`${ns} (${seen.get(ns)}, ${area})`);
                seen.set(ns, area);
            }
        }
        expect(duplicates).toEqual([]);
    });

    it.each([...byLocale.keys()].filter((l) => l !== 'en'))('%s uses the same area layout as en', (locale) => {
        expect(layout(locale)).toEqual(layout('en'));
    });
});
