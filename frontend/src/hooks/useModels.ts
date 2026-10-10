import { useEffect, useState } from 'react';
import { api, unwrap, type Model } from '@/client';

let cache: { models: Model[]; error?: string } | null = null;
let inflight: Promise<{ models: Model[]; error?: string }> | null = null;

/** Models offered by the saved connection, fetched once per page load. */
export function useModels() {
    const [state, setState] = useState(cache);
    useEffect(() => {
        if (cache) return;
        inflight ??= unwrap(api.GET('/api/v1/models'))
            .then((r) => ({ models: r.models, error: r.error }))
            .catch((e: Error) => ({ models: [], error: e.message }));
        inflight.then((r) => {
            cache = r;
            setState(r);
        });
    }, []);
    return { models: state?.models ?? [], error: state?.error, loading: !state };
}

/** Drop the cached list, e.g. after the connection settings change. */
export function invalidateModels() {
    cache = null;
    inflight = null;
}
