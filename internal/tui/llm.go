// Package tui — LLM view removed.
//
// AI analysis was moved out of the TUI intentionally. The TUI is now a
// pure proxy/recon/scope workbench. All LLM-assisted triage is via the
// decoupled skill interface which never blocks the UI:
//
//   bash skills/ouroboros-advisor/scripts/query.sh overview
//   bash skills/ouroboros-advisor/scripts/query.sh triage
//   bash skills/ouroboros-advisor/scripts/query.sh flow <id>
//   bash skills/ouroboros-advisor/scripts/query.sh triage-injections
//
// Internal LLM provider code lives in internal/llm and is only used by the
// skill/query layer, not by Bubble Tea handlers. Keeping this file as a
// placeholder avoids dead imports and documents the architecture decision.
package tui
