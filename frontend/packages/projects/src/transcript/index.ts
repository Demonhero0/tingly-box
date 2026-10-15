// @tingly/projects/transcript: an agent session's transcript, as a model and
// as the components that render it. Props only — the host (Desk, Projects'
// thread page) loads the entries and answers requests; nothing here calls an
// API. Strings live under projects.transcript.* in the host's i18n bundles.

export type {
    ActivityStep, TaskEvent, TaskState, ThinkingStep, ToolStep, TranscriptBlock, TranscriptEntry,
} from './model';
export {
    agentReport, agentStatus, buildTranscript, foldTasks, formatTokens, isAgentTool, pendingRequestId, toolSummary,
} from './model';

export type {TrajectoryAnchor, TrajectoryFilter, TrajectoryKind, TrajectoryRow, TrajectorySummary} from './trajectoryRows';
export {matchesFilter, plainLine, trajectoryRows, trajectorySummary} from './trajectoryRows';

export type {CodeBlockComponent, DraftStorage} from './context';
export {createMemoryDraftStorage} from './context';

export {default as Transcript} from './Transcript';
export {default as Trajectory} from './Trajectory';
export {default as Markdown} from './Markdown';
