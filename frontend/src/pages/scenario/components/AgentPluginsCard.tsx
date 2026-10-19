import { Box, Card } from '@mui/material';
import PluginFeatures from '@/components/PluginFeatures';

/**
 * Page-level plugins (Thinking, Vision Proxy, Record) as their own card, right
 * under the connection section on every page that has one (Agent pages, a
 * Claude Code profile, a Team, Image API). Always shown, whether the connection
 * above is folded or not: plugins are a setting people come back to, the
 * connection is set once.
 */
export const AgentPluginsCard: React.FC<{ scenario: string }> = ({ scenario }) => (
    <Card sx={{ borderRadius: 2, border: '1px solid', borderColor: 'divider', boxShadow: 'none', backgroundColor: 'background.paper' }}>
        {/* Same height and left inset as the folded connection card above. */}
        <Box sx={{ px: 2, py: 0.5, minHeight: 58, display: 'flex', alignItems: 'center', '& > *': { flex: 1, minWidth: 0 } }}>
            <PluginFeatures scenario={scenario} />
        </Box>
    </Card>
);

export default AgentPluginsCard;
